import { useTranslation } from "react-i18next";
import { RefreshCw } from "lucide-react";
import { latestLabel, useCheckUpdateNow, useUpdateStatus, type UpdateStatus } from "@evey/client";
import { Button } from "@/components/ui/Button";
import { Switch } from "@/components/ui/Switch";
import { SettingRow } from "@/components/ui/SettingRow";
import { ICON } from "@/constants/iconSizes";
import { useDemoLockFor } from "@/hooks/useDemoMode";
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
  const { lock } = useDemoLockFor("POST /api/v1/admin/update-status/check");
  const enabled = entries?.find((e) => e.key === ENABLED_KEY)?.value !== "false";

  return (
    <div className="flex flex-col divide-y divide-[var(--border)]" data-testid="update-check-card">
      <SettingRow label={t("admin.update.enabled")} info={t("admin.update.noAuto")} hint={t("admin.update.desc")} kind="toggle">
        <Switch
          label={t("admin.update.enabled")}
          checked={enabled}
          onChange={() => setEntry.mutate({ key: ENABLED_KEY, value: enabled ? "false" : "true" })}
          disabled={setEntry.isPending}
          data-testid="update-check-toggle"
        />
      </SettingRow>
      <div className="flex flex-wrap items-center gap-3 py-3 last:pb-0">
        <span className="text-sm text-[var(--text)]" data-testid="update-status-line">{updateStatusLine(data, t)}</span>
        {data?.checkedAt && (
          <span className="text-xs text-[var(--text-muted)]">
            {t("admin.update.checkedAt", { when: new Date(data.checkedAt).toLocaleString() })}
          </span>
        )}
        <Button variant="secondary" loading={checkNow.isPending} disabled={!enabled} onClick={() => checkNow.mutate()} className="sm:ml-auto" {...lock}>
          <RefreshCw size={ICON.xs} aria-hidden="true" />
          {t("admin.update.checkNow")}
        </Button>
      </div>
    </div>
  );
}
