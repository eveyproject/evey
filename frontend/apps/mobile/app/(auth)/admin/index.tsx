import { View, Text, ScrollView, Pressable, RefreshControl, StyleSheet } from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { Stack, useRouter } from "expo-router";
import { useState, useCallback } from "react";
import { useTranslation } from "react-i18next";
import {
  useAdminServiceGetDashboard,
  useAdminServiceGetSystemInfo,
} from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";

function StatCard({ label, value, icon, theme }: { label: string; value: string | number; icon: string; theme: ReturnType<typeof useTheme> }) {
  return (
    <View style={[styles.statCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
      <Ionicons name={icon as never} size={24} color={theme.colors.accent} />
      <Text style={[styles.statValue, { color: theme.colors.text }]}>{value}</Text>
      <Text style={[styles.statLabel, { color: theme.colors.textMuted }]}>{label}</Text>
    </View>
  );
}

function formatUptime(seconds?: string): string {
  if (!seconds) return "—";
  const s = parseInt(seconds, 10);
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

export default function AdminDashboard() {
  const theme = useTheme();
  const { t } = useTranslation();
  const router = useRouter();
  const [refreshing, setRefreshing] = useState(false);

  const { data: dashboard, refetch: refetchDash } = useAdminServiceGetDashboard({ query: { refetchInterval: 10_000 } });
  const { data: sysInfo, refetch: refetchSys } = useAdminServiceGetSystemInfo();

  const onRefresh = useCallback(async () => {
    setRefreshing(true);
    await Promise.all([refetchDash(), refetchSys()]);
    setRefreshing(false);
  }, [refetchDash, refetchSys]);

  const navItems = [
    { label: t("admin.users.title"), icon: "people-outline", path: "/admin/users" },
    { label: t("admin.nav.libraries"), icon: "library-outline", path: "/admin/libraries" },
    { label: t("admin.sessions.title"), icon: "play-circle-outline", path: "/admin/sessions" },
    { label: "Plugins", icon: "extension-puzzle-outline", path: "/admin/plugins" },
    { label: "API Keys", icon: "key-outline", path: "/admin/api-keys" },
    { label: "Server Config", icon: "server-outline", path: "/admin/server-config" },
    { label: t("admin.backup.title") || "Database Backup", icon: "cloud-download-outline", path: "/admin/backup" },
  ];

  return (
    <ScrollView
      style={{ flex: 1, backgroundColor: theme.colors.bg }}
      contentContainerStyle={{ paddingBottom: 40 }}
      refreshControl={<RefreshControl refreshing={refreshing} onRefresh={onRefresh} tintColor={theme.colors.accent} />}
    >
      <Stack.Screen
        options={{
          headerShown: true,
          title: t("admin.dashboard.title"),
          headerStyle: { backgroundColor: theme.colors.surface },
          headerTintColor: theme.colors.text,
        }}
      />

      {/* Stats grid */}
      <View style={styles.statsGrid}>
        <StatCard label={t("admin.systemHealth.totalMedia")} value={dashboard?.totalMediaItems ?? 0} icon="film-outline" theme={theme} />
        <StatCard label={t("admin.systemHealth.totalUsers")} value={dashboard?.totalUsers ?? 0} icon="people-outline" theme={theme} />
        <StatCard label={t("admin.systemHealth.activeStreams")} value={dashboard?.activeStreams ?? 0} icon="play-outline" theme={theme} />
        <StatCard label={t("admin.systemHealth.activeTranscodes")} value={dashboard?.activeTranscodes ?? 0} icon="swap-horizontal-outline" theme={theme} />
      </View>

      {/* System info */}
      <Text style={[styles.sectionTitle, { color: theme.colors.textMuted }]}>{t("admin.systemHealth.serverInfo")}</Text>
      <View style={[styles.infoCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md, marginHorizontal: 16 }]}>
        {[
          { label: "Version", value: sysInfo?.version },
          { label: "OS", value: sysInfo?.os ? `${sysInfo.os} ${sysInfo.arch}` : undefined },
          { label: "Go", value: sysInfo?.goVersion },
          { label: "FFmpeg", value: sysInfo?.ffmpegVersion },
          { label: "Uptime", value: formatUptime(sysInfo?.uptimeSeconds) },
        ].map((row) => (
          <View key={row.label} style={[styles.infoRow, { borderBottomColor: theme.colors.border }]}>
            <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>{row.label}</Text>
            <Text style={{ color: theme.colors.text, fontSize: 13, flex: 1, textAlign: "right" }} numberOfLines={1}>{row.value ?? "—"}</Text>
          </View>
        ))}
      </View>

      {/* CPU / Memory */}
      {(dashboard?.cpuPercent != null || dashboard?.memoryPercent != null) && (
        <>
          <Text style={[styles.sectionTitle, { color: theme.colors.textMuted }]}>Resources</Text>
          <View style={[styles.infoCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md, marginHorizontal: 16 }]}>
            {dashboard?.cpuPercent != null && (
              <View style={[styles.infoRow, { borderBottomColor: theme.colors.border }]}>
                <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>CPU</Text>
                <Text style={{ color: theme.colors.text, fontSize: 13 }}>{Math.round(dashboard.cpuPercent)}%</Text>
              </View>
            )}
            {dashboard?.memoryPercent != null && (
              <View style={[styles.infoRow, { borderBottomColor: theme.colors.border }]}>
                <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>Memory</Text>
                <Text style={{ color: theme.colors.text, fontSize: 13 }}>{Math.round(dashboard.memoryPercent)}%</Text>
              </View>
            )}
          </View>
        </>
      )}

      {/* Navigation */}
      <Text style={[styles.sectionTitle, { color: theme.colors.textMuted }]}>{t("admin.nav.monitoring")}</Text>
      <View style={[styles.infoCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md, marginHorizontal: 16 }]}>
        {navItems.map((item) => (
          <Pressable key={item.path} onPress={() => router.push(item.path as never)} style={[styles.navRow, { borderBottomColor: theme.colors.border }]}>
            <Ionicons name={item.icon as never} size={20} color={theme.colors.accent} />
            <Text style={{ color: theme.colors.text, fontSize: 15, flex: 1 }}>{item.label}</Text>
            <Ionicons name="chevron-forward" size={16} color={theme.colors.textMuted} />
          </Pressable>
        ))}
      </View>
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  statsGrid: { flexDirection: "row", flexWrap: "wrap", padding: 16, gap: 12 },
  statCard: { flex: 1, minWidth: "45%", padding: 16, alignItems: "center", gap: 4 },
  statValue: { fontSize: 28, fontWeight: "700" },
  statLabel: { fontSize: 12 },
  sectionTitle: { fontSize: 13, fontWeight: "600", textTransform: "uppercase", marginTop: 20, marginBottom: 8, marginHorizontal: 16 },
  infoCard: { overflow: "hidden" },
  infoRow: { flexDirection: "row", justifyContent: "space-between", paddingVertical: 12, paddingHorizontal: 16, borderBottomWidth: StyleSheet.hairlineWidth },
  navRow: { flexDirection: "row", alignItems: "center", gap: 12, paddingVertical: 14, paddingHorizontal: 16, borderBottomWidth: StyleSheet.hairlineWidth },
});
