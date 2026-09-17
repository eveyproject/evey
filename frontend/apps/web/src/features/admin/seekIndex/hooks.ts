import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "@evey/store";
import {
  useLibraryServiceListLibraries,
  useAdminMediaServiceScanSeekIndex,
  useAdminMediaServiceStartSeekIndexScan,
  useAdminMediaServiceGetSeekIndexJob,
  useAdminMediaServiceReindexMediaSeekIndex,
  useAdminMediaServiceRollbackMediaSeekIndex,
  useAdminMediaServiceReindexLibrarySeekIndex,
  useAdminMediaServiceRollbackLibrarySeekIndex,
  useAdminMediaServiceDeleteSeekIndexBackup,
  useAdminMediaServicePurgeSeekIndexBackups,
  useAdminMediaServiceAckSeekIndexMedia,
  useAdminMediaServiceAckSeekIndexGroup,
  useAdminMediaServiceFixContainerMedia,
  useAdminMediaServiceReinterleaveMedia, audioDelayed, audioTracksDisagree, brokenInterleave } from "@evey/api";
import {
  SEEK_INDEX_SCAN_QUERY_BASE,
  SEEK_INDEX_KIND_KEYS,
  errText,
  isAcked,
  triageOf,
  triageRows,
  verdictOf,
  viewCounts,
  volumeBackupsRisky,
} from "@evey/api";
import type { SeekSort, SeekView } from "@evey/api";

// Rows per page: a 7 000-file library must not render 900 rows at once, and
// the operator triages a screen at a time anyway.
export const SEEK_INDEX_PAGE_SIZE = 50;

