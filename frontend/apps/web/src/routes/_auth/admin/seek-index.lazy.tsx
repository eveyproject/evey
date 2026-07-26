import { useEffect, useRef, useState } from "react";
import { ScanSearch } from "lucide-react";
import { createLazyFileRoute } from "@tanstack/react-router";
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
import type {
  V1SeekIndexItem,
  V1GetSeekIndexJobResponse,
  V1SeekIndexJobFailure,
  V1SeekIndexVolume,
} from "@evey/api";
import { Spinner } from "@/components/ui/Spinner";
import { PageHeader } from "@/components/ui/PageHeader";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { Card } from "@/components/ui/Card";
import { Modal } from "@/components/ui/Modal";
import { Select } from "@/components/ui/Select";
import { Checkbox } from "@/components/ui/Checkbox";
import { EmptyState } from "@/components/ui/EmptyState";

export const Route = createLazyFileRoute("/_auth/admin/seek-index")({
  component: SeekIndexPage,
});

// Base path of the cached-scan GET - invalidating it refreshes every scope's
// cached result once a job finishes (the job key has a different base string).
const SCAN_QUERY_BASE = "/api/v1/admin/media/seek-index";

const KIND_KEYS: Record<string, string> = {
  scan: "admin.seekIndex.jobScan",
  reindex: "admin.seekIndex.jobReindex",
  rollback: "admin.seekIndex.jobRollback",
  purge: "admin.seekIndex.jobPurge",
  remux_audio: "admin.seekIndex.jobRemuxAudio",
};

// Mirror of the backend's maintenance.AudioDelayFlagSec: above it the file's
// audio starts late enough to be a mux defect, not codec priming.
const AUDIO_DELAY_FLOOR_SEC = 0.1;

function audioDelayed(it: V1SeekIndexItem): boolean {
  return (it.audioDelaySec ?? 0) > AUDIO_DELAY_FLOOR_SEC;
}

function formatSize(bytes: number): string {
  if (!bytes) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return `${(bytes / Math.pow(1024, i)).toFixed(i > 0 ? 1 : 0)} ${units[i]}`;
}

// Mirror of the server's rewrite space-preflight margin: the numbers shown to
// the operator should predict what the repair itself will accept.
const VOLUME_HEADROOM_BYTES = 256 * 1024 * 1024;

// Rolling-backup window for whole-library repairs: at most this many
// .mkvgobak copies exist at any moment during the sweep.
const BACKUP_WINDOW = 5;

// Keeping backups persists the SUM of the candidates on the volume.
function volumeBackupsRisky(v: V1SeekIndexVolume): boolean {
  return Number(v.missingBytes ?? 0) + VOLUME_HEADROOM_BYTES > Number(v.freeBytes ?? 0);
}

// One rewrite needs the largest candidate free as a temporary copy.
function volumeTempRisky(v: V1SeekIndexVolume): boolean {
  return Number(v.largestMissingBytes ?? 0) + VOLUME_HEADROOM_BYTES > Number(v.freeBytes ?? 0);
}

function errText(err: { message?: string } | null | undefined, fallback: string): string {
  return err?.message || fallback;
}

function baseName(p: string): string {
  const i = p.lastIndexOf("/");
  return i >= 0 ? p.slice(i + 1) : p;
}

function JobProgressCard({ job }: { job: V1GetSeekIndexJobResponse }) {
  const { t } = useTranslation();
  const done = job.done ?? 0;
  const total = job.total ?? 0;
  const failed = job.failed ?? 0;
  const pct = total > 0 ? Math.min(100, Math.round((done / total) * 100)) : 0;
  const kind = t(KIND_KEYS[job.kind ?? ""] ?? KIND_KEYS.scan);
  const phase =
    job.phase === "processing"
      ? t("admin.seekIndex.phaseProcessing")
      : t("admin.seekIndex.phaseScanning");

  return (
    <Card data-testid="seek-index-job-card">
      <div className="flex items-center justify-between mb-2">
        <div className="flex items-center gap-2">
          <Spinner size="sm" />
          <span className="text-sm font-medium text-[var(--text)]">
            {t("admin.seekIndex.jobRunning", { kind, phase })}
          </span>
        </div>
        <span className="text-sm text-[var(--text-muted)]" data-testid="seek-index-job-progress">
          {total > 0 ? `${done}/${total}` : "…"}
          {failed > 0 && (
            <span className="text-[var(--color-danger)] ml-2">
              {t("admin.seekIndex.jobFailedCount", { count: failed })}
            </span>
          )}
        </span>
      </div>
      <div
        className="h-2 rounded-full bg-[var(--surface-hover)] overflow-hidden"
        role="progressbar"
        aria-label={t("admin.seekIndex.jobRunning", { kind, phase })}
        aria-valuemin={0}
        aria-valuemax={total > 0 ? total : undefined}
        aria-valuenow={total > 0 ? done : undefined}
        aria-valuetext={total > 0 ? `${done}/${total}` : t("admin.seekIndex.phaseScanning")}
      >
        <div
          className={`h-full bg-[var(--accent)] transition-all ${total === 0 ? "w-full opacity-40 animate-pulse" : ""}`}
          style={total > 0 ? { width: `${pct}%` } : undefined}
        />
      </div>
      {job.currentPath && (
        <p
          className="mt-2 font-mono text-xs text-[var(--text-muted)] truncate"
          title={job.currentPath}
        >
          {baseName(job.currentPath)}
        </p>
      )}
    </Card>
  );
}

