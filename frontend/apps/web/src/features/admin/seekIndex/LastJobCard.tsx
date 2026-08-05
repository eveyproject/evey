import { useTranslation } from "react-i18next";
import type { V1GetSeekIndexJobResponse, V1SeekIndexJobFailure } from "@evey/api";
import { Badge } from "@/components/ui/Badge";
import { Card } from "@/components/ui/Card";
import { SEEK_INDEX_KIND_KEYS, baseName } from "@evey/api";

export function LastJobCard({ last }: { last: NonNullable<V1GetSeekIndexJobResponse["last"]> }) {
  const { t } = useTranslation();
  const kind = t(SEEK_INDEX_KIND_KEYS[last.kind ?? ""] ?? SEEK_INDEX_KIND_KEYS.scan);
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
