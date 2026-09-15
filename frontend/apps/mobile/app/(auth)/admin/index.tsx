import { View, Text, ScrollView, Pressable, RefreshControl, ActivityIndicator, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";

// The icon-name union, so a typo is a compile error rather than a blank square.
type IoniconName = keyof typeof Ionicons.glyphMap;
import { Stack, useRouter } from "expo-router";
import { useState, useCallback, useEffect, useMemo } from "react";
import { useTranslation } from "react-i18next";
import {
  useAdminSystemServiceGetDashboard,
  useAdminSystemServiceRestartServer,
  useAdminSessionServiceListActiveSessions,
  usePresenceServiceListOnline,
  useActivityServiceGetAnalytics,
  useLibraryServiceListMediaItems,
  useLibraryServiceListLibraries,
  useLibraryServiceScanLibrary,
  useUserServiceListAllProfiles,
  getAdminSessionServiceListActiveSessionsQueryKey,
  eveyFetch,
  type V1ActiveSession,
} from "@evey/api";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { LoadFailed } from "../../../src/components/LoadFailed";
import { SessionCard } from "../../../src/components/admin/SessionCard";
import { BackgroundTasksPanel } from "../../../src/components/admin/BackgroundTasksPanel";

function StatCard({ label, value, icon, theme, sub, valueColor }: { label: string; value: string | number; icon: IoniconName; theme: ReturnType<typeof useTheme>; sub?: string; valueColor?: string }) {
  return (
    <View style={[styles.statCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
      <Ionicons name={icon} size={24} color={theme.colors.accent} />
      <Text style={[styles.statValue, { color: valueColor ?? theme.colors.text }]}>{value}</Text>
      <Text style={[styles.statLabel, { color: theme.colors.textMuted }]}>{label}</Text>
      {sub ? <Text style={[styles.statLabel, { color: theme.colors.textMuted, fontSize: 10 }]}>{sub}</Text> : null}
    </View>
  );
}

// formatBps mirrors the web dashboard's rendering: -1 means the server has no
// previous sample to diff against yet (first poll), so an em dash beats a
// misleading 0.
function formatBps(bps: string | number | undefined): string {
  const n = typeof bps === "string" ? parseInt(bps, 10) : (bps ?? 0);
  if (!Number.isFinite(n) || n < 0) return "-";
  if (n >= 1e9) return `${(n / 1e9).toFixed(2)} Gbps`;
  if (n >= 1e6) return `${(n / 1e6).toFixed(1)} Mbps`;
  if (n >= 1e3) return `${(n / 1e3).toFixed(0)} kbps`;
  return `${n} bps`;
}

function getUsageColor(pct: number, theme: ReturnType<typeof useTheme>): string {
  if (pct > 85) return theme.colors.danger;
  if (pct > 70) return theme.colors.warning;
  return theme.colors.success;
}

function deviceIconName(userAgent: string): IoniconName {
  const ua = userAgent.toLowerCase();
  if (ua.includes("eveytv")) return "tv-outline";
  if (ua.includes("eveymobile") || ua.includes("mobile") || ua.includes("android") || ua.includes("iphone")) return "phone-portrait-outline";
  return "desktop-outline";
}

function SectionHeader({ title, theme, onViewAll }: { title: string; theme: ReturnType<typeof useTheme>; onViewAll?: () => void }) {
  const { t } = useTranslation();
  return (
    <View style={styles.sectionHeaderRow}>
      <Text style={[styles.sectionTitle, { color: theme.colors.textMuted, marginTop: 0, marginBottom: 0 }]}>{title}</Text>
      {onViewAll && (
        <Pressable onPress={onViewAll}>
          <Text style={{ color: theme.colors.accent, fontSize: 12, fontWeight: "600" }}>{t("admin.dashboardStats.viewAll")}</Text>
        </Pressable>
      )}
    </View>
  );
}

// NowPlayingCard is the dashboard's mini control player for active playback:
// the same rich SessionCard the /admin/sessions screen uses, with live
// play/pause, stop and message controls (parity with the web dashboard).
function NowPlayingCard({ theme }: { theme: ReturnType<typeof useTheme> }) {
  const { t } = useTranslation();
  const router = useRouter();
  const qc = useQueryClient();
  const { data, isLoading, isError, refetch } = useAdminSessionServiceListActiveSessions({
    query: { refetchInterval: 5_000, refetchIntervalInBackground: false },
  });
  const sessions = data?.sessions ?? [];

  // Same live controls the Sessions screen has, via the shared SessionCard: the
  // dashboard's "Now playing" is a mini control player, not a read-only list
  // (parity with the web DashboardNowPlaying).
  const listKey = getAdminSessionServiceListActiveSessionsQueryKey();
  const killSession = useMutation({
    mutationFn: (sessionId: string) => eveyFetch({ url: `/api/v1/admin/sessions/${sessionId}`, method: "DELETE" }),
    onSuccess: () => {
      toast.success(t("admin.sessions.sessionStopped", "Session arrêtée"));
      void qc.invalidateQueries({ queryKey: listKey });
    },
    onError: () => toast.error(t("admin.sessions.failedStop", "Impossible d'arrêter la session")),
  });
  const sendCommand = useMutation({
    mutationFn: ({ sessionId, command }: { sessionId: string; command: string }) =>
      eveyFetch({ url: `/api/v1/admin/sessions/${sessionId}/command`, method: "POST", headers: { "Content-Type": "application/json" }, data: { command } }),
    onSuccess: () => { void qc.invalidateQueries({ queryKey: listKey }); },
    onError: () => toast.error(t("admin.sessions.commandFailed", "Échec de l'envoi de la commande")),
  });
  const handleKill = useCallback((s: V1ActiveSession) => {
    killSession.mutate(s.sessionId ?? "");
  }, [killSession]);
  const handleCommand = useCallback((sessionId: string, command: string) => {
    sendCommand.mutate({ sessionId, command });
  }, [sendCommand]);

  return (
    <View style={{ marginHorizontal: 16, marginTop: 20 }}>
      <SectionHeader title={t("admin.dashboardStats.nowPlaying")} theme={theme} onViewAll={() => router.push("/admin/sessions" as never)} />
      {isLoading ? (
        <View style={[styles.infoCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md, marginTop: 8 }]}>
          <ActivityIndicator color={theme.colors.accent} style={{ paddingVertical: 20 }} />
        </View>
      ) : isError ? (
        <View style={[styles.infoCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md, marginTop: 8 }]}>
          <LoadFailed inline onRetry={() => void refetch()} />
        </View>
      ) : sessions.length === 0 ? (
        <View style={[styles.infoCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md, marginTop: 8 }]}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 13, textAlign: "center", paddingVertical: 20 }}>
            {t("admin.dashboardStats.noActivePlayback")}
          </Text>
        </View>
      ) : (
        <View style={{ gap: 10, marginTop: 8 }}>
          {sessions.map((s, i) => (
            <SessionCard
              key={s.sessionId ?? i}
              session={s}
              theme={theme}
              t={t as never}
              onKill={handleKill}
              onCommand={handleCommand}
            />
          ))}
        </View>
      )}
    </View>
  );
}

