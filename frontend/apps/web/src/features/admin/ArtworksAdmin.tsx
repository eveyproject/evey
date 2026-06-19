import { useState, useEffect, useRef } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Download, CheckCircle, RefreshCw, Image as ImageIcon, Trash2 } from "lucide-react";
import { eveyFetch } from "@evey/api";
import { toast } from "@evey/store";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { PageHeader } from "@/components/ui/PageHeader";

const STATS_QUERY_KEY = ["admin", "artworks", "stats"];

interface ArtworkStats {
  totalRecords: number;
  withLocalPath: number;
  missingLocalPath: number;
  totalSizeBytes: number;
}

interface JobStatus {
  status: "running" | "done" | "failed";
  processed: number;
  total: number;
  errors: string[];
}

function useArtworkStats() {
  return useQuery<ArtworkStats>({
    queryKey: STATS_QUERY_KEY,
    queryFn: () =>
      eveyFetch<ArtworkStats>({
        url: "/api/v1/admin/artworks/stats",
        method: "GET",
      }),
    staleTime: 30_000,
  });
}

function useStartDownload() {
  return useMutation<{ jobId: string; totalToDownload: number }, Error>({
    mutationFn: () =>
      eveyFetch<{ jobId: string; totalToDownload: number }>({
        url: "/api/v1/admin/artworks/download-all",
        method: "POST",
      }),
  });
}

// useBackfillBlurhashes drives POST /api/v1/admin/artworks/backfill-blurhashes.
// The server runs it as a pollable background job (decode can take minutes on a
// large catalogue) and returns {jobId,total}; the UI polls progress through the
// same status route as download-all.
function useBackfillBlurhashes() {
  return useMutation<{ jobId: string; total: number }, Error>({
    mutationFn: () =>
      eveyFetch<{ jobId: string; total: number }>({
        url: "/api/v1/admin/artworks/backfill-blurhashes",
        method: "POST",
      }),
  });
}

// usePurgeVariants drives POST /api/v1/admin/artworks/purge-variants, deleting
// 0-byte resized variants (<base>_w<width>.webp) left behind by failed resizes.
// The server walks the artwork tree inline (stat-only, fast) and returns the
// number of files removed.
function usePurgeVariants() {
  return useMutation<{ removed: number }, Error>({
    mutationFn: () =>
      eveyFetch<{ removed: number }>({
        url: "/api/v1/admin/artworks/purge-variants",
        method: "POST",
      }),
  });
}

function useJobStatus(jobId: string | null) {
  return useQuery<JobStatus>({
    queryKey: ["admin", "artworks", "job", jobId],
    queryFn: () =>
      eveyFetch<JobStatus>({
        url: `/api/v1/admin/artworks/download-all/${jobId}`,
        method: "GET",
      }),
    enabled: !!jobId,
    refetchInterval: (query) => {
      const s = query.state.data?.status;
      // Stop polling once the job reaches a terminal state. "failed" was missing,
      // so a failed job was re-fetched every 2s forever with no admin feedback.
      if (s === "done" || s === "failed") return false;
      return 2000;
    },
    staleTime: 0,
  });
}

function formatPct(n: number, total: number): string {
  if (total === 0) return "0%";
  return `${Math.round((n / total) * 100)}%`;
}

