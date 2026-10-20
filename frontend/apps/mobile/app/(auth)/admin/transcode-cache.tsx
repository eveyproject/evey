import { useState, useCallback } from "react";
import { View, Text, FlatList, Pressable, Alert, ActivityIndicator, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { eveyFetch, useAdminSystemServiceGetSystemInfo, formatBytes, apiErrorMessage } from "@evey/api";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { ServerSettingsGroup } from "../../../src/components/admin/settings/ServerSettingsGroup";

interface CacheFile {
  name: string;
  size: number;
  lastModified: string;
}

interface CacheInfo {
  totalSize: number;
  fileCount: number;
  files: CacheFile[];
  // Viewers being transcoded right now - the ones a wipe cuts off. Optional
  // because an older server does not send it, and an admin app that crashed
  // against a server one version behind would be worse than a missing warning.
  activeEncodingSessions?: number;
}

const CACHE_KEY = ["admin", "transcode-cache"];

function formatDate(d: string): string {
  return new Date(d).toLocaleString();
}

// The background-conversion cap through the shared server-settings form, the
// same field as Transcoding > Capacity: seeded with the stored override only
// (blank = the server default), a refetch never overwrites a typed edit, an
// emptied field clears the override and a value below 1 is refused inline.
function TranscodingSettings() {
  const { t } = useTranslation();
  return (
    <ServerSettingsGroup
      title={t("admin.transcodeCache.transcodingSettings", "Réglages de transcodage")}
      fields={["maxConcurrentTranscodes"]}
      saveTestID="transcode-cache-settings-save"
      testID="group-transcode-cache-settings"
    />
  );
}

export default function AdminTranscodeCacheScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [deletingFile, setDeletingFile] = useState<string | null>(null);

  const { data, isLoading, refetch } = useQuery<CacheInfo>({
    queryKey: CACHE_KEY,
    queryFn: () => eveyFetch<CacheInfo>({ url: "/api/v1/admin/transcode-cache", method: "GET" }),
  });

  const { data: sysInfo } = useAdminSystemServiceGetSystemInfo();

  const clearAll = useMutation({
    mutationFn: () => eveyFetch<void>({ url: "/api/v1/admin/transcode-cache", method: "DELETE" }),
    onSuccess: async () => {
      toast.success(t("admin.transcodeCache.cacheCleared", "Cache cleared"));
      await qc.invalidateQueries({ queryKey: CACHE_KEY });
    },
    onError: (err) => toast.error(apiErrorMessage(err as Error, t, t("admin.transcodeCache.failedClear"))),
  });

  const handleClearAll = useCallback(() => {
    // A wipe deletes the segments a live encode is still writing, so anyone
    // being transcoded rebuffers. Nothing on this page shows that, and the
    // warning the server logs reaches nobody holding the phone.
    const live = data?.activeEncodingSessions ?? 0;
    const liveWarning = live > 0
      ? "\n\n" + t("admin.transcodeCache.clearConfirmLiveSessions", {
        count: live,
        defaultValue: `${live} viewer(s) are being transcoded right now; their stream will rebuffer.`,
      })
      : "";
    Alert.alert(
      t("admin.transcodeCache.clearConfirmTitle", "Clear Cache"),
      t("admin.transcodeCache.clearConfirm", {
        count: data?.fileCount ?? 0,
        size: formatBytes(data?.totalSize ?? 0),
        defaultValue: `Delete all ${data?.fileCount ?? 0} cached files (${formatBytes(data?.totalSize ?? 0)})?`,
      }) + liveWarning,
      [
        { text: t("common.cancel", "Cancel"), style: "cancel" },
        { text: t("admin.transcodeCache.clearAll", "Clear All"), style: "destructive", onPress: () => clearAll.mutate() },
      ],
    );
  }, [data, clearAll, t]);

  const handleDeleteFile = useCallback(async (filename: string) => {
    setDeletingFile(filename);
    try {
      await eveyFetch<void>({ url: `/api/v1/admin/transcode-cache/${encodeURIComponent(filename)}`, method: "DELETE" });
      toast.success(t("admin.transcodeCache.deleted", { name: filename, defaultValue: "Deleted" }));
      await qc.invalidateQueries({ queryKey: CACHE_KEY });
    } catch (err) {
      toast.error(apiErrorMessage(err as Error, t, t("admin.transcodeCache.failedDeleteFile")));
    } finally {
      setDeletingFile(null);
    }
  }, [qc, t]);

  const hwDevices = sysInfo?.hwDevices ?? [];
  const files = data?.files ?? [];

  const renderFile = useCallback(({ item }: { item: CacheFile }) => (
    <View style={[styles.fileRow, { borderBottomColor: theme.colors.border }]}>
      <View style={{ flex: 1 }}>
        <Text style={{ color: theme.colors.text, fontSize: 13, fontFamily: "monospace" }} numberOfLines={1}>{item.name}</Text>
        <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginTop: 2 }}>
          {formatBytes(item.size)} · {formatDate(item.lastModified)}
        </Text>
      </View>
      <Pressable
        onPress={() => handleDeleteFile(item.name)}
        disabled={deletingFile === item.name}
        accessible
        accessibilityRole="button"
        accessibilityLabel={t("common.delete", "Supprimer") + " " + item.name}
        style={{ padding: 8, opacity: deletingFile === item.name ? 0.4 : 1 }}
      >
        {deletingFile === item.name ? (
          <ActivityIndicator size="small" color={theme.colors.error} />
        ) : (
          <Ionicons name="trash-outline" size={20} color={theme.colors.error} />
        )}
      </Pressable>
    </View>
  ), [theme, deletingFile, handleDeleteFile, t]);

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <Stack.Screen options={{
        headerShown: true,
        title: t("admin.transcodeCache.title", "Transcode Cache"),
        headerStyle: { backgroundColor: theme.colors.surface },
        headerTintColor: theme.colors.text,
      }} />

      {isLoading ? (
        <View style={{ flex: 1, justifyContent: "center", alignItems: "center" }}>
          <ActivityIndicator color={theme.colors.accent} size="large" />
        </View>
      ) : (
        <FlatList
          automaticallyAdjustKeyboardInsets
          keyboardShouldPersistTaps="handled"
          data={files}
          keyExtractor={(item) => item.name}
          renderItem={renderFile}
          contentContainerStyle={{ paddingBottom: 40 }}
          onRefresh={refetch}
          refreshing={false}
          ListHeaderComponent={
            <View style={{ padding: 16, gap: 12 }}>
              <TranscodingSettings />

              {/* Stats */}
              <View style={{ flexDirection: "row", gap: 12 }}>
                <View style={[styles.statCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
                  <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>{t("admin.transcodeCache.totalSize", "Total Size")}</Text>
                  <Text style={{ color: theme.colors.text, fontSize: 22, fontWeight: "700" }}>{formatBytes(data?.totalSize ?? 0)}</Text>
                </View>
                <View style={[styles.statCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
                  <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>{t("admin.transcodeCache.files", "Files")}</Text>
                  <Text style={{ color: theme.colors.text, fontSize: 22, fontWeight: "700" }}>{data?.fileCount ?? 0}</Text>
                </View>
              </View>

              {/* HW devices */}
              {hwDevices.length > 0 && (
                <View style={[styles.section, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
                  <Text style={{ color: theme.colors.textMuted, fontSize: 12, fontWeight: "600", textTransform: "uppercase", marginBottom: 8 }}>
                    {t("admin.transcodeCache.hwDevices", "Hardware Acceleration")}
                  </Text>
                  {hwDevices.map((d, i) => (
                    <View key={i} style={{ flexDirection: "row", alignItems: "center", gap: 8, marginTop: i > 0 ? 6 : 0 }}>
                      <Ionicons name="hardware-chip-outline" size={16} color={theme.colors.accent} />
                      <Text style={{ color: theme.colors.text, fontSize: 13 }}>
                        {d.type?.replace("HW_ACCEL_TYPE_", "")} - {d.name ?? "unknown"}
                      </Text>
                    </View>
                  ))}
                </View>
              )}

              {/* Files header */}
              {files.length > 0 && (
                <Text style={{ color: theme.colors.textMuted, fontSize: 13, fontWeight: "600", textTransform: "uppercase", marginTop: 4 }}>
                  {t("admin.transcodeCache.tableHeaders.file", "Cached Files")}
                </Text>
              )}

              {files.length === 0 && (
                <View style={{ alignItems: "center", paddingVertical: 40 }}>
                  <Ionicons name="film-outline" size={48} color={theme.colors.textMuted} />
                  <Text style={{ color: theme.colors.textMuted, fontSize: 15, marginTop: 12 }}>
                    {t("admin.transcodeCache.cacheEmpty", "Cache is empty")}
                  </Text>
                </View>
              )}
            </View>
          }
          ListFooterComponent={
            files.length > 0 ? (
              <View style={{ padding: 16 }}>
                <Pressable
                  onPress={handleClearAll}
                  disabled={clearAll.isPending}
                  style={[styles.dangerBtn, { backgroundColor: theme.colors.error, borderRadius: theme.radii.md, opacity: clearAll.isPending ? 0.5 : 1 }]}
                >
                  {clearAll.isPending && <ActivityIndicator size="small" color={theme.colors.onError} style={{ marginRight: 8 }} />}
                  <Text style={{ color: theme.colors.onError, fontWeight: "600", fontSize: 15 }}>
                    {t("admin.transcodeCache.clearAll", "Clear All Cache")}
                  </Text>
                </Pressable>
              </View>
            ) : null
          }
        />
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  statCard: { flex: 1, padding: 16, alignItems: "center", gap: 4 },
  section: { padding: 16 },
  fileRow: { flexDirection: "row", alignItems: "center", paddingVertical: 12, paddingHorizontal: 16, borderBottomWidth: StyleSheet.hairlineWidth },
  dangerBtn: { paddingVertical: 14, alignItems: "center", justifyContent: "center", flexDirection: "row" },
});