// OnlineNowCard mirrors the web dashboard's "Who's Online" panel, one row per
// (user, device) pair since the presence tracker keys on user agent.
function OnlineNowCard({ theme }: { theme: ReturnType<typeof useTheme> }) {
  const { t } = useTranslation();
  const { data, isError, refetch } = usePresenceServiceListOnline({
    query: { refetchInterval: 5_000, refetchIntervalInBackground: false },
  });
  const users = data?.users ?? [];
  const total = data?.total ?? users.length;

  // Presence carries the profile ID but not its name; resolve it from the
  // profile list the admin screens already fetch, same as the web dashboard.
  // Absent (deleted profile, or a client that never sent one) shows nothing
  // extra rather than a raw id.
  const { data: profileData } = useUserServiceListAllProfiles();
  const profilesById = useMemo(() => {
    const map = new Map<string, { name?: string; isDefault?: boolean }>();
    for (const p of profileData?.profiles ?? []) {
      if (p.id) map.set(p.id, { name: p.name, isDefault: p.isDefault });
    }
    return map;
  }, [profileData]);

  function lastSeen(idleSeconds?: number): string {
    const idle = idleSeconds ?? 0;
    if (idle < 5) return t("admin.online.justNow");
    if (idle < 60) return t("admin.online.secondsAgo", { count: idle });
    return t("admin.online.minutesAgo", { count: Math.floor(idle / 60) });
  }

  // The household default profile is named after the account, which says
  // nothing: label it "Default" instead, the same word the profile manager
  // badges it with.
  function profileLabel(profileId?: string): string | undefined {
    if (!profileId) return undefined;
    const profile = profilesById.get(profileId);
    if (!profile) return undefined;
    return profile.isDefault ? t("auth.profile.manage.defaultProfileBadge") : profile.name;
  }

  return (
    <View style={{ marginHorizontal: 16, marginTop: 20 }}>
      <View style={styles.sectionHeaderRow}>
        <Text style={[styles.sectionTitle, { color: theme.colors.textMuted, marginTop: 0, marginBottom: 0 }]}>{t("admin.online.title")}</Text>
        <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
          {t(total === 1 ? "admin.online.countOne" : "admin.online.countOther", { count: total })}
        </Text>
      </View>
      <View style={[styles.infoCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md, marginTop: 8 }]}>
        {isError ? (
          <LoadFailed inline onRetry={() => void refetch()} />
        ) : users.length === 0 ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 13, textAlign: "center", paddingVertical: 20 }}>
            {t("admin.online.empty")}
          </Text>
        ) : (
          users.slice(0, 6).map((u, i) => {
            const label = profileLabel(u.profileId);
            return (
              <View key={`${u.userId}|${u.userAgent ?? i}`} style={[styles.infoRow, { borderBottomColor: theme.colors.border }]}>
                <Ionicons name={deviceIconName(u.userAgent ?? "")} size={16} color={theme.colors.textMuted} />
                <View style={{ flex: 1 }}>
                  <Text style={{ color: theme.colors.text, fontSize: 13 }} numberOfLines={1}>
                    {u.username || u.userId}
                    {u.isAdmin ? ` · ${t("admin.online.adminBadge")}` : ""}
                  </Text>
                  {label ? (
                    <Text style={{ color: theme.colors.textMuted, fontSize: 11, flex: 1 }} numberOfLines={1}>
                      {label}
                    </Text>
                  ) : null}
                </View>
                <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>{lastSeen(u.idleSeconds)}</Text>
              </View>
            );
          })
        )}
      </View>
    </View>
  );
}

