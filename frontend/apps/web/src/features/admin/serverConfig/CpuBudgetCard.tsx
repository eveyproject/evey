import { useTranslation } from "react-i18next";
import { Spinner } from "@/components/ui/Spinner";
import { SegmentedControl } from "@/components/ui/SegmentedControl";
import { SettingRow } from "@/components/ui/SettingRow";
import {
  CPU_BUDGET_GLOBAL_CHOICES,
  CPU_BUDGET_TASK_CHOICES,
  CPU_BUDGET_TASKS,
  useCpuBudgetForm,
} from "@evey/client";
import { useSetConfigEntry, useDeleteConfigEntry, type ConfigEntry } from "./hooks";

// CpuBudgetCard is where the admin bounds the CPU the background tasks take
// together: the global share (25 % gentle, 50 % boost) and, per heavy task,
// a share of its own up to 80 %. The list and the key logic are
// @evey/client's (useCpuBudgetForm), shared with the phone.
//
// Every choice is a segmented control of equal cells: "follow the global
// share" and "25 %" take the same width, so the rows line up.
export function CpuBudgetCard({ entries }: { entries: ConfigEntry[] | undefined }) {
  const { t } = useTranslation();
  const setEntry = useSetConfigEntry();
  const deleteEntry = useDeleteConfigEntry();
  const { globalPercent, taskPercent, setGlobal, setTask } = useCpuBudgetForm(entries, {
    save: (key, value) => setEntry.mutate({ key, value }),
    remove: (key) => deleteEntry.mutate(key),
  });
  const busy = setEntry.isPending || deleteEntry.isPending;
  const globalLabel = t("admin.serverConfig.cpuBudget.globalLabel");

  return (
    <div className="flex flex-col divide-y divide-[var(--border)]">
      <SettingRow label={globalLabel} info={t("admin.serverConfig.cpuBudget.globalDesc")} hint={t("admin.serverConfig.cpuBudget.applyHint")} kind="wide">
        <SegmentedControl
          id="cpu-budget-global"
          label={globalLabel}
          value={String(globalPercent)}
          onChange={(v) => setGlobal(Number(v))}
          disabled={busy}
          options={CPU_BUDGET_GLOBAL_CHOICES.map((pct) => ({
            value: String(pct),
            label: t(`admin.serverConfig.cpuBudget.global${pct}`),
          }))}
        />
      </SettingRow>

      <div className="py-3">
        <p className="flex items-center gap-1.5 text-sm font-semibold text-[var(--text)]">
          {t("admin.serverConfig.cpuBudget.tasksTitle")}
        </p>
        <p className="mt-0.5 text-xs text-[var(--text-muted)]">{t("admin.serverConfig.cpuBudget.tasksDesc")}</p>
      </div>
      {CPU_BUDGET_TASKS.map((field) => {
        const label = t(field.labelKey);
        return (
          <SettingRow key={field.task} label={label} info={t(field.descKey)} kind="wide" data-testid={`cpu-budget-row-${field.task}`}>
            <SegmentedControl
              id={`cpu-budget-${field.task}`}
              label={label}
              value={taskPercent[field.task] === null ? "" : String(taskPercent[field.task])}
              onChange={(v) => setTask(field.task, v === "" ? null : Number(v))}
              disabled={busy}
              options={[
                { value: "", label: t("admin.serverConfig.cpuBudget.followGlobal", { percent: globalPercent }) },
                ...CPU_BUDGET_TASK_CHOICES.map((pct) => ({
                  value: String(pct),
                  label: t("admin.serverConfig.cpuBudget.share", { percent: pct }),
                })),
              ]}
            />
          </SettingRow>
        );
      })}
      {busy && <div className="pt-3"><Spinner size="sm" /></div>}
    </div>
  );
}
