import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Switch } from "@/components/ui/Switch";
import { BACKGROUND_JOB_RATES, BACKGROUND_JOB_TOGGLES, useBackgroundJobsForm } from "@evey/client";
import { useSetConfigEntry, useDeleteConfigEntry, type ConfigEntry } from "./hooks";

export function BackgroundJobsCard({ entries }: { entries: ConfigEntry[] | undefined }) {
  const { t } = useTranslation();
  const setEntry = useSetConfigEntry();
  const deleteEntry = useDeleteConfigEntry();
  const quietSet = useSetConfigEntry({ ownToast: true });
  const quietDelete = useDeleteConfigEntry({ ownToast: true });

  const { rateDrafts, rateDirty, setRate, isEnabled, toggle, saveRates } = useBackgroundJobsForm({
    entries,
    save: (key, value) => setEntry.mutateAsync({ key, value }),
    remove: (key) => deleteEntry.mutateAsync(key),
    quietSave: (key, value) => quietSet.mutateAsync({ key, value }),
    quietRemove: (key) => quietDelete.mutateAsync(key),
    onRatesSaved: (failed) =>
      failed.length === 0 ? toast.success(t("admin.serverConfig.configUpdated")) : toast.error(t("admin.serverConfig.failedUpdate")),
  });

  return (
    <div className="flex flex-col gap-4">
      <div className="rounded-[var(--radius-md)] border border-[color-mix(in_srgb,var(--color-warning)_40%,transparent)] bg-[color-mix(in_srgb,var(--color-warning)_5%,transparent)] p-3">
        <p className="text-sm font-medium text-[var(--color-warning)]">
          {t("admin.serverConfig.backgroundJobs.restartWarning")}
        </p>
      </div>

      <div className="flex flex-col gap-3">
        {BACKGROUND_JOB_TOGGLES.map((field) => (
          <div key={field.key} className="flex flex-col gap-1">
            <div className="flex items-center gap-3">
              <span className="text-sm font-medium text-[var(--text)]">{t(field.labelKey)}</span>
              <Switch
                label={t(field.labelKey)}
                checked={isEnabled(field.key)}
                onChange={() => toggle(field.key, !isEnabled(field.key))}
                disabled={setEntry.isPending || deleteEntry.isPending}
              />
            </div>
            <p className="text-xs text-[var(--text-muted)]">{t(field.descKey)}</p>
          </div>
        ))}
      </div>

      <div className="flex flex-col gap-3 pt-2 border-t border-[var(--border)]">
        <p className="text-sm font-medium text-[var(--text)]">
          {t("admin.serverConfig.backgroundJobs.paceTitle")}
        </p>
        <p className="text-xs text-[var(--text-muted)]">
          {t("admin.serverConfig.backgroundJobs.paceDesc")}
        </p>
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
          {BACKGROUND_JOB_RATES.map((field) => (
            <Input
              key={field.key}
              label={t(field.labelKey)}
              type="number"
              min={0}
              placeholder={t("admin.serverConfig.backgroundJobs.paceDefaultPlaceholder")}
              value={rateDrafts[field.key] ?? ""}
              onChange={(e) => setRate(field.key, e.target.value)}
            />
          ))}
        </div>
        <div className="flex justify-end">
          <Button onClick={saveRates} loading={quietSet.isPending || quietDelete.isPending} disabled={!rateDirty}>
            {t("common.save")}
          </Button>
        </div>
      </div>

      {(setEntry.isPending || deleteEntry.isPending) && <Spinner size="sm" />}
    </div>
  );
}
