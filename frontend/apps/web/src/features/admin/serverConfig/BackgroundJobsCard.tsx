import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Switch } from "@/components/ui/Switch";
import { SettingRow } from "@/components/ui/SettingRow";
import { BACKGROUND_JOB_RATES, BACKGROUND_JOB_TOGGLES, useBackgroundJobsForm } from "@evey/client";
import { useSetConfigEntry, useDeleteConfigEntry, type ConfigEntry } from "./hooks";

// paceInfoKey picks the explanation of a pacing field by what it measures:
// the batch size, the pause between batches, or the wait once caught up.
function paceInfoKey(key: string): string {
  if (key.endsWith("_batch")) return "admin.settingsHelp.paceBatch";
  if (key.endsWith("_pause_ms")) return "admin.settingsHelp.pacePause";
  return "admin.settingsHelp.paceIdle";
}

// BackgroundJobsCard holds the background workers' switches, each read at
// boot (hence the restart badge on every row), and their pacing numbers
// tucked behind an "Advanced" disclosure: most servers never touch them.
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
    <div className="flex flex-col divide-y divide-[var(--border)]">
      <p className="pb-3 text-xs text-[var(--text-muted)]">{t("admin.serverConfig.backgroundJobs.restartWarning")}</p>

      {BACKGROUND_JOB_TOGGLES.map((field) => (
        <SettingRow key={field.key} label={t(field.labelKey)} info={t(field.descKey)} restart kind="toggle">
          <Switch
            label={t(field.labelKey)}
            checked={isEnabled(field.key)}
            onChange={() => toggle(field.key, !isEnabled(field.key))}
            disabled={setEntry.isPending || deleteEntry.isPending}
          />
        </SettingRow>
      ))}

      <details className="group py-3 last:pb-0" data-testid="background-pacing">
        <summary className="cursor-pointer select-none text-sm font-medium text-[var(--text)]">
          {t("admin.settingsLayout.advancedPacing")}
        </summary>
        <p className="mt-2 text-xs text-[var(--text-muted)]">{t("admin.serverConfig.backgroundJobs.paceDesc")}</p>
        <div className="mt-3 flex flex-col divide-y divide-[var(--border)]">
          {BACKGROUND_JOB_RATES.map((field) => {
            const id = `pace-${field.key}`;
            return (
              <SettingRow key={field.key} label={t(field.labelKey)} htmlFor={id} info={t(paceInfoKey(field.key))} restart>
                <Input
                  id={id}
                  className="w-full"
                  type="number"
                  min={0}
                  placeholder={t("admin.serverConfig.backgroundJobs.paceDefaultPlaceholder")}
                  value={rateDrafts[field.key] ?? ""}
                  onChange={(e) => setRate(field.key, e.target.value)}
                />
              </SettingRow>
            );
          })}
        </div>
        <div className="flex justify-end pt-3">
          <Button onClick={saveRates} loading={quietSet.isPending || quietDelete.isPending} disabled={!rateDirty}>
            {t("common.save")}
          </Button>
        </div>
      </details>

      {(setEntry.isPending || deleteEntry.isPending) && <div className="pt-3"><Spinner size="sm" /></div>}
    </div>
  );
}
