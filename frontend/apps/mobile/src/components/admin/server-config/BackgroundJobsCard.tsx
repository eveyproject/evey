import { View, Text, TextInput, Switch, TouchableOpacity, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import { useTheme } from "../../../providers/ThemeProvider";
import { BACKGROUND_JOB_RATES, BACKGROUND_JOB_TOGGLES, useBackgroundJobsForm } from "@evey/client";
import type { ConfigEntry, SaveEntryAsync } from "./types";

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
    <View style={{ gap: 14 }}>
      <View
        style={{
          borderWidth: 1,
          borderColor: theme.colors.warning,
          backgroundColor: theme.colors.surface,
          borderRadius: theme.radii.sm,
          padding: 10,
        }}
      >
        <Text style={{ color: theme.colors.warning, fontSize: 13, fontWeight: "600" }}>
          {t(
            "admin.serverConfig.backgroundJobs.restartWarning",
            "These flags are read at server startup - changes take effect after a restart.",
          )}
        </Text>
      </View>

      {BACKGROUND_JOB_TOGGLES.map((field) => (
        <View key={field.key} style={{ gap: 2 }}>
          <View style={{ flexDirection: "row", alignItems: "center" }}>
            <Text style={{ color: theme.colors.text, fontSize: 14, flex: 1 }}>{t(field.labelKey)}</Text>
            <Switch
              testID={`bg-toggle-${field.key}`}
              value={isEnabled(field.key)}
              onValueChange={() => toggle(field.key, !isEnabled(field.key))}
              disabled={saving}
              trackColor={{ true: theme.colors.accent, false: theme.colors.surfaceRaised }}
            />
          </View>
          <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>{t(field.descKey)}</Text>
        </View>
      ))}

      <View style={{ borderTopWidth: StyleSheet.hairlineWidth, borderTopColor: theme.colors.border, paddingTop: 12, gap: 8 }}>
        <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600" }}>
          {t("admin.serverConfig.backgroundJobs.paceTitle")}
        </Text>
        <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
          {t("admin.serverConfig.backgroundJobs.paceDesc")}
        </Text>
        {BACKGROUND_JOB_RATES.map((field) => (
          <View key={field.key} style={{ gap: 4 }}>
            <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>{t(field.labelKey)}</Text>
            <TextInput
              testID={`bg-rate-${field.key}`}
              style={[
                styles.input,
                { backgroundColor: theme.colors.surface, color: theme.colors.text, borderColor: theme.colors.border },
              ]}
              value={rateDrafts[field.key] ?? ""}
              onChangeText={(v) => setRate(field.key, v)}
              placeholder={t("admin.serverConfig.backgroundJobs.paceDefaultPlaceholder", "default")}
              placeholderTextColor={theme.colors.textMuted}
              keyboardType="number-pad"
            />
          </View>
        ))}
        <TouchableOpacity
          testID="bg-rates-save"
          onPress={saveRates}
          disabled={!rateDirty || saving}
          style={{
            backgroundColor: theme.colors.accent,
            borderRadius: theme.radii.sm,
            paddingVertical: 10,
            alignItems: "center",
            opacity: !rateDirty || saving ? 0.5 : 1,
          }}
        >
          <Text style={{ color: theme.colors.onAccent, fontSize: 14, fontWeight: "600" }}>{t("common.save", "Save")}</Text>
        </TouchableOpacity>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  input: { borderWidth: 1, borderRadius: 8, paddingHorizontal: 12, paddingVertical: 9, fontSize: 14 },
});
