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
  useAdminMediaServiceRemuxAudioMedia,
} from "@evey/api";
import { SCAN_QUERY_BASE, KIND_KEYS, errText, audioDelayed, volumeBackupsRisky } from "./helpers";

// useSeekIndexPage owns every piece of state, query and mutation the seek-index
// admin page renders from - the page component itself only composes panels
// around the values this returns.
export function useSeekIndexPage() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [libraryId, setLibraryId] = useState("");
  const [onlyAttention, setOnlyAttention] = useState(true);
  const [confirmBatch, setConfirmBatch] = useState<"repair" | "rollback" | "purge" | null>(null);
  // "Repair all" exposes just two independent choices; the rest is decided for
  // the operator as the safe default. Strategy is always "auto" (surgical
  // patch when the layout allows, full rewrite otherwise) - the per-file
  // power modes are not worth a whole-library toggle.
  //
  // keepBackups ON  -> a .mkvgobak per repaired file survives the job (roll
  //                    back anytime, costs one extra copy per file on disk).
  // keepBackups OFF -> a rolling window keeps only the last few backups during
  //                    the run and drains them at the end: the volume finishes
  //                    with none, and each repair is deep-verified in exchange
  //                    for the lost post-run rollback. Pre-set from the live
  //                    volume space so a tight disk defaults to OFF.
  const [keepBackups, setKeepBackups] = useState(true);
  // alsoAudio folds the audio-delay remux of every flagged file into the same
  // job. Independent of keepBackups. Default ON: "repair all" should fix every
  // defect the scan found, not just the index.
  const [alsoAudio, setAlsoAudio] = useState(true);

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
  const missing = result?.missingIndex ?? 0;
  const withBackup = result?.withBackup ?? 0;
  const delayedAudio = result?.audioDelayed ?? 0;
  const wrongContainer = result?.wrongContainer ?? 0;
  const trailingJunk = result?.trailingJunk ?? 0;
  const audioShort = result?.audioShort ?? 0;
  const seekCoarse = result?.seekCoarse ?? 0;
  // What "Repair all" will actually act on: the missing-index count minus the
  // files the batch (rightly) skips - advertising the raw missing count
  // promised repairs the job then silently declined. Truncated: the tail is
  // gone, no repair exists. Coarse: already rebuilt once and still too sparse
  // to seek, because the encode's keyframes are the limit, not the index.
  const repairable = items.filter((it) => !it.hasIndex && !it.truncated && !it.seekCoarse).length;
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
      void queryClient.invalidateQueries({ queryKey: [SCAN_QUERY_BASE] });
      const last = job?.last;
      if (last) {
        const kind = t(KIND_KEYS[last.kind ?? ""] ?? KIND_KEYS.scan);
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
  const remuxAudioOne = useAdminMediaServiceRemuxAudioMedia({
    mutation: {
      onSuccess: onStarted("admin.seekIndex.remuxAudioStarted"),
      onError: onStartError,
    },
  });
  // Synchronous (an unlink server-side): refresh the cached scan directly
  // instead of the job-completion path the batch mutations rely on.
  const deleteBackup = useAdminMediaServiceDeleteSeekIndexBackup({
    mutation: {
      onSuccess: () => {
        toast.success(t("admin.seekIndex.backupDeleted"));
        void queryClient.invalidateQueries({ queryKey: [SCAN_QUERY_BASE] });
      },
      onError: (err: { message?: string } | null) =>
        toast.error(errText(err, t("admin.seekIndex.startFailed"))),
    },
  });
  const purgeLib = useAdminMediaServicePurgeSeekIndexBackups({
    mutation: {
      onSuccess: onStarted("admin.seekIndex.purgeStarted"),
      onError: onStartError,
    },
  });

  const visible = onlyAttention
    ? items.filter(
        (it) =>
          !it.hasIndex ||
          it.hasBackup ||
          audioDelayed(it) ||
          it.wrongContainer ||
          // Files whose defect has no automated repair still NEED the
          // operator's attention - truncated/damaged/short-audio all mean
          // "re-acquire the source", and hiding them under the default filter
          // made the badges unreachable without unticking it.
          it.truncated ||
          Number(it.damagedBytes ?? 0) > 0 ||
          (it.audioShortfallSec ?? 0) > 0,
      )
    : items;
  const busy = running || startScan.isPending;

  return {
    libraryId,
    setLibraryId,
    onlyAttention,
    setOnlyAttention,
    confirmBatch,
    setConfirmBatch,
    keepBackups,
    setKeepBackups,
    alsoAudio,
    setAlsoAudio,
    libraries,
    job,
    running,
    scan,
    result,
    items,
    missing,
    withBackup,
    delayedAudio,
    wrongContainer,
    trailingJunk,
    audioShort,
    seekCoarse,
    repairable,
    repairVolumes,
    backupsRisky,
    startScan,
    repairOne,
    rollbackOne,
    repairLib,
    rollbackLib,
    remuxAudioOne,
    deleteBackup,
    purgeLib,
    visible,
    busy,
  };
}

export type SeekIndexPageState = ReturnType<typeof useSeekIndexPage>;