// QuickActionsRow mirrors the web dashboard's quick actions - scan every
// library and a two-tap-confirm restart to avoid a modal for a rare op.
function QuickActionsRow({ theme }: { theme: ReturnType<typeof useTheme> }) {
  const { t } = useTranslation();
  const router = useRouter();
  const { data: libData } = useLibraryServiceListLibraries();
  const libraries = libData?.libraries ?? [];
  const [scanningAll, setScanningAll] = useState(false);
  const scanLibrary = useLibraryServiceScanLibrary({
    mutation: {
      onError: () => toast.error(t("admin.dashboardStats.scanFailed")),
    },
  });

  const [restartArmed, setRestartArmed] = useState(false);
  useEffect(() => {
    if (!restartArmed) return;
    const id = setTimeout(() => setRestartArmed(false), 4000);
    return () => clearTimeout(id);
  }, [restartArmed]);
  const restart = useAdminSystemServiceRestartServer({
    mutation: {
      onSuccess: () => toast.success(t("admin.dashboardStats.restartTriggered")),
      onError: (err: { code?: number; message?: string }) => {
        toast.error(err?.code === 14 ? t("admin.dashboardStats.restartUnavailable") : (err?.message || t("admin.dashboardStats.restartFailed")));
      },
    },
  });

  async function scanAll() {
    setScanningAll(true);
    let failed = 0;
    for (const lib of libraries) {
      if (lib.id) {
        try { await scanLibrary.mutateAsync({ id: lib.id }); } catch { failed++; }
      }
    }
    setScanningAll(false);
    if (failed > 0) toast.error(t("admin.dashboardStats.scanFailed"));
    else toast.success(t("admin.dashboardStats.allScansTriggered"));
  }

  function onRestartPress() {
    if (!restartArmed) {
      setRestartArmed(true);
      return;
    }
    setRestartArmed(false);
    restart.mutate({ data: {} });
  }

  return (
    <View style={{ marginHorizontal: 16, marginTop: 20 }}>
      <Text style={[styles.sectionTitle, { color: theme.colors.textMuted, marginTop: 0 }]}>{t("admin.dashboardStats.quickActions")}</Text>
      <View style={styles.quickActionsRow}>
        <Pressable
          onPress={scanAll}
          disabled={scanningAll || libraries.length === 0}
          style={[styles.quickActionBtn, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md, opacity: libraries.length === 0 ? 0.5 : 1 }]}
        >
          {scanningAll ? <ActivityIndicator size="small" color={theme.colors.accent} /> : <Ionicons name="search-outline" size={16} color={theme.colors.accent} />}
          <Text style={{ color: theme.colors.text, fontSize: 12, fontWeight: "600" }}>{t("admin.dashboardStats.scanAllLibraries")}</Text>
        </Pressable>
        <Pressable
          onPress={() => router.push("/admin/libraries" as never)}
          style={[styles.quickActionBtn, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
        >
          <Ionicons name="folder-outline" size={16} color={theme.colors.accent} />
          <Text style={{ color: theme.colors.text, fontSize: 12, fontWeight: "600" }}>{t("admin.dashboardStats.manageLibraries")}</Text>
        </Pressable>
        <Pressable
          onPress={() => router.push("/admin/users" as never)}
          style={[styles.quickActionBtn, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
        >
          <Ionicons name="people-outline" size={16} color={theme.colors.accent} />
          <Text style={{ color: theme.colors.text, fontSize: 12, fontWeight: "600" }}>{t("admin.dashboardStats.manageUsers")}</Text>
        </Pressable>
        <Pressable
          onPress={onRestartPress}
          style={[styles.quickActionBtn, { backgroundColor: restartArmed ? theme.colors.danger : theme.colors.surface, borderRadius: theme.radii.md }]}
        >
          {restart.isPending ? (
            <ActivityIndicator size="small" color={restartArmed ? theme.colors.onAccent : theme.colors.accent} />
          ) : (
            <Ionicons name="power-outline" size={16} color={restartArmed ? theme.colors.onAccent : theme.colors.accent} />
          )}
          <Text style={{ color: restartArmed ? theme.colors.onAccent : theme.colors.text, fontSize: 12, fontWeight: "600" }}>
            {restartArmed ? t("admin.dashboardStats.restartConfirm") : t("admin.dashboardStats.restartServer")}
          </Text>
        </Pressable>
      </View>
    </View>
  );
}

// SimpleBarChart is a plain-View bar chart (no react-native-svg needed for
// straight rectangles), used for the two time-series mini charts below.
function SimpleBarChart({ data, theme, maxBarHeight = 60 }: { data: { label: string; value: number }[]; theme: ReturnType<typeof useTheme>; maxBarHeight?: number }) {
  const maxVal = Math.max(...data.map((d) => d.value), 1);
  return (
    <View style={{ flexDirection: "row", alignItems: "flex-end", gap: 3, height: maxBarHeight + 16 }}>
      {data.map((d, i) => (
        <View key={i} style={{ flex: 1, alignItems: "center", gap: 2 }}>
          <View
            style={{
              width: "100%",
              height: Math.max(2, Math.round((d.value / maxVal) * maxBarHeight)),
              backgroundColor: theme.colors.accent,
              borderRadius: 2,
            }}
          />
        </View>
      ))}
    </View>
  );
}

// PlayActivityCard/PeakHoursCard are simplified read-only versions of the
// web dashboard's interactive bar charts (no mode toggle, no scroll-to-end).
function PlayActivityCard({ theme }: { theme: ReturnType<typeof useTheme> }) {
  const { t } = useTranslation();
  const { data } = useActivityServiceGetAnalytics({
    query: { refetchInterval: 60_000, refetchIntervalInBackground: false },
  });
  const daily = data?.dailyPlays ?? [];
  if (daily.length === 0) return null;

  const bars = daily.map((d) => {
    const parts = (d.date ?? "").split("-");
    const label = parts.length >= 3 ? `${parts[2]}/${parts[1]}` : (d.date ?? "");
    return { label, value: d.playCount ?? 0 };
  });

  return (
    <View style={{ marginHorizontal: 16, marginTop: 20 }}>
      <Text style={[styles.sectionTitle, { color: theme.colors.textMuted, marginTop: 0 }]}>{t("admin.dashboardStats.playActivity")}</Text>
      <View style={[styles.infoCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md, marginTop: 8, padding: 14 }]}>
        <SimpleBarChart data={bars} theme={theme} />
      </View>
    </View>
  );
}

function PeakHoursCard({ theme }: { theme: ReturnType<typeof useTheme> }) {
  const { t } = useTranslation();
  const { data } = useActivityServiceGetAnalytics({
    query: { refetchInterval: 60_000, refetchIntervalInBackground: false },
  });
  const hourly = data?.hourlyPlays ?? [];
  if (hourly.length === 0) return null;

  const bars = Array.from({ length: 24 }, (_, i) => {
    const entry = hourly.find((h) => h.hour === i);
    return { label: `${i}`, value: entry?.count ?? 0 };
  });

  return (
    <View style={{ marginHorizontal: 16, marginTop: 20 }}>
      <Text style={[styles.sectionTitle, { color: theme.colors.textMuted, marginTop: 0 }]}>{t("admin.dashboardStats.peakHours")}</Text>
      <View style={[styles.infoCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md, marginTop: 8, padding: 14 }]}>
        <SimpleBarChart data={bars} theme={theme} maxBarHeight={44} />
      </View>
    </View>
  );
}

export default function AdminDashboard() {
  const theme = useTheme();
  const { t } = useTranslation();
  const [refreshing, setRefreshing] = useState(false);

  const { data: dashboard, refetch: refetchDash } = useAdminSystemServiceGetDashboard({ query: { refetchInterval: 10_000, refetchIntervalInBackground: false } });
  const { data: moviesData, refetch: refetchMovies } = useLibraryServiceListMediaItems({
    type: "MEDIA_ITEM_TYPE_MOVIE" as never,
    "pagination.page": 1,
    "pagination.pageSize": 1,
  });
  const { data: seriesData, refetch: refetchSeries } = useLibraryServiceListMediaItems({
    type: "MEDIA_ITEM_TYPE_SERIES" as never,
    "pagination.page": 1,
    "pagination.pageSize": 1,
  });
  const movieCount = moviesData?.pagination?.totalItems ?? 0;
  const seriesCount = seriesData?.pagination?.totalItems ?? 0;

  const onRefresh = useCallback(async () => {
    setRefreshing(true);
    await Promise.all([refetchDash(), refetchMovies(), refetchSeries()]);
    setRefreshing(false);
  }, [refetchDash, refetchMovies, refetchSeries]);

  const cpu = dashboard?.cpuPercent;
  const mem = dashboard?.memoryPercent;
  const transcodes = dashboard?.activeTranscodes ?? 0;

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

      {/* Inventory on one line - storage, server details and the finer metrics
          live on System Health; popular media and the audit trail on Analytics
          and Activity. The dashboard is the live view (mirror of the web). */}
      <Text style={{ color: theme.colors.textMuted, fontSize: 13, marginHorizontal: 16, marginTop: 16 }}>
        {t("admin.dashboardStats.inventory", {
          movies: movieCount,
          series: seriesCount,
          media: dashboard?.totalMediaItems ?? 0,
          users: dashboard?.totalUsers ?? 0,
        })}
      </Text>

      <NowPlayingCard theme={theme} />
      <OnlineNowCard theme={theme} />

      {/* The four live headline metrics, same as the web dashboard. */}
      <View style={[styles.statsGrid, { marginTop: 20 }]}>
        <StatCard
          label={t("admin.dashboardStats.activeStreams")}
          value={dashboard?.activeStreams ?? 0}
          sub={transcodes > 0 ? t("admin.dashboardStats.transcodesSub", { count: transcodes }) : undefined}
          icon="play-outline"
          theme={theme}
        />
        <StatCard
          label={t("admin.dashboardStats.throughput")}
          value={formatBps(dashboard?.streamingBps)}
          sub={t("admin.dashboardStats.theoreticalSub", { rate: formatBps(dashboard?.activeSessionsBps) })}
          icon="pulse-outline"
          theme={theme}
        />
        <StatCard
          label={t("admin.dashboardStats.cpu")}
          value={cpu != null ? `${Math.round(cpu)}%` : "-"}
          valueColor={cpu != null ? getUsageColor(cpu, theme) : undefined}
          icon="hardware-chip-outline"
          theme={theme}
        />
        <StatCard
          label={t("admin.dashboardStats.memory")}
          value={mem != null ? `${Math.round(mem)}%` : "-"}
          valueColor={mem != null ? getUsageColor(mem, theme) : undefined}
          icon="server-outline"
          theme={theme}
        />
      </View>

      {/* What Evey is doing on its own, and its live cost. */}
      <View style={{ marginHorizontal: 16, marginTop: 20 }}>
        <BackgroundTasksPanel theme={theme} />
      </View>

      <QuickActionsRow theme={theme} />

      <PlayActivityCard theme={theme} />
      <PeakHoursCard theme={theme} />
      {/* Storage, server info, popular media and recent activity are NOT here
          on purpose: they live on System Health and Analytics (parity with the
          web dashboard's deliberate split). The drawer carries every section. */}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  statsGrid: { flexDirection: "row", flexWrap: "wrap", paddingHorizontal: 16, gap: 12 },
  statCard: { flex: 1, minWidth: "45%", padding: 16, alignItems: "center", gap: 4 },
  statValue: { fontSize: 28, fontWeight: "700" },
  statLabel: { fontSize: 12, textAlign: "center" },
  sectionTitle: { fontSize: 13, fontWeight: "600", textTransform: "uppercase", marginTop: 20, marginBottom: 8, marginHorizontal: 16 },
  sectionHeaderRow: { flexDirection: "row", justifyContent: "space-between", alignItems: "center" },
  infoCard: { overflow: "hidden" },
  infoRow: { flexDirection: "row", alignItems: "center", gap: 12, paddingVertical: 12, paddingHorizontal: 16, borderBottomWidth: StyleSheet.hairlineWidth },
  quickActionsRow: { flexDirection: "row", flexWrap: "wrap", gap: 8, marginTop: 8 },
  quickActionBtn: { flexDirection: "row", alignItems: "center", gap: 6, paddingVertical: 10, paddingHorizontal: 14 },
});
