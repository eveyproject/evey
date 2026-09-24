import { View, Text, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../../providers/ThemeProvider";
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
  const { globalPercent, taskPercent, setGlobal, setTask } = useCpuBudgetForm(entries, {
    save: (key, value) => saveEntry({ key, value }),
    remove: (key) => deleteEntry(key),
  });

  return (
    <View style={{ gap: 14 }}>
      <View>
        <Text style={[styles.label, { color: theme.colors.textMuted }]}>
          {t("admin.serverConfig.cpuBudget.globalLabel", "Global share")}
        </Text>
        <ChipSelect
          value={String(globalPercent)}
          onChange={(v) => setGlobal(Number(v))}
          disabled={saving}
          options={CPU_BUDGET_GLOBAL_CHOICES.map((pct) => ({
            value: String(pct),
            label: t(`admin.serverConfig.cpuBudget.global${pct}`, `${pct} %`),
          }))}
        />
        <Text style={[styles.desc, { color: theme.colors.textMuted }]}>
          {t("admin.serverConfig.cpuBudget.globalDesc", "Share of the processor every background task draws from.")}
        </Text>
      </View>

      <View style={{ borderTopWidth: StyleSheet.hairlineWidth, borderTopColor: theme.colors.border, paddingTop: 12, gap: 12 }}>
        <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600" }}>
          {t("admin.serverConfig.cpuBudget.tasksTitle", "Heavy tasks")}
        </Text>
        <Text style={[styles.desc, { color: theme.colors.textMuted, marginTop: 0 }]}>
          {t("admin.serverConfig.cpuBudget.tasksDesc", "A heavy task may get a share of its own; shares never stack.")}
        </Text>
        {CPU_BUDGET_TASKS.map((field) => (
          <View key={field.task} testID={`cpu-budget-${field.task}`}>
            <Text style={[styles.label, { color: theme.colors.textMuted }]}>{t(field.labelKey)}</Text>
            <ChipSelect
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
            <Text style={[styles.desc, { color: theme.colors.textMuted }]}>{t(field.descKey)}</Text>
          </View>
        ))}
        <Text style={[styles.desc, { color: theme.colors.textMuted, marginTop: 0 }]}>
          {t("admin.serverConfig.cpuBudget.applyHint", "Applies to the next item, no restart needed.")}
        </Text>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  label: { fontSize: 12, fontWeight: "600", textTransform: "uppercase", marginBottom: 6 },
  desc: { fontSize: 12, marginTop: 8 },
});
