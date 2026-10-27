import { useEffect, useState } from "react";
import { View, Text, Pressable, ActivityIndicator, StyleSheet } from "react-native";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useAdminPluginServiceGetPluginMetadataSettings,
  useAdminPluginServiceUpdatePluginMetadataSettings,
  useAdminPluginServiceRefreshPluginMetadata,
  getAdminPluginServiceGetPluginMetadataSettingsQueryKey,
  apiErrorMessage,
} from "@evey/api";
import { toast, useDemoStore, demoLocks } from "@evey/store";
import { useTheme } from "../../../providers/ThemeProvider";

// How often the section re-reads the library fetch while it runs.
const REFRESH_POLL_MS = 2000;

// The fields a metadata plugin replaces and the fetch of the existing library
// (mirror web PluginMetadataSettings).
export function PluginMetadataSection({ pluginId }: { pluginId: string }) {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();
  const s = styles(theme);
  const saveLocked = useDemoStore((st) => demoLocks(st, "/evey.v1.AdminPluginService/UpdatePluginMetadataSettings"));
  const refreshLocked = useDemoStore((st) => demoLocks(st, "/evey.v1.AdminPluginService/RefreshPluginMetadata"));
  const [running, setRunning] = useState(false);
  const { data, isLoading } = useAdminPluginServiceGetPluginMetadataSettings(pluginId, {
    query: { refetchInterval: running ? REFRESH_POLL_MS : false },
  });
  const [fields, setFields] = useState<string[]>([]);

  useEffect(() => {
    if (!data) return;
    setFields(data.fields ?? []);
    setRunning(!!data.refresh?.running);
  }, [data]);

  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: getAdminPluginServiceGetPluginMetadataSettingsQueryKey(pluginId) });
  };
  const save = useAdminPluginServiceUpdatePluginMetadataSettings({
    mutation: {
      onSuccess: () => { invalidate(); toast.success(t("admin.plugin.metadata.saved")); },
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.plugin.metadata.saveFailed"))),
    },
  });
  const refresh = useAdminPluginServiceRefreshPluginMetadata({
    mutation: {
      onSuccess: () => { setRunning(true); invalidate(); toast.success(t("admin.plugin.metadata.refreshStarted")); },
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.plugin.metadata.refreshFailed"))),
    },
  });

  if (isLoading) {
    return <ActivityIndicator color={theme.colors.accent} style={{ paddingVertical: 12 }} />;
  }
  const available = data?.availableFields ?? [];
  const status = data?.refresh;
  const toggle = (f: string) => setFields((cur) => (cur.includes(f) ? cur.filter((x) => x !== f) : [...cur, f]));

  return (
    <View testID="plugin-metadata-section">
      <Text style={s.hint}>{t("admin.plugin.metadata.description")}</Text>
      <Text style={[s.label, { marginTop: 10 }]}>{t("admin.plugin.metadata.fieldsLabel")}</Text>
      <View style={s.chipRow}>
        {available.map((f) => {
          const active = fields.includes(f);
          return (
            <Pressable
              key={f}
              testID={`metadata-field-${f}`}
              onPress={() => toggle(f)}
              disabled={saveLocked}
              accessibilityRole="checkbox"
              accessibilityState={{ checked: active, disabled: saveLocked }}
              style={{ paddingHorizontal: 10, paddingVertical: 5, borderRadius: theme.radii.sm, borderWidth: 1, borderColor: active ? theme.colors.accent : theme.colors.border, backgroundColor: active ? theme.colors.accent : "transparent" }}
            >
              <Text style={{ color: active ? theme.colors.onAccent : theme.colors.text, fontSize: 12, fontWeight: "600" }}>
                {t(`admin.plugin.metadata.fields.${f}`)}
              </Text>
            </Pressable>
          );
        })}
      </View>
      <Pressable
        testID="metadata-save"
        onPress={() => save.mutate({ pluginId, data: { fields } })}
        disabled={save.isPending || saveLocked}
        accessibilityHint={saveLocked ? t("common.demo.locked") : undefined}
        style={[s.btn, { backgroundColor: theme.colors.accent, opacity: save.isPending || saveLocked ? 0.5 : 1 }]}
      >
        {save.isPending
          ? <ActivityIndicator size="small" color={theme.colors.onAccent} />
          : <Text style={{ color: theme.colors.onAccent, fontWeight: "600", fontSize: 15 }}>{t("admin.plugin.metadata.save")}</Text>}
      </Pressable>

      <View style={{ borderTopWidth: 1, borderTopColor: theme.colors.border, marginTop: 14, paddingTop: 10 }}>
        <Text style={s.hint}>{t("admin.plugin.metadata.refreshHint")}</Text>
        {status?.running ? (
          <Text testID="metadata-refresh-running" style={s.status}>
            {t("admin.plugin.metadata.running", { done: status.done ?? 0, total: status.total ?? 0 })}
          </Text>
        ) : status?.finishedAt ? (
          <Text testID="metadata-refresh-finished" style={s.status}>
            {t("admin.plugin.metadata.finished", { covered: status.covered ?? 0, total: status.total ?? 0, failed: status.failed ?? 0 })}
          </Text>
        ) : null}
        <Pressable
          testID="metadata-refresh"
          onPress={() => refresh.mutate({ pluginId, data: {} })}
          disabled={refresh.isPending || !!status?.running || refreshLocked}
          accessibilityHint={refreshLocked ? t("common.demo.locked") : undefined}
          style={[s.btn, { borderWidth: 1, borderColor: theme.colors.accent, opacity: refresh.isPending || status?.running || refreshLocked ? 0.5 : 1 }]}
        >
          {refresh.isPending
            ? <ActivityIndicator size="small" color={theme.colors.accent} />
            : <Text style={{ color: theme.colors.accent, fontWeight: "600", fontSize: 15 }}>{t("admin.plugin.metadata.refresh")}</Text>}
        </Pressable>
      </View>
    </View>
  );
}

const styles = (theme: ReturnType<typeof useTheme>) =>
  StyleSheet.create({
    hint: { color: theme.colors.textMuted, fontSize: 12, marginTop: 2 },
    status: { color: theme.colors.text, fontSize: 13, marginTop: 8 },
    label: { color: theme.colors.textMuted, fontSize: 11, fontWeight: "600", textTransform: "uppercase", marginBottom: 4 },
    chipRow: { flexDirection: "row", flexWrap: "wrap", gap: 6, marginBottom: 4 },
    btn: { borderRadius: theme.radii.md, paddingVertical: 12, alignItems: "center", marginTop: 12 },
  });