function LastJobCard({ last }: { last: NonNullable<V1GetSeekIndexJobResponse["last"]> }) {
  const { t } = useTranslation();
  const kind = t(KIND_KEYS[last.kind ?? ""] ?? KIND_KEYS.scan);
  const failed = last.failed ?? 0;
  const failures = last.failures ?? [];
  // "Processed" counts files the job LOOKED at, skips included. A run that
  // repaired nothing and one that repaired everything both read "138/138", which
  // is how a repair batch skipping 133 files went unnoticed for days (prod
  // 2026-07-26). The skipped count is the difference, so it is shown.
  const skipped = last.skipped ?? 0;

  return (
    <Card data-testid="seek-index-last-job">
      <div className="flex flex-wrap items-center gap-2 text-sm">
        <span className="font-medium text-[var(--text)]">{t("admin.seekIndex.lastJob")}</span>
        <span className="text-[var(--text-muted)]">
          {t("admin.seekIndex.lastJobLine", {
            kind,
            done: last.done ?? 0,
            total: last.total ?? 0,
            failed,
          })}
        </span>
        {skipped > 0 && (
          <span
            className="text-[var(--text-muted)]"
            title={t("admin.seekIndex.jobSkippedHint")}
            data-testid="seek-index-skipped-count"
          >
            {t("admin.seekIndex.jobSkippedCount", { count: skipped })}
          </span>
        )}
        {failed > 0 ? (
          <Badge variant="warning">{t("admin.seekIndex.jobFailedCount", { count: failed })}</Badge>
        ) : !last.error ? (
          <Badge variant="success">{t("admin.seekIndex.lastJobOk")}</Badge>
        ) : null}
      </div>
      {last.error && (
        <p className="mt-2 text-sm text-[var(--color-danger)]" role="alert">
          {t("admin.seekIndex.lastJobError", { error: last.error })}
        </p>
      )}
      {last.readOnly && (
        <p className="mt-2 text-sm text-[var(--color-warning)]" data-testid="seek-index-ro-hint">
          {t("admin.seekIndex.jobReadOnlyHint")}
        </p>
      )}
      {failures.length > 0 && (
        <ul className="mt-2 flex flex-col gap-1">
          {failures.map((f: V1SeekIndexJobFailure) => (
            <li key={f.path} className="text-xs text-[var(--text-muted)]">
              <span className="font-mono" title={f.path}>
                {baseName(f.path ?? "")}
              </span>
              {" - "}
              {f.error}
            </li>
          ))}
        </ul>
      )}
    </Card>
  );
}

