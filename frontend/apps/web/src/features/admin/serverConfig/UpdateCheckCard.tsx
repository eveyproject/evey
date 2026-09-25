import { useTranslation } from "react-i18next";
import { RefreshCw } from "lucide-react";
import { latestLabel, useCheckUpdateNow, useUpdateStatus, type UpdateStatus } from "@evey/client";
import { Button } from "@/components/ui/Button";
import { Checkbox } from "@/components/ui/Checkbox";
import { ICON } from "@/constants/iconSizes";
import { useSetConfigEntry, type ConfigEntry } from "./hooks";

const ENABLED_KEY = "update_check.enabled";

export function updateStatusLine(st: UpdateStatus | undefined, t: (k: string, o?: Record<string, unknown>) => string): string {
  if (!st) return "";
  switch (st.state) {
    case "update_available":
      return t("admin.update.available", { latest: latestLabel(st) });
    case "up_to_date":
      return t("admin.update.upToDate");
    case "disabled":
      return t("admin.update.disabled");
    case "pending":
      return t("admin.update.pending");
    default:
      return t("admin.update.unknown", { reason: st.reason ?? "" });
  }
}

// The switch for the periodic "newer image published?" check and its last
// answer. Evey only reads the registry; it never updates itself.
export function UpdateCheckCard({ entries }: { entries: ConfigEntry[] | undefined }) {
  const { t } = useTranslation();
  const setEntry = useSetConfigEntry();
  const { data } = useUpdateStatus();
  const checkNow = useCheckUpdateNow();
  const enabled = entries?.find((e) => e.key === ENABLED_KEY)?.value !== "false";

  return (
    <div className="flex flex-col gap-3" data-testid="update-check-card">
      <Checkbox
        label={t("admin.update.enabled")}
        checked={enabled}
        onChange={(v) => setEntry.mutate({ key: ENABLED_KEY, value: v ? "true" : "false" })}
        disabled={setEntry.isPending}
      />
      <p className="text-xs text-[var(--text-muted)]">{t("admin.update.noAuto")}</p>
      <div className="flex flex-wrap items-center gap-3">
        <span className="text-sm text-[var(--text)]" data-testid="update-status-line">{updateStatusLine(data, t)}</span>
        {data?.checkedAt && (
          <span className="text-xs text-[var(--text-muted)]">
            {t("admin.update.checkedAt", { when: new Date(data.checkedAt).toLocaleString() })}
          </span>
        )}
        <Button variant="secondary" loading={checkNow.isPending} disabled={!enabled} onClick={() => checkNow.mutate()}>
          <RefreshCw size={ICON.xs} aria-hidden="true" />
          {t("admin.update.checkNow")}
        </Button>
      </div>
    </div>
  );
}
