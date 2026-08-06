import { useEffect, useRef, useState } from "react";
import { View, Text, TextInput, Switch, TouchableOpacity, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../../providers/ThemeProvider";
import type { ConfigEntry, SaveEntry } from "./types";

// These flags are read once by resolveFlag() at server boot and never re-read:
// the warning banner mirrors the web card - the write lands in server_config
// immediately but has zero effect until the server restarts.
const BG_TOGGLE_FIELDS = [
  {
    key: "transcoding.pause_bg_on_playback",
    labelKey: "admin.serverConfig.backgroundJobs.pauseBgOnPlaybackLabel",
    descKey: "admin.serverConfig.backgroundJobs.pauseBgOnPlaybackDesc",
  },
  {
    key: "transcoding.level_backfill",
    labelKey: "admin.serverConfig.backgroundJobs.levelBackfillLabel",
    descKey: "admin.serverConfig.backgroundJobs.levelBackfillDesc",
  },
  {
    key: "transcoding.keyframe_backfill",
    labelKey: "admin.serverConfig.backgroundJobs.keyframeBackfillLabel",
    descKey: "admin.serverConfig.backgroundJobs.keyframeBackfillDesc",
  },
  {
    key: "transcoding.snapshot_fastpath",
    labelKey: "admin.serverConfig.backgroundJobs.snapshotFastpathLabel",
    descKey: "admin.serverConfig.backgroundJobs.snapshotFastpathDesc",
  },
] as const;

const BG_RATE_FIELDS = [
  { key: "transcoding.level_backfill_batch", labelKey: "admin.serverConfig.backgroundJobs.levelBackfillBatchLabel" },
  { key: "transcoding.level_backfill_pause_ms", labelKey: "admin.serverConfig.backgroundJobs.levelBackfillPauseMsLabel" },
  { key: "transcoding.level_backfill_idle_sec", labelKey: "admin.serverConfig.backgroundJobs.levelBackfillIdleSecLabel" },
  { key: "transcoding.keyframe_backfill_batch", labelKey: "admin.serverConfig.backgroundJobs.keyframeBackfillBatchLabel" },
  { key: "transcoding.keyframe_backfill_pause_ms", labelKey: "admin.serverConfig.backgroundJobs.keyframeBackfillPauseMsLabel" },
  { key: "transcoding.keyframe_backfill_idle_sec", labelKey: "admin.serverConfig.backgroundJobs.keyframeBackfillIdleSecLabel" },
] as const;

interface BackgroundJobsCardProps {
  entries: ConfigEntry[] | undefined;
  saveEntry: SaveEntry;
  deleteEntry: (key: string, callbacks?: { onSuccess?: () => void; onError?: () => void }) => void;
  saving: boolean;
}

export function BackgroundJobsCard({ entries, saveEntry, deleteEntry, saving }: BackgroundJobsCardProps) {
  const theme = useTheme();
  const { t } = useTranslation();

  const [rateDrafts, setRateDrafts] = useState<Record<string, string>>({});
  const [rateDirty, setRateDirty] = useState(false);

  // Seed rate drafts only the first time real data arrives (same pattern as
  // SmtpSettingsCard) so a refetch never clobbers in-progress edits.
  const seeded = useRef(false);
  useEffect(() => {
    if (seeded.current || !entries) return;
    seeded.current = true;
    const draft: Record<string, string> = {};
    for (const field of BG_RATE_FIELDS) {
      draft[field.key] = entries.find((e) => e.key === field.key)?.value ?? "";
    }
    setRateDrafts(draft);
  }, [entries]);

  function isEnabled(key: string) {
    return entries?.find((e) => e.key === key)?.value !== "off";
  }

  function handleToggle(key: string, enabled: boolean) {
    if (enabled) {
      // Re-enabling reverts to the server default rather than writing "on",
      // consistent with the reset-to-default pattern of this page.
      deleteEntry(key);
    } else {
      saveEntry({ key, value: "off" });
    }
  }

  function handleSaveRates() {
    // Don't clear the dirty flag until every async write lands - clearing up
    // front reported success even when every write failed (web card comment).
    let remaining = BG_RATE_FIELDS.length;
    let hasError = false;
    const settle = () => {
      remaining -= 1;
      if (remaining === 0 && !hasError) setRateDirty(false);
    };
    for (const field of BG_RATE_FIELDS) {
      const value = (rateDrafts[field.key] ?? "").trim();
      const callbacks = {
        onSuccess: settle,
        onError: () => {
          hasError = true;
        },
      };
      if (value) {
        saveEntry({ key: field.key, value }, callbacks);
      } else {
        deleteEntry(field.key, callbacks);
      }
    }
  }

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

      {BG_TOGGLE_FIELDS.map((field) => (
        <View key={field.key} style={{ gap: 2 }}>
          <View style={{ flexDirection: "row", alignItems: "center" }}>
            <Text style={{ color: theme.colors.text, fontSize: 14, flex: 1 }}>{t(field.labelKey)}</Text>
            <Switch
              testID={`bg-toggle-${field.key}`}
              value={isEnabled(field.key)}
              onValueChange={() => handleToggle(field.key, !isEnabled(field.key))}
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
        {BG_RATE_FIELDS.map((field) => (
          <View key={field.key} style={{ gap: 4 }}>
            <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>{t(field.labelKey)}</Text>
            <TextInput
              testID={`bg-rate-${field.key}`}
              style={[
                styles.input,
                { backgroundColor: theme.colors.surface, color: theme.colors.text, borderColor: theme.colors.border },
              ]}
              value={rateDrafts[field.key] ?? ""}
              onChangeText={(v) => {
                setRateDrafts((p) => ({ ...p, [field.key]: v }));
                setRateDirty(true);
              }}
              placeholder={t("admin.serverConfig.backgroundJobs.paceDefaultPlaceholder", "default")}
              placeholderTextColor={theme.colors.textMuted}
              keyboardType="number-pad"
            />
          </View>
        ))}
        <TouchableOpacity
          testID="bg-rates-save"
          onPress={handleSaveRates}
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
