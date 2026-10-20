import { useEffect, useState } from "react";
import { ActivityIndicator, StyleSheet, TextInput, View } from "react-native";
import { useTranslation } from "react-i18next";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  getServerConfigServiceListServerConfigEntriesQueryKey,
  serverConfigServiceSetServerConfigEntry,
  useServerConfigServiceListServerConfigEntries,
} from "@evey/api";
import { toast } from "@evey/store";
import { FONT_SIZE } from "@evey/theme";
import { useTheme } from "../../../providers/ThemeProvider";
import { SettingRow } from "../settings/SettingRow";
import { SettingsGroup } from "../settings/SettingsGroup";
import { SaveButton } from "../settings/SaveButton";
import { ServerSettingFieldRow, useServerSettingsForm } from "../settings/ServerSettingsGroup";

const MAX_VIDEO_SESSIONS_KEY = "transcoding.max_video_sessions";

// CapacitySettingsGroup holds the two "how many at once" limits with one
// Save (the web's CapacitySettingsGroup): the live video streams (a config
// entry, it can only lower what the host measured; blank = the measure) and
// the background conversions queued in advance (a server setting).
export function CapacitySettingsGroup() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();
  const form = useServerSettingsForm(["maxConcurrentTranscodes"]);
  const { data: entries } = useServerConfigServiceListServerConfigEntries({ query: { select: (d) => d.entries ?? [] } });
  const stored = entries?.find((e) => e.key === MAX_VIDEO_SESSIONS_KEY)?.value ?? "";
  const [sessions, setSessions] = useState(stored);
  const [sessionsDirty, setSessionsDirty] = useState(false);
  const [sessionsError, setSessionsError] = useState<string | undefined>();
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!sessionsDirty) setSessions(stored);
  }, [stored, sessionsDirty]);

  const saveSessions = useMutation({
    mutationFn: (v: string) => serverConfigServiceSetServerConfigEntry(MAX_VIDEO_SESSIONS_KEY, { value: v }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: getServerConfigServiceListServerConfigEntriesQueryKey() });
      setSessionsDirty(false);
      toast.success(t("admin.transcoding.videoSessionsSaved", "Video transcode capacity updated"));
    },
    onError: () => toast.error(t("admin.transcoding.videoSessionsFailed", "Could not update the capacity")),
  });

  async function submitSessions(): Promise<boolean> {
    if (!sessionsDirty) return true;
    const v = sessions.trim();
    if (v !== "" && (!/^\d+$/.test(v) || parseInt(v, 10) < 1)) {
      setSessionsError(t("admin.settingsLayout.errorNumber", { min: 1 }));
      return false;
    }
    try {
      await saveSessions.mutateAsync(v);
      return true;
    } catch {
      return false;
    }
  }

  async function save() {
    setSaving(true);
    try {
      // Both run even if one fails: each reports its own outcome.
      await Promise.all([submitSessions(), form.submit()]);
    } finally {
      setSaving(false);
    }
  }

  const sessionsLabel = t("admin.transcoding.maxVideoSessionsLabel");
  return (
    <SettingsGroup
      title={t("admin.settingsLayout.groupCapacity")}
      description={t("admin.settingsLayout.groupCapacityDesc")}
      testID="group-capacity"
      footer={<SaveButton onPress={() => void save()} loading={saving} disabled={!sessionsDirty && !form.dirty} testID="capacity-save-btn" />}
    >
      <SettingRow label={sessionsLabel} info={t("admin.transcoding.maxVideoSessionsDesc")} error={sessionsError} testID="max-video-sessions-row">
        <TextInput
          value={sessions}
          onChangeText={(v) => {
            setSessions(v);
            setSessionsDirty(true);
            setSessionsError(undefined);
          }}
          placeholder={t("admin.transcoding.maxVideoSessionsAuto")}
          placeholderTextColor={theme.colors.textMuted}
          keyboardType="number-pad"
          accessibilityLabel={sessionsLabel}
          testID="max-video-sessions-input"
          style={[
            styles.input,
            {
              backgroundColor: theme.colors.surfaceRaised,
              color: theme.colors.text,
              borderRadius: theme.radii.md,
              borderColor: sessionsError ? theme.colors.danger : theme.colors.border,
            },
          ]}
        />
      </SettingRow>
      {form.isLoading || !form.data ? (
        <View style={styles.loading}>
          <ActivityIndicator color={theme.colors.accent} />
        </View>
      ) : (
        <ServerSettingFieldRow view={form.field("maxConcurrentTranscodes")} />
      )}
    </SettingsGroup>
  );
}

const styles = StyleSheet.create({
  input: { borderWidth: StyleSheet.hairlineWidth, paddingHorizontal: 14, paddingVertical: 12, fontSize: FONT_SIZE.sm },
  loading: { paddingVertical: 16, alignItems: "center" },
});