// useSeekIndexPage owns every piece of state, query and mutation the media
// health admin page renders from - the page component itself only composes
// panels around the values this returns.
export function useSeekIndexPage() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [libraryId, setLibraryId] = useState("");
  // The displayed rows are triaged three ways: a view (what awaits attention
  // by default, or one verdict, or acknowledged/backups/all), a search on
  // title or path, and a sort whose default puts what needs doing first.
  const [view, setViewState] = useState<SeekView>("attention");
  const [query, setQueryState] = useState("");
  const [sort, setSort] = useState<SeekSort>("priority");
  const [page, setPage] = useState(1);
  // A new view or search restarts at the first page: page 4 of the old list
  // is nowhere in the new one.
  const setView = (v: SeekView) => {
    setViewState(v);
    setPage(1);
  };
  const setQuery = (q: string) => {
    setQueryState(q);
    setPage(1);
  };
  const [confirmBatch, setConfirmBatch] = useState<"repair" | "rollback" | "purge" | null>(null);
  // "Repair all" repairs EVERY repairable verdict - index, audio start delay,
  // container - with no per-defect checkbox: a button named "all" that left a
  // known, fixable defect behind an option was lying about its name. The two
  // choices that remain are real ones:
  //
  // keepBackups ON  -> a .mkvgobak per repaired file survives the job (roll
  //                    back anytime, costs one extra copy per file on disk).
  // keepBackups OFF -> a rolling window keeps only the last few backups during
  //                    the run and drains them at the end: the volume finishes
  //                    with none, and each repair is deep-verified in exchange
  //                    for the lost post-run rollback. Pre-set from the live
  //                    volume space so a tight disk defaults to OFF.
  // resyncDamaged   -> also rewrite the files whose index is fine but which
  //                    carry a corrupted stretch. It is the ONLY way they
  //                    become playable, and it LOSES those bytes for good, so
  //                    it is asked with the count and the bytes at stake.
  //                    Their backup is always kept, whatever keepBackups says.
  const [keepBackups, setKeepBackups] = useState(true);
  const [resyncDamaged, setResyncDamaged] = useState(true);

  const { data: libData } = useLibraryServiceListLibraries();
  const libraries = libData?.libraries ?? [];

  // The job endpoint is the live heartbeat: poll fast while a job runs so the
  // progress bar moves, slowly otherwise to pick up jobs started elsewhere.
  const jobQuery = useAdminMediaServiceGetSeekIndexJob({
    query: {
      refetchInterval: (q) => (q.state.data?.running ? 1500 : 10_000),
      refetchIntervalInBackground: false,
    },
  });
  const job = jobQuery.data;
  const running = job?.running ?? false;

  // Serves the cached result of the last completed scan - instant.
  const scan = useAdminMediaServiceScanSeekIndex(libraryId ? { libraryId } : undefined, {
    query: { retry: false },
  });
  const hasScan = Number(scan.data?.scannedAtMs ?? 0) > 0;
  const result = hasScan ? scan.data : undefined;
  const items = result?.items ?? [];
  const withBackup = result?.withBackup ?? 0;
  const containerFixAvailable = result?.containerFixAvailable ?? false;
  const reinterleaveAvailable = result?.reinterleaveAvailable ?? false;
  // The triage is derived from the very rows the table shows, so the group
  // cards, the button and the table can never disagree with each other.
  const triage = triageOf(items);
  const repairable = triage.repairable;
  // Per-category breakdown under each group card: the same rows, by defect.
  const breakdown = {
    missing: items.filter((it) => verdictOf(it) === "repairable" && !it.hasIndex).length,
    damaged: items.filter((it) => verdictOf(it) === "repairable" && it.hasIndex && Number(it.damagedBytes ?? 0) > 0).length,
    damagedBytes: items
      .filter((it) => verdictOf(it) === "repairable" && it.hasIndex)
      .reduce((sum, it) => sum + Number(it.damagedBytes ?? 0), 0),
    // An offset is information, not a repair - it belongs under the
    // information card, and it is counted whether or not the coherence
    // comparison could run.
    delayedAudio: items.filter((it) => verdictOf(it) === "informational" && audioDelayed(it)).length,
    tracksDisagree: items.filter((it) => audioTracksDisagree(it) && !isAcked(it)).length,
    wrongContainer: items.filter((it) => it.wrongContainer).length,
    truncated: items.filter((it) => it.truncated && !isAcked(it)).length,
    seekCoarse: items.filter((it) => it.seekCoarse && !it.truncated && !isAcked(it)).length,
    audioShort: items.filter((it) => verdictOf(it) === "informational" && (it.audioShortfallSec ?? 0) > 0 && !isAcked(it)).length,
    trailingJunk: items.filter((it) => verdictOf(it) === "informational" && it.trailingJunk && !isAcked(it)).length,
    brokenInterleave: items.filter((it) => brokenInterleave(it) && !isAcked(it)).length,
    // Counted APART from `damaged`, and never added to `damagedBytes`: these
    // are the files a repair already refused as corrupt, and how much a resync
    // would drop is unknown for them. Folding them into the byte total would
    // put a figure in front of the operator that understates what is at stake.
    resyncRequired: items.filter((it) => it.resyncRequired).length,
  };
  // Per-filesystem numbers for the candidates: Evey pre-sets the safe backup
  // choice itself (a nearly-full volume must not accumulate one copy per
  // repaired file) and flags files whose rewrite temp copy will not fit.
  const repairVolumes = (result?.volumes ?? []).filter((v) => (v.missingFiles ?? 0) > 0);
  const backupsRisky = repairVolumes.some(volumeBackupsRisky);

  // When the running job finishes, refresh the cached scans and report the
  // outcome - completion happens server-side, not in a mutation callback.
  const prevRunning = useRef(false);
  useEffect(() => {
    if (prevRunning.current && !running) {
      void queryClient.invalidateQueries({ queryKey: [SEEK_INDEX_SCAN_QUERY_BASE] });
      const last = job?.last;
      if (last) {
        const kind = t(SEEK_INDEX_KIND_KEYS[last.kind ?? ""] ?? SEEK_INDEX_KIND_KEYS.scan);
        if (last.error) {
          toast.error(t("admin.seekIndex.lastJobError", { error: last.error }));
        } else if ((last.failed ?? 0) > 0) {
          toast.error(
            t("admin.seekIndex.jobCompletedFailures", {
              kind,
              done: last.done ?? 0,
              failed: last.failed ?? 0,
            }),
          );
        } else if ((last.skipped ?? 0) > 0) {
          // Never announce a plain success on a run that mostly skipped: that
          // toast is what made a no-op repair batch look like a repaired library.
          toast.success(
            t("admin.seekIndex.jobCompletedSkipped", {
              kind,
              done: (last.done ?? 0) - (last.skipped ?? 0),
              skipped: last.skipped ?? 0,
            }),
          );
        } else {
          toast.success(t("admin.seekIndex.jobCompleted", { kind, done: last.done ?? 0 }));
        }
      }
    }
    prevRunning.current = running;
  }, [running, job?.last, queryClient, t]);

  const onStarted = (msgKey: string) => () => {
    toast.info(t(msgKey));
    void queryClient.invalidateQueries({ queryKey: jobQuery.queryKey ?? [] });
  };
  const onStartError = (err: { message?: string } | null) =>
    toast.error(errText(err, t("admin.seekIndex.startFailed")));
  // Synchronous mutations (a cache update or an unlink server-side): refresh
  // the cached scan directly instead of the job-completion path.
  const refreshScan = () => void queryClient.invalidateQueries({ queryKey: [SEEK_INDEX_SCAN_QUERY_BASE] });

  const startScan = useAdminMediaServiceStartSeekIndexScan({
    mutation: {
      onSuccess: onStarted("admin.seekIndex.scanStarted"),
      onError: onStartError,
    },
  });
  const repairOne = useAdminMediaServiceReindexMediaSeekIndex({
    mutation: {
      onSuccess: onStarted("admin.seekIndex.repairStarted"),
      onError: onStartError,
    },
  });
  const rollbackOne = useAdminMediaServiceRollbackMediaSeekIndex({
    mutation: {
      onSuccess: onStarted("admin.seekIndex.rollbackStarted"),
      onError: onStartError,
    },
  });
  const repairLib = useAdminMediaServiceReindexLibrarySeekIndex({
    mutation: {
      onSuccess: onStarted("admin.seekIndex.repairStarted"),
      onError: onStartError,
    },
  });
  const rollbackLib = useAdminMediaServiceRollbackLibrarySeekIndex({
    mutation: {
      onSuccess: onStarted("admin.seekIndex.rollbackStarted"),
      onError: onStartError,
    },
  });
  const fixContainerOne = useAdminMediaServiceFixContainerMedia({
    mutation: {
      onSuccess: onStarted("admin.seekIndex.fixContainerStarted"),
      onError: onStartError,
    },
  });
  const reinterleaveOne = useAdminMediaServiceReinterleaveMedia({
    mutation: {
      onSuccess: onStarted("admin.seekIndex.reinterleaveStarted"),
      onError: onStartError,
    },
  });
  const deleteBackup = useAdminMediaServiceDeleteSeekIndexBackup({
    mutation: {
      onSuccess: () => {
        toast.success(t("admin.seekIndex.backupDeleted"));
        refreshScan();
      },
      onError: onStartError,
    },
  });
  const purgeLib = useAdminMediaServicePurgeSeekIndexBackups({
    mutation: {
      onSuccess: onStarted("admin.seekIndex.purgeStarted"),
      onError: onStartError,
    },
  });
  // Acknowledging is the operator saying "seen, nothing to do here": the row
  // leaves the attention list until the file's bytes change.
  const ackOne = useAdminMediaServiceAckSeekIndexMedia({
    mutation: {
      onSuccess: (_res, vars) => {
        toast.success(t(vars.data.acked ? "admin.seekIndex.ackDone" : "admin.seekIndex.unackDone"));
        refreshScan();
      },
      onError: onStartError,
    },
  });
  const ackGroup = useAdminMediaServiceAckSeekIndexGroup({
    mutation: {
      onSuccess: (res) => {
        toast.success(t("admin.seekIndex.ackGroupDone", { count: res.acked ?? 0 }));
        refreshScan();
      },
      onError: onStartError,
    },
  });

  const counts = viewCounts(items);
  const visible = triageRows(items, view, query, sort);
  const totalPages = Math.max(1, Math.ceil(visible.length / SEEK_INDEX_PAGE_SIZE));
  const safePage = Math.min(page, totalPages);
  const paged = visible.slice((safePage - 1) * SEEK_INDEX_PAGE_SIZE, safePage * SEEK_INDEX_PAGE_SIZE);
  const busy = running || startScan.isPending;

  return {
    libraryId,
    setLibraryId,
    view,
    setView,
    query,
    setQuery,
    sort,
    setSort,
    page: safePage,
    setPage,
    totalPages,
    counts,
    confirmBatch,
    setConfirmBatch,
    keepBackups,
    setKeepBackups,
    resyncDamaged,
    setResyncDamaged,
    libraries,
    job,
    running,
    scan,
    result,
    items,
    withBackup,
    containerFixAvailable,
    reinterleaveAvailable,
    triage,
    breakdown,
    repairable,
    repairVolumes,
    backupsRisky,
    startScan,
    repairOne,
    rollbackOne,
    repairLib,
    rollbackLib,
    fixContainerOne,
    reinterleaveOne,
    deleteBackup,
    purgeLib,
    ackOne,
    ackGroup,
    visible,
    paged,
    busy,
  };
}

export type SeekIndexPageState = ReturnType<typeof useSeekIndexPage>;
