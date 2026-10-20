import { View, Text, TextInput, Switch, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import { useTheme } from "../../../providers/ThemeProvider";
import { SettingRow } from "../settings/SettingRow";
import { SaveButton } from "../settings/SaveButton";
import { BACKGROUND_JOB_RATES, BACKGROUND_JOB_TOGGLES, useBackgroundJobsForm } from "@evey/client";
import type { ConfigEntry, SaveEntryAsync } from "./types";

// paceInfoKey picks the explanation of a pacing field by what it measures:
// the batch size, the pause between batches, or the wait once caught up.
function paceInfoKey(key: string): string {
  if (key.endsWith("_batch")) return "admin.settingsHelp.paceBatch";
  if (key.endsWith("_pause_ms")) return "admin.settingsHelp.pacePause";
  return "admin.settingsHelp.paceIdle";
}

interface BackgroundJobsCardProps {
  entries: ConfigEntry[] | undefined;
  saveEntry: SaveEntryAsync;
  deleteEntry: (key: string) => Promise<unknown>;
  // The pace Save writes six keys and toasts once: these writers stay quiet.
  quietSaveEntry: SaveEntryAsync;
  quietDeleteEntry: (key: string) => Promise<unknown>;
  saving: boolean;
}

export function BackgroundJobsCard({ entries, saveEntry, deleteEntry, quietSaveEntry, quietDeleteEntry, saving }: BackgroundJobsCardProps) {
  const theme = useTheme();
  const { t } = useTranslation();

  const { rateDrafts, rateDirty, setRate, isEnabled, toggle, saveRates } = useBackgroundJobsForm({
    entries,
    save: (key, value) => saveEntry({ key, value }),
    remove: (key) => deleteEntry(key),
    quietSave: (key, value) => quietSaveEntry({ key, value }),
    quietRemove: (key) => quietDeleteEntry(key),
    onRatesSaved: (failed) =>
      failed.length === 0
        ? toast.success(t("admin.serverConfig.configUpdated", "Config saved"))
        : toast.error(t("admin.serverConfig.failedUpdate", "Failed to update config")),
  });

  return (
    <View>
      <Text style={{ color: theme.colors.textMuted, fontSize: 12, paddingBottom: 4 }}>
        {t(
          "admin.serverConfig.backgroundJobs.restartWarning",
          "These flags are read at server startup - changes take effect after a restart.",
        )}
      </Text>

      {BACKGROUND_JOB_TOGGLES.map((field) => (
        <SettingRow key={field.key} label={t(field.labelKey)} info={t(field.descKey)} restart kind="toggle">
          <Switch
            testID={`bg-toggle-${field.key}`}
            accessibilityLabel={t(field.labelKey)}
            value={isEnabled(field.key)}
            onValueChange={() => toggle(field.key, !isEnabled(field.key))}
            disabled={saving}
            trackColor={{ true: theme.colors.accent, false: theme.colors.surfaceRaised }}
          />
        </SettingRow>
      ))}

      <View style={{ borderTopWidth: StyleSheet.hairlineWidth, borderTopColor: theme.colors.border, paddingTop: 12 }}>
        <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600" }}>
          {t("admin.settingsLayout.advancedPacing")}
        </Text>
        <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginTop: 2 }}>
          {t("admin.serverConfig.backgroundJobs.paceDesc")}
        </Text>
        {BACKGROUND_JOB_RATES.map((field) => (
          <SettingRow key={field.key} label={t(field.labelKey)} info={t(paceInfoKey(field.key))} restart>
            <TextInput
              testID={`bg-rate-${field.key}`}
              accessibilityLabel={t(field.labelKey)}
              style={[
                styles.input,
                { backgroundColor: theme.colors.surfaceRaised, color: theme.colors.text, borderColor: theme.colors.border },
              ]}
              value={rateDrafts[field.key] ?? ""}
              onChangeText={(v) => setRate(field.key, v)}
              placeholder={t("admin.serverConfig.backgroundJobs.paceDefaultPlaceholder", "default")}
              placeholderTextColor={theme.colors.textMuted}
              keyboardType="number-pad"
            />
          </SettingRow>
        ))}
        <SaveButton testID="bg-rates-save" onPress={saveRates} disabled={!rateDirty || saving} />
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  input: { borderWidth: 1, borderRadius: 8, paddingHorizontal: 12, paddingVertical: 9, fontSize: 14 },
});