export function ArtworksAdmin() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [jobId, setJobId] = useState<string | null>(null);
  const prevDoneRef = useRef(false);

  const { data: stats, isLoading: statsLoading } = useArtworkStats();
  const startDownload = useStartDownload();
  const backfillBlurhashes = useBackfillBlurhashes();
  const purgeVariants = usePurgeVariants();
  const { data: job } = useJobStatus(jobId);

  function handlePurgeVariants() {
    purgeVariants.mutate(undefined, {
      onSuccess: (data) => {
        toast.success(
          t("admin.artworks.purgeVariantsSuccess", { count: data.removed }),
        );
      },
      onError: () => {
        toast.error(t("admin.artworks.purgeVariantsFailed"));
      },
    });
  }

  function handleBackfillBlurhashes() {
    prevDoneRef.current = false;
    backfillBlurhashes.mutate(undefined, {
      onSuccess: (data) => {
        setJobId(data.jobId);
      },
      onError: () => {
        toast.error(t("admin.artworks.backfillBlurhashesFailed"));
      },
    });
  }

  // When the job reaches a terminal state, refresh stats and notify once.
  useEffect(() => {
    if (!job || prevDoneRef.current) return;
    if (job.status === "done") {
      prevDoneRef.current = true;
      qc.invalidateQueries({ queryKey: STATS_QUERY_KEY });
      toast.success(t("admin.artworks.downloadComplete"));
    } else if (job.status === "failed") {
      prevDoneRef.current = true;
      toast.error(t("admin.artworks.downloadJobFailed"));
    }
  }, [job, qc, t]);

  function handleStartDownload() {
    prevDoneRef.current = false;
    startDownload.mutate(undefined, {
      onSuccess: (data) => {
        setJobId(data.jobId);
      },
      onError: () => {
        toast.error(t("admin.artworks.downloadFailed"));
      },
    });
  }

  const isRunning = !!jobId && job?.status === "running";
  const isDone = job?.status === "done";

  return (
    <div className="max-w-2xl mx-auto">
      <PageHeader
        title={t("admin.artworks.title")}
        subtitle={t("admin.artworks.subtitle")}
      />

      {/* Stats card */}
      <Card className="mb-6 p-5">
        <h2 className="font-semibold text-[var(--text)] mb-4">
          {t("admin.artworks.cacheStatus")}
        </h2>

        {statsLoading ? (
          <div className="flex justify-center py-4">
            <Spinner />
          </div>
        ) : stats ? (
          <div className="flex flex-col gap-3">
            {/* Coverage bar */}
            <div>
              <div className="flex justify-between text-sm text-[var(--text-muted)] mb-1">
                <span>
                  {stats.withLocalPath} / {stats.totalRecords}{" "}
                  {t("admin.artworks.storedLocally")}
                </span>
                <span>{formatPct(stats.withLocalPath, stats.totalRecords)}</span>
              </div>
              <div className="h-2 rounded-full bg-[var(--surface-raised)] overflow-hidden">
                <div
                  className="h-full rounded-full transition-[width] duration-500"
                  style={{
                    width: formatPct(stats.withLocalPath, stats.totalRecords),
                    backgroundColor: "var(--accent)",
                  }}
                />
              </div>
            </div>

            <div className="grid grid-cols-2 gap-3 text-sm mt-1">
              <div className="rounded-[var(--radius-sm)] bg-[var(--surface-raised)] p-3">
                <div className="text-[var(--text-muted)] text-xs mb-0.5">
                  {t("admin.artworks.totalRecords")}
                </div>
                <div className="font-semibold text-[var(--text)]">
                  {stats.totalRecords}
                </div>
              </div>
              <div className="rounded-[var(--radius-sm)] bg-[var(--surface-raised)] p-3">
                <div className="text-[var(--text-muted)] text-xs mb-0.5">
                  {t("admin.artworks.missing")}
                </div>
                <div className="font-semibold text-[var(--color-danger,var(--accent))]">
                  {stats.missingLocalPath}
                </div>
              </div>
            </div>
          </div>
        ) : (
          <p className="text-sm text-[var(--text-muted)]">
            {t("admin.artworks.statsUnavailable")}
          </p>
        )}
      </Card>

      {/* Download section */}
      <Card className="p-5">
        <h2 className="font-semibold text-[var(--text)] mb-2">
          {t("admin.artworks.downloadSection")}
        </h2>
        <p className="text-sm text-[var(--text-muted)] mb-4">
          {t("admin.artworks.downloadDesc")}
        </p>

        <Button
          onClick={handleStartDownload}
          disabled={isRunning || startDownload.isPending}
          className="flex items-center gap-2"
        >
          {isRunning ? (
            <RefreshCw size={16} className="animate-spin" />
          ) : isDone ? (
            <CheckCircle size={16} />
          ) : (
            <Download size={16} />
          )}
          {isRunning
            ? t("admin.artworks.downloading")
            : isDone
              ? t("admin.artworks.downloadAgain")
              : t("admin.artworks.downloadMissing")}
        </Button>

        {/* Progress bar */}
        {jobId && job && (
          <div className="mt-4">
            <div className="flex justify-between text-xs text-[var(--text-muted)] mb-1">
              <span>
                {job.processed} / {job.total}
              </span>
              <span>
                {job.status === "done"
                  ? t("admin.artworks.done")
                  : formatPct(job.processed, job.total)}
              </span>
            </div>
            <div className="h-2 rounded-full bg-[var(--surface-raised)] overflow-hidden">
              <div
                className="h-full rounded-full transition-[width] duration-300"
                style={{
                  width: job.total > 0 ? formatPct(job.processed, job.total) : "0%",
                  backgroundColor:
                    job.status === "done"
                      ? "var(--color-success, var(--accent))"
                      : "var(--accent)",
                }}
              />
            </div>

            {job.errors.length > 0 && (
              <details className="mt-2">
                <summary className="text-xs text-[var(--color-warning)] cursor-pointer select-none">
                  {job.errors.length} {t("admin.artworks.errorsPartial")}
                </summary>
                <ul className="mt-1 text-xs text-[var(--text-muted)] font-mono space-y-0.5 max-h-32 overflow-y-auto">
                  {job.errors.slice(0, 20).map((e, i) => (
                    <li key={i} className="truncate">{e}</li>
                  ))}
                </ul>
              </details>
            )}
          </div>
        )}
      </Card>

      {/* Blurhash backfill — retro-fits the placeholder hash onto artworks
          ingested before the blurhash column existed. Sync call; the button
          stays disabled while the request is in flight, and the toast names
          succeeded/failed so the admin can verify the pass landed. */}
      <Card className="p-5 mt-6">
        <h2 className="font-semibold text-[var(--text)] mb-2">
          {t("admin.artworks.backfillBlurhashesSection")}
        </h2>
        <p className="text-sm text-[var(--text-muted)] mb-4">
          {t("admin.artworks.backfillBlurhashesDesc")}
        </p>
        <Button
          onClick={handleBackfillBlurhashes}
          disabled={backfillBlurhashes.isPending}
          className="flex items-center gap-2"
        >
          {backfillBlurhashes.isPending ? (
            <RefreshCw size={16} className="animate-spin" />
          ) : (
            <ImageIcon size={16} />
          )}
          {backfillBlurhashes.isPending
            ? t("admin.artworks.backfillBlurhashesRunning")
            : t("admin.artworks.backfillBlurhashesRun")}
        </Button>
      </Card>

      {/* Purge corrupt variants — deletes 0-byte resized posters left by a
          failed/timed-out resize. The serving path regenerates them on demand,
          so this is a bulk-clear convenience rather than a fix; the toast names
          how many files were removed. */}
      <Card className="p-5 mt-6">
        <h2 className="font-semibold text-[var(--text)] mb-2">
          {t("admin.artworks.purgeVariantsSection")}
        </h2>
        <p className="text-sm text-[var(--text-muted)] mb-4">
          {t("admin.artworks.purgeVariantsDesc")}
        </p>
        <Button
          variant="secondary"
          onClick={handlePurgeVariants}
          disabled={purgeVariants.isPending}
          className="flex items-center gap-2"
        >
          {purgeVariants.isPending ? (
            <RefreshCw size={16} className="animate-spin" />
          ) : (
            <Trash2 size={16} />
          )}
          {purgeVariants.isPending
            ? t("admin.artworks.purgeVariantsRunning")
            : t("admin.artworks.purgeVariantsRun")}
        </Button>
      </Card>
    </div>
  );
}
