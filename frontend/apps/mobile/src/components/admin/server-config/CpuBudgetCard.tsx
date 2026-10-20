import { View, Text } from "react-native";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../../providers/ThemeProvider";
import { SettingRow } from "../settings/SettingRow";
import { ChipSelect } from "./ChipSelect";
import {
  CPU_BUDGET_GLOBAL_CHOICES,
  CPU_BUDGET_TASK_CHOICES,
  CPU_BUDGET_TASKS,
  useCpuBudgetForm,
} from "@evey/client";
import type { ConfigEntry, SaveEntry } from "./types";

interface CpuBudgetCardProps {
  entries: ConfigEntry[] | undefined;
  saveEntry: SaveEntry;
  deleteEntry: (key: string) => void;
  saving: boolean;
}

// The background CPU budget, same list and key logic as the web card
// (@evey/client useCpuBudgetForm): global share, then one chip row per heavy
// task with "follow the global share" as the first chip.
export function CpuBudgetCard({ entries, saveEntry, deleteEntry, saving }: CpuBudgetCardProps) {
  const theme = useTheme();
  const { t } = useTranslation();
  const globalLabel = t("admin.serverConfig.cpuBudget.globalLabel", "Global share");
  const { globalPercent, taskPercent, setGlobal, setTask } = useCpuBudgetForm(entries, {
    save: (key, value) => saveEntry({ key, value }),
    remove: (key) => deleteEntry(key),
  });

  return (
    <View>
      <SettingRow
        label={globalLabel}
        info={t("admin.serverConfig.cpuBudget.globalDesc")}
        hint={t("admin.serverConfig.cpuBudget.applyHint")}
        kind="wide"
        testID="cpu-budget-global"
      >
        <ChipSelect
          label={globalLabel}
          value={String(globalPercent)}
          onChange={(v) => setGlobal(Number(v))}
          disabled={saving}
          options={CPU_BUDGET_GLOBAL_CHOICES.map((pct) => ({
            value: String(pct),
            label: t(`admin.serverConfig.cpuBudget.global${pct}`, `${pct} %`),
          }))}
        />
      </SettingRow>

      <View style={{ paddingTop: 12 }}>
        <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600" }}>
          {t("admin.serverConfig.cpuBudget.tasksTitle", "Heavy tasks")}
        </Text>
        <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginTop: 2 }}>
          {t("admin.serverConfig.cpuBudget.tasksDesc", "A heavy task may get a share of its own; shares never stack.")}
        </Text>
      </View>
      {CPU_BUDGET_TASKS.map((field) => {
        const label = t(field.labelKey);
        return (
          <SettingRow key={field.task} label={label} info={t(field.descKey)} kind="wide" testID={`cpu-budget-${field.task}`}>
            <ChipSelect
              label={label}
              value={taskPercent[field.task] === null ? "" : String(taskPercent[field.task])}
              onChange={(v) => setTask(field.task, v === "" ? null : Number(v))}
              disabled={saving}
              options={[
                { value: "", label: t("admin.serverConfig.cpuBudget.followGlobal", { percent: globalPercent, defaultValue: `Global (${globalPercent} %)` }) },
                ...CPU_BUDGET_TASK_CHOICES.map((pct) => ({
                  value: String(pct),
                  label: t("admin.serverConfig.cpuBudget.share", { percent: pct, defaultValue: `${pct} %` }),
                })),
              ]}
            />
          </SettingRow>
        );
      })}
    </View>
  );
}