function SeekIndexPage() {
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

  return (
    <div className="flex flex-col gap-6" data-testid="seek-index-page">
      <PageHeader title={t("admin.seekIndex.title")} />

      <Card>
        <div className="flex flex-col gap-2 text-sm text-[var(--text-muted)]">
          <p>{t("admin.seekIndex.description")}</p>
          <p data-testid="seek-index-ro-note">{t("admin.seekIndex.readOnlyNote")}</p>
        </div>
      </Card>

      <div className="flex flex-wrap items-end gap-3">
        <div className="w-64">
          <Select
            label={t("admin.seekIndex.library")}
            value={libraryId}
            onChange={setLibraryId}
            options={[
              { value: "", label: t("admin.seekIndex.allLibraries") },
              ...libraries.map((l) => ({ value: l.id ?? "", label: l.name ?? l.id ?? "" })),
            ]}
          />
        </div>
        <Button
          onClick={() => startScan.mutate({ data: libraryId ? { libraryId } : {} })}
          loading={busy}
          data-testid="seek-index-scan-btn"
        >
          {result ? t("admin.seekIndex.scanAgain") : t("admin.seekIndex.scan")}
        </Button>
        {result && (
          <>
            <Button
              variant="secondary"
              disabled={(repairable === 0 && delayedAudio === 0) || busy}
              onClick={() => {
                // Pre-set the safe choices; the operator can still flip them.
                setKeepBackups(!backupsRisky);
                setAlsoAudio(delayedAudio > 0);
                setConfirmBatch("repair");
              }}
              data-testid="seek-index-repair-all-btn"
            >
              {t("admin.seekIndex.repairAll", { count: repairable + delayedAudio })}
            </Button>
            <Button
              variant="secondary"
              disabled={withBackup === 0 || busy}
              onClick={() => setConfirmBatch("rollback")}
              data-testid="seek-index-rollback-all-btn"
            >
              {t("admin.seekIndex.rollbackAll", { count: withBackup })}
            </Button>
            <Button
              variant="ghost"
              disabled={withBackup === 0 || busy}
              onClick={() => setConfirmBatch("purge")}
              data-testid="seek-index-purge-all-btn"
            >
              {t("admin.seekIndex.purgeAll", { count: withBackup })}
            </Button>
          </>
        )}
      </div>

      {running && job && <JobProgressCard job={job} />}
      {!running && job?.last && <LastJobCard last={job.last} />}

      {scan.isError && (
        <p className="text-sm text-[var(--color-danger)]" role="alert">
          {errText(scan.error, t("admin.seekIndex.scanFailed"))}
        </p>
      )}

      {!result ? (
        <div className="rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]">
          <EmptyState
            icon={<ScanSearch size={48} strokeWidth={1.5} />}
            title={t("admin.seekIndex.emptyTitle")}
            description={t("admin.seekIndex.emptyDescription")}
          />
        </div>
      ) : (
        <>
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
            <Card>
              <dt className="text-xs font-medium mb-1 text-[var(--text-muted)]">
                {t("admin.seekIndex.statsInspected")}
              </dt>
              <dd className="text-xl font-bold text-[var(--text)]">{result.total ?? 0}</dd>
            </Card>
            <Card>
              <dt className="text-xs font-medium mb-1 text-[var(--text-muted)]">
                {t("admin.seekIndex.statsMissing")}
              </dt>
              <dd className="text-xl font-bold text-[var(--text)]" data-testid="seek-index-missing-count">
                {missing}
              </dd>
            </Card>
            <Card>
              <dt className="text-xs font-medium mb-1 text-[var(--text-muted)]">
                {t("admin.seekIndex.statsBackup")}
              </dt>
              <dd className="text-xl font-bold text-[var(--text)]">{withBackup}</dd>
            </Card>
            <Card>
              <dt className="text-xs font-medium mb-1 text-[var(--text-muted)]">
                {t("admin.seekIndex.statsAudioDelay")}
              </dt>
              <dd className="text-xl font-bold text-[var(--text)]" data-testid="seek-index-audio-delay-count">
                {delayedAudio}
              </dd>
            </Card>
            {wrongContainer > 0 && (
              <Card>
                <dt className="text-xs font-medium mb-1 text-[var(--text-muted)]">
                  {t("admin.seekIndex.statsWrongContainer")}
                </dt>
                <dd className="text-xl font-bold text-[var(--text)]" data-testid="seek-index-wrong-container-count">
                  {wrongContainer}
                </dd>
              </Card>
            )}
            {trailingJunk > 0 && (
              <Card>
                <dt className="text-xs font-medium mb-1 text-[var(--text-muted)]">
                  {t("admin.seekIndex.statsTrailingJunk")}
                </dt>
                <dd className="text-xl font-bold text-[var(--text)]" data-testid="seek-index-trailing-junk-count">
                  {trailingJunk}
                </dd>
              </Card>
            )}
            {audioShort > 0 && (
              <Card>
                <dt className="text-xs font-medium mb-1 text-[var(--text-muted)]">
                  {t("admin.seekIndex.statsAudioShort")}
                </dt>
                <dd className="text-xl font-bold text-[var(--text)]" data-testid="seek-index-audio-short-count">
                  {audioShort}
                </dd>
              </Card>
            )}
            {seekCoarse > 0 && (
              <Card>
                <dt className="text-xs font-medium mb-1 text-[var(--text-muted)]">
                  {t("admin.seekIndex.statsSeekCoarse")}
                </dt>
                <dd className="text-xl font-bold text-[var(--text)]" data-testid="seek-index-coarse-count">
                  {seekCoarse}
                </dd>
              </Card>
            )}
          </div>

          <div className="flex flex-wrap items-center justify-between gap-2">
            <Checkbox
              label={t("admin.seekIndex.onlyAttention")}
              checked={onlyAttention}
              onChange={setOnlyAttention}
            />
            <span className="text-xs text-[var(--text-muted)]" data-testid="seek-index-scanned-at">
              {t("admin.seekIndex.scannedAt", {
                date: new Date(Number(result.scannedAtMs)).toLocaleString(),
              })}
            </span>
          </div>

          {items.length === 0 ? (
            <div className="rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]">
              <EmptyState title={t("admin.seekIndex.noEbml")} />
            </div>
          ) : visible.length === 0 ? (
            <div className="rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]">
              <EmptyState title={t("admin.seekIndex.allGood")} />
            </div>
          ) : (
            <div className="overflow-x-auto rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]">
              <table className="w-full text-sm table-cards">
                <thead>
                  <tr className="border-b border-[var(--border)]">
                    <th className="text-left px-4 py-3 font-medium text-[var(--text-muted)]">
                      {t("admin.seekIndex.tableHeaders.file")}
                    </th>
                    <th className="text-left px-4 py-3 font-medium text-[var(--text-muted)]">
                      {t("admin.seekIndex.tableHeaders.container")}
                    </th>
                    <th className="text-left px-4 py-3 font-medium text-[var(--text-muted)]">
                      {t("admin.seekIndex.tableHeaders.size")}
                    </th>
                    <th className="text-left px-4 py-3 font-medium text-[var(--text-muted)]">
                      {t("admin.seekIndex.tableHeaders.status")}
                    </th>
                    <th className="px-4 py-3 w-44" />
                  </tr>
                </thead>
                <tbody>
                  {visible.map((it: V1SeekIndexItem) => (
                    <tr key={it.mediaId} className="border-b border-[var(--border)]" data-testid="seek-index-row">
                      <td className="px-4 py-3" data-label={t("admin.seekIndex.tableHeaders.file")}>
                        <div className="text-[var(--text)]">{it.title || it.path}</div>
                        <div
                          className="font-mono text-xs text-[var(--text-muted)] truncate max-w-[280px] sm:max-w-[480px]"
                          title={it.path}
                        >
                          {it.path}
                        </div>
                      </td>
                      <td className="px-4 py-3" data-label={t("admin.seekIndex.tableHeaders.container")}>
                        <Badge>{(it.container ?? "").toUpperCase()}</Badge>
                      </td>
                      <td className="px-4 py-3 whitespace-nowrap text-[var(--text-muted)]" data-label={t("admin.seekIndex.tableHeaders.size")}>
                        {formatSize(Number(it.sizeBytes ?? 0))}
                      </td>
                      <td className="px-4 py-3 whitespace-nowrap" data-label={t("admin.seekIndex.tableHeaders.status")}>
                        {it.hasIndex ? (
                          <Badge variant="success">{t("admin.seekIndex.statusIndexed")}</Badge>
                        ) : it.seekCoarse ? (
                          // Not "missing": the index exists and was rebuilt, it
                          // just cannot be finer than the encode's keyframes.
                          // Showing the repairable badge here promised a fix
                          // that no repair can deliver.
                          <span title={t("admin.seekIndex.seekCoarseHint")} className="inline-block">
                            <Badge variant="info">{t("admin.seekIndex.seekCoarseBadge")}</Badge>
                          </span>
                        ) : (
                          <Badge variant="warning">{t("admin.seekIndex.statusMissing")}</Badge>
                        )}
                        {it.hasBackup && (
                          <Badge variant="info" className="ml-1.5">
                            {t("admin.seekIndex.backupBadge")}
                          </Badge>
                        )}
                        {audioDelayed(it) && (
                          <Badge variant="warning" className="ml-1.5">
                            {t("admin.seekIndex.audioDelayBadge", {
                              sec: (it.audioDelaySec ?? 0).toFixed(1),
                            })}
                          </Badge>
                        )}
                        {it.truncated ? (
                          <span title={t("admin.seekIndex.truncatedHint")} className="ml-1.5 inline-block">
                            <Badge variant="error">
                              {Number(it.missingTailBytes ?? 0) > 0
                                ? t("admin.seekIndex.truncatedSizeBadge", {
                                    size: formatSize(Number(it.missingTailBytes ?? 0)),
                                  })
                                : t("admin.seekIndex.truncatedBadge")}
                            </Badge>
                          </span>
                        ) : (
                          Number(it.damagedBytes ?? 0) > 0 && (
                            <Badge variant="warning" className="ml-1.5">
                              {t("admin.seekIndex.damagedBadge", {
                                size: formatSize(Number(it.damagedBytes ?? 0)),
                              })}
                            </Badge>
                          )
                        )}
                        {it.wrongContainer && (
                          <span title={t("admin.seekIndex.wrongContainerHint")} className="ml-1.5 inline-block">
                            <Badge variant="error">{t("admin.seekIndex.wrongContainerBadge")}</Badge>
                          </span>
                        )}
                        {it.trailingJunk && (
                          <span title={t("admin.seekIndex.trailingJunkHint")} className="ml-1.5 inline-block">
                            <Badge variant="info">{t("admin.seekIndex.trailingJunkBadge")}</Badge>
                          </span>
                        )}
                        {(it.audioShortfallSec ?? 0) > 0 && (
                          <span
                            title={t("admin.seekIndex.audioShortHint", {
                              sec: Math.round(it.audioShortfallSec ?? 0),
                            })}
                            className="ml-1.5 inline-block"
                          >
                            <Badge variant="warning">
                              {t("admin.seekIndex.audioShortBadge", {
                                sec: Math.round(it.audioShortfallSec ?? 0),
                              })}
                            </Badge>
                          </span>
                        )}
                      </td>
                      <td className="px-4 py-3 whitespace-nowrap" data-label="">
                        <div className="flex items-center justify-end gap-2">
                          {audioDelayed(it) && (
                            <Button
                              variant="secondary"
                              className="text-xs"
                              disabled={busy}
                              loading={remuxAudioOne.isPending && remuxAudioOne.variables?.mediaId === it.mediaId}
                              onClick={() => it.mediaId && remuxAudioOne.mutate({ mediaId: it.mediaId })}
                              data-testid="seek-index-remux-audio-btn"
                            >
                              {t("admin.seekIndex.remuxAudioBtn")}
                            </Button>
                          )}
                          {/* No repair exists for a truncated file - the tail is
                              simply absent, a real attempt always fails a verify
                              step - nor for a coarse one, whose index was already
                              rebuilt and cannot beat the encode's keyframes. No
                              button is offered (the badge's tooltip says why);
                              the batch's needsReindex skips both for the same
                              reason. */}
                          {!it.hasIndex && !it.truncated && !it.seekCoarse && (
                            <Button
                              variant="secondary"
                              className="text-xs"
                              disabled={busy}
                              loading={
                                repairOne.isPending &&
                                repairOne.variables?.mediaId === it.mediaId &&
                                !repairOne.variables?.data?.resync
                              }
                              title={t("admin.seekIndex.repairHint")}
                              onClick={() => it.mediaId && repairOne.mutate({ mediaId: it.mediaId, data: { mode: "auto" } })}
                              data-testid="seek-index-repair-btn"
                            >
                              {t("admin.seekIndex.repair")}
                            </Button>
                          )}
                          {!it.hasIndex && !it.truncated && !it.seekCoarse && (
                            <Button
                              variant="ghost"
                              className="text-xs"
                              disabled={busy}
                              loading={
                                repairOne.isPending &&
                                repairOne.variables?.mediaId === it.mediaId &&
                                repairOne.variables?.data?.resync === true
                              }
                              title={t("admin.seekIndex.repairResyncHint")}
                              onClick={() => it.mediaId && repairOne.mutate({ mediaId: it.mediaId, data: { resync: true } })}
                              data-testid="seek-index-repair-resync-btn"
                            >
                              {t("admin.seekIndex.repairResync")}
                            </Button>
                          )}
                          {it.hasBackup && (
                            <Button
                              variant="ghost"
                              className="text-xs"
                              disabled={busy}
                              loading={rollbackOne.isPending && rollbackOne.variables?.mediaId === it.mediaId}
                              onClick={() => it.mediaId && rollbackOne.mutate({ mediaId: it.mediaId })}
                              data-testid="seek-index-rollback-btn"
                            >
                              {t("admin.seekIndex.rollback")}
                            </Button>
                          )}
                          {it.hasBackup && (
                            <Button
                              variant="ghost"
                              className="text-xs"
                              disabled={deleteBackup.isPending}
                              loading={deleteBackup.isPending && deleteBackup.variables?.data?.mediaId === it.mediaId}
                              onClick={() => it.mediaId && deleteBackup.mutate({ data: { mediaId: it.mediaId } })}
                              data-testid="seek-index-delete-backup-btn"
                            >
                              {t("admin.seekIndex.deleteBackup")}
                            </Button>
                          )}
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </>
      )}

      <Modal
        open={confirmBatch !== null}
        onClose={() => setConfirmBatch(null)}
        title={
          confirmBatch === "rollback"
            ? t("admin.seekIndex.rollbackAllConfirmTitle")
            : confirmBatch === "purge"
              ? t("admin.seekIndex.purgeAllConfirmTitle")
              : t("admin.seekIndex.repairAllConfirmTitle")
        }
      >
        <p className="text-sm text-[var(--text-muted)] mb-4">
          {confirmBatch === "rollback"
            ? t("admin.seekIndex.rollbackAllConfirm", { count: withBackup })
            : confirmBatch === "purge"
              ? t("admin.seekIndex.purgeAllConfirm", { count: withBackup })
              : t("admin.seekIndex.repairAllConfirm", { count: repairable })}
        </p>
        {confirmBatch === "repair" && repairVolumes.length > 0 && (
          <ul className="mb-4 flex flex-col gap-1" data-testid="seek-index-volumes">
            {repairVolumes.map((v) => (
              <li key={v.root} className="text-xs text-[var(--text-muted)]">
                {t("admin.seekIndex.volumeLine", {
                  root: v.root,
                  free: formatSize(Number(v.freeBytes ?? 0)),
                  missing: formatSize(Number(v.missingBytes ?? 0)),
                  count: v.missingFiles ?? 0,
                })}
                {volumeBackupsRisky(v) && (
                  <span className="text-[var(--color-warning)]">
                    {" "}
                    - {t("admin.seekIndex.volumeBackupsRisky")}
                  </span>
                )}
                {volumeTempRisky(v) && (
                  <span className="text-[var(--color-danger)]">
                    {" "}
                    - {t("admin.seekIndex.volumeTempRisky", {
                      largest: formatSize(Number(v.largestMissingBytes ?? 0)),
                    })}
                  </span>
                )}
              </li>
            ))}
          </ul>
        )}
        {confirmBatch === "repair" && (
          <div className="mb-4 flex flex-col gap-3">
            {delayedAudio > 0 && (
              <div>
                <Checkbox
                  label={t("admin.seekIndex.alsoAudio", { count: delayedAudio })}
                  checked={alsoAudio}
                  onChange={setAlsoAudio}
                  data-testid="seek-index-also-audio"
                />
                <p className="mt-1 text-xs text-muted-foreground">
                  {t("admin.seekIndex.alsoAudioHint")}
                </p>
              </div>
            )}
            <div>
              <Checkbox
                label={t("admin.seekIndex.keepBackups")}
                checked={keepBackups}
                onChange={setKeepBackups}
                data-testid="seek-index-keep-backups"
              />
              <p className="mt-1 text-xs text-muted-foreground">
                {keepBackups
                  ? t("admin.seekIndex.keepBackupsHint")
                  : t("admin.seekIndex.keepBackupsOffHint", { n: BACKUP_WINDOW })}
              </p>
            </div>
          </div>
        )}
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={() => setConfirmBatch(null)}>
            {t("common.cancel")}
          </Button>
          <Button
            loading={repairLib.isPending || rollbackLib.isPending || purgeLib.isPending}
            onClick={() => {
              if (confirmBatch === "rollback") rollbackLib.mutate({ data: libraryId ? { libraryId } : {} });
              else if (confirmBatch === "purge") purgeLib.mutate({ data: libraryId ? { libraryId } : {} });
              else
                // Strategy fixed to auto; when backups are not kept, a rolling
                // window (drained at job end) protects the run and deep-verify
                // proves each repair in place of the lost post-run rollback.
                repairLib.mutate({
                  data: {
                    ...(libraryId ? { libraryId } : {}),
                    mode: "auto",
                    includeAudio: alsoAudio,
                    deepVerify: !keepBackups,
                    ...(keepBackups ? {} : { backupWindow: BACKUP_WINDOW }),
                  },
                });
              setConfirmBatch(null);
            }}
            data-testid="seek-index-confirm-batch-btn"
          >
            {t("common.confirm")}
          </Button>
        </div>
      </Modal>
    </div>
  );
}
