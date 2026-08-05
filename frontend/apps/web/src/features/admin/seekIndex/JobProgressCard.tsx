import { useTranslation } from "react-i18next";
import type { V1GetSeekIndexJobResponse } from "@evey/api";
import { Spinner } from "@/components/ui/Spinner";
import { Card } from "@/components/ui/Card";
import { SEEK_INDEX_KIND_KEYS, baseName } from "@evey/api";

export function JobProgressCard({ job }: { job: V1GetSeekIndexJobResponse }) {
  const { t } = useTranslation();
  const done = job.done ?? 0;
  const total = job.total ?? 0;
  const failed = job.failed ?? 0;
  const pct = total > 0 ? Math.min(100, Math.round((done / total) * 100)) : 0;
  const kind = t(SEEK_INDEX_KIND_KEYS[job.kind ?? ""] ?? SEEK_INDEX_KIND_KEYS.scan);
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
