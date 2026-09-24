import { useTranslation } from "react-i18next";
import { Spinner } from "@/components/ui/Spinner";
import { Select } from "@/components/ui/Select";
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
export function CpuBudgetCard({ entries }: { entries: ConfigEntry[] | undefined }) {
  const { t } = useTranslation();
  const setEntry = useSetConfigEntry();
  const deleteEntry = useDeleteConfigEntry();
  const { globalPercent, taskPercent, setGlobal, setTask } = useCpuBudgetForm(entries, {
    save: (key, value) => setEntry.mutate({ key, value }),
    remove: (key) => deleteEntry.mutate(key),
  });
  const busy = setEntry.isPending || deleteEntry.isPending;

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-1">
        <Select
          id="cpu-budget-global"
          label={t("admin.serverConfig.cpuBudget.globalLabel")}
          value={String(globalPercent)}
          onChange={(v) => setGlobal(Number(v))}
          disabled={busy}
          options={CPU_BUDGET_GLOBAL_CHOICES.map((pct) => ({
            value: String(pct),
            label: t(`admin.serverConfig.cpuBudget.global${pct}`),
          }))}
        />
        <p className="text-xs text-[var(--text-muted)]">{t("admin.serverConfig.cpuBudget.globalDesc")}</p>
      </div>

      <div className="flex flex-col gap-3 border-t border-[var(--border)] pt-3">
        <p className="text-sm font-medium text-[var(--text)]">{t("admin.serverConfig.cpuBudget.tasksTitle")}</p>
        <p className="text-xs text-[var(--text-muted)]">{t("admin.serverConfig.cpuBudget.tasksDesc")}</p>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          {CPU_BUDGET_TASKS.map((field) => (
            <div key={field.task} className="flex flex-col gap-1">
              <Select
                id={`cpu-budget-${field.task}`}
                label={t(field.labelKey)}
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
              <p className="text-xs text-[var(--text-muted)]">{t(field.descKey)}</p>
            </div>
          ))}
        </div>
        <p className="text-xs text-[var(--text-muted)]">{t("admin.serverConfig.cpuBudget.applyHint")}</p>
      </div>
      {busy && <Spinner size="sm" />}
    </div>
  );
}
