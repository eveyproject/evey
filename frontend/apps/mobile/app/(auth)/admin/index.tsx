import { View, Text, ScrollView, Pressable, RefreshControl, ActivityIndicator, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";

// The icon-name union, so a typo is a compile error rather than a blank square.
type IoniconName = keyof typeof Ionicons.glyphMap;
import { Stack, useRouter } from "expo-router";
import { useState, useCallback, useEffect } from "react";
import { useTranslation } from "react-i18next";
import Svg, { Circle } from "react-native-svg";
import {
  useAdminSystemServiceGetDashboard,
  useAdminSystemServiceGetSystemInfo,
  useAdminSystemServiceRestartServer,
  useAdminSessionServiceListActiveSessions,
  useAdminSessionServiceGetAuditLog,
  usePresenceServiceListOnline,
  useActivityServiceGetServerStats,
  useActivityServiceGetAnalytics,
  useLibraryServiceListMediaItems,
  useLibraryServiceListLibraries,
  useLibraryServiceScanLibrary,
} from "@evey/api";
import { toast } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { usePluginSlotEntries } from "../../../src/features/plugins/PluginSlot";
import { LoadFailed } from "../../../src/components/LoadFailed";
import { formatBytes } from "@evey/api";

function StatCard({ label, value, icon, theme }: { label: string; value: string | number; icon: IoniconName; theme: ReturnType<typeof useTheme> }) {
  return (
    <View style={[styles.statCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
      <Ionicons name={icon} size={24} color={theme.colors.accent} />
      <Text style={[styles.statValue, { color: theme.colors.text }]}>{value}</Text>
      <Text style={[styles.statLabel, { color: theme.colors.textMuted }]}>{label}</Text>
    </View>
  );
}

function formatUptime(seconds?: string): string {
  if (!seconds) return "-";
  const s = parseInt(seconds, 10);
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
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

// NowPlayingCard is a compact read-only summary of active playback sessions -
// full transport controls (pause/kill/message) already live on /admin/sessions.
function NowPlayingCard({ theme }: { theme: ReturnType<typeof useTheme> }) {
  const { t } = useTranslation();
  const router = useRouter();
  const { data, isLoading, isError, refetch } = useAdminSessionServiceListActiveSessions({
    query: { refetchInterval: 5_000, refetchIntervalInBackground: false },
  });
  const sessions = data?.sessions ?? [];

  return (
    <View style={{ marginHorizontal: 16, marginTop: 20 }}>
      <SectionHeader title={t("admin.dashboardStats.nowPlaying")} theme={theme} onViewAll={() => router.push("/admin/sessions" as never)} />
      <View style={[styles.infoCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md, marginTop: 8 }]}>
        {isLoading ? (
          <ActivityIndicator color={theme.colors.accent} style={{ paddingVertical: 20 }} />
        ) : isError ? (
          <LoadFailed inline onRetry={() => void refetch()} />
        ) : sessions.length === 0 ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 13, textAlign: "center", paddingVertical: 20 }}>
            {t("admin.dashboardStats.noActivePlayback")}
          </Text>
        ) : (
          sessions.slice(0, 4).map((s, i) => {
            const progress = Math.min(100, Math.max(0, s.progress ?? 0));
            const epLabel = s.seriesTitle && (s.seasonNumber ?? 0) > 0 && (s.episodeNumber ?? 0) > 0
              ? `S${String(s.seasonNumber).padStart(2, "0")}E${String(s.episodeNumber).padStart(2, "0")} - `
              : "";
            return (
              <View key={s.sessionId ?? i} style={[styles.infoRow, { borderBottomColor: theme.colors.border, flexDirection: "column", alignItems: "stretch", gap: 4 }]}>
                <View style={{ flexDirection: "row", justifyContent: "space-between" }}>
                  <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600", flex: 1 }} numberOfLines={1}>
                    {epLabel}{s.mediaTitle ?? "-"}
                  </Text>
                  <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>{s.profileName ?? "-"}</Text>
                </View>
                <View style={[styles.progressBg, { backgroundColor: theme.colors.border }]}>
                  <View style={[styles.progressFill, { width: `${progress}%`, backgroundColor: theme.colors.accent }]} />
                </View>
              </View>
            );
          })
        )}
      </View>
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

  function lastSeen(idleSeconds?: number): string {
    const idle = idleSeconds ?? 0;
    if (idle < 5) return t("admin.online.justNow");
    if (idle < 60) return t("admin.online.secondsAgo", { count: idle });
    return t("admin.online.minutesAgo", { count: Math.floor(idle / 60) });
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
          users.slice(0, 6).map((u, i) => (
            <View key={`${u.userId}|${u.userAgent ?? i}`} style={[styles.infoRow, { borderBottomColor: theme.colors.border }]}>
              <Ionicons name={deviceIconName(u.userAgent ?? "")} size={16} color={theme.colors.textMuted} />
              <Text style={{ color: theme.colors.text, fontSize: 13, flex: 1 }} numberOfLines={1}>
                {u.username || u.userId}
                {u.isAdmin ? ` · ${t("admin.online.adminBadge")}` : ""}
              </Text>
              <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>{lastSeen(u.idleSeconds)}</Text>
            </View>
          ))
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

// PopularMediaCard reuses the same server-stats endpoint the web dashboard's
// PopularMedia widget calls (top played items over the last 90 days).
function PopularMediaCard({ theme }: { theme: ReturnType<typeof useTheme> }) {
  const { t } = useTranslation();
  const router = useRouter();
  const { data } = useActivityServiceGetServerStats({ popularLimit: 5 });
  const popular = data?.popularMedia ?? [];
  if (popular.length === 0) return null;

  return (
    <View style={{ marginHorizontal: 16, marginTop: 20 }}>
      <Text style={[styles.sectionTitle, { color: theme.colors.textMuted, marginTop: 0 }]}>{t("admin.dashboardStats.popularMedia")}</Text>
      <View style={[styles.infoCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md, marginTop: 8 }]}>
        {popular.map((m, i) => (
          <Pressable
            key={m.mediaItemId ?? i}
            onPress={() => m.mediaItemId && router.push(`/media/${m.mediaItemId}` as never)}
            style={[styles.infoRow, { borderBottomColor: theme.colors.border }]}
          >
            <Text style={{ color: theme.colors.accent, fontSize: 13, fontWeight: "700", width: 20 }}>{i + 1}</Text>
            <Text style={{ color: theme.colors.text, fontSize: 13, flex: 1 }} numberOfLines={1}>{m.title ?? "-"}</Text>
            <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
              {t("admin.activity.plays", { count: m.playCount ?? 0, defaultValue: "{{count}} plays" })}
            </Text>
          </Pressable>
        ))}
      </View>
    </View>
  );
}

// RecentActivityCard is a compact read-only preview of the audit log - full
// filtering/pagination lives on /admin/activity.
function RecentActivityCard({ theme }: { theme: ReturnType<typeof useTheme> }) {
  const { t } = useTranslation();
  const router = useRouter();
  const { data } = useAdminSessionServiceGetAuditLog({ limit: 5, offset: 0 });
  const entries = data?.entries ?? [];
  if (entries.length === 0) return null;

  return (
    <View style={{ marginHorizontal: 16, marginTop: 20 }}>
      <SectionHeader title={t("admin.dashboardStats.recentActivity")} theme={theme} onViewAll={() => router.push("/admin/activity" as never)} />
      <View style={[styles.infoCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md, marginTop: 8 }]}>
        {entries.map((entry, i) => (
          <View key={entry.id ?? i} style={[styles.infoRow, { borderBottomColor: theme.colors.border }]}>
            <View style={{ flex: 1, gap: 2 }}>
              <Text style={{ color: theme.colors.text, fontSize: 13 }} numberOfLines={1}>
                {entry.username || "system"}{entry.resourceType ? ` · ${entry.resourceType}` : ""}
              </Text>
              <Text style={{ color: theme.colors.textMuted, fontSize: 11 }} numberOfLines={1}>{entry.action ?? "-"}</Text>
            </View>
            <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
              {entry.createdAt ? new Date(entry.createdAt).toLocaleTimeString() : "-"}
            </Text>
          </View>
        ))}
      </View>
    </View>
  );
}

// StorageDonut draws one ring per physical disk (react-native-svg is already
// a project dependency, used elsewhere for the player's scrub timeline).
function StorageDonut({ storage, theme }: { storage: { path?: string; totalBytes?: string; usedBytes?: string }[]; theme: ReturnType<typeof useTheme> }) {
  const { t } = useTranslation();
  if (!storage || storage.length === 0) return null;

  const radius = 34;
  const strokeWidth = 8;
  const circumference = 2 * Math.PI * radius;

  return (
    <View style={{ marginHorizontal: 16, marginTop: 20 }}>
      <Text style={[styles.sectionTitle, { color: theme.colors.textMuted, marginTop: 0 }]}>{t("admin.dashboardStats.storage")}</Text>
      <View style={[styles.infoCard, styles.storageRow, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md, marginTop: 8 }]}>
        {storage.map((s, i) => {
          const used = parseInt(s.usedBytes ?? "0", 10);
          const total = parseInt(s.totalBytes ?? "1", 10);
          const pct = total > 0 ? Math.round((used / total) * 100) : 0;
          const dash = circumference * Math.min(1, Math.max(0, pct / 100));
          return (
            <View key={s.path ?? i} style={{ alignItems: "center", gap: 4, width: 100 }}>
              <View style={{ width: radius * 2 + strokeWidth, height: radius * 2 + strokeWidth }}>
                <Svg width={radius * 2 + strokeWidth} height={radius * 2 + strokeWidth}>
                  <Circle
                    cx={radius + strokeWidth / 2}
                    cy={radius + strokeWidth / 2}
                    r={radius}
                    stroke={theme.colors.border}
                    strokeWidth={strokeWidth}
                    fill="none"
                  />
                  <Circle
                    cx={radius + strokeWidth / 2}
                    cy={radius + strokeWidth / 2}
                    r={radius}
                    stroke={theme.colors.accent}
                    strokeWidth={strokeWidth}
                    fill="none"
                    strokeDasharray={`${dash} ${circumference}`}
                    strokeLinecap="round"
                    rotation="-90"
                    origin={`${radius + strokeWidth / 2}, ${radius + strokeWidth / 2}`}
                  />
                </Svg>
                <View style={StyleSheet.absoluteFillObject}>
                  <View style={{ flex: 1, alignItems: "center", justifyContent: "center" }}>
                    <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "700" }}>{pct}%</Text>
                  </View>
                </View>
              </View>
              <Text style={{ color: theme.colors.textMuted, fontSize: 11 }} numberOfLines={1}>{s.path}</Text>
              <Text style={{ color: theme.colors.textMuted, fontSize: 10 }}>{formatBytes(s.usedBytes)} / {formatBytes(s.totalBytes)}</Text>
            </View>
          );
        })}
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
  const router = useRouter();
  const [refreshing, setRefreshing] = useState(false);

  const pluginEntries = usePluginSlotEntries("admin.nav");

  const { data: dashboard, refetch: refetchDash } = useAdminSystemServiceGetDashboard({ query: { refetchInterval: 10_000, refetchIntervalInBackground: false } });
  const { data: sysInfo, refetch: refetchSys } = useAdminSystemServiceGetSystemInfo();
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
    await Promise.all([refetchDash(), refetchSys(), refetchMovies(), refetchSeries()]);
    setRefreshing(false);
  }, [refetchDash, refetchSys, refetchMovies, refetchSeries]);

  // Grouped exactly as the web sidebar groups them (features/admin/AdminNav.tsx),
  // using the six heading keys that already existed in admin.yaml and that only
  // the web ever used. Mobile listed all twenty entries in one block under the
  // heading "Monitoring" - not merely flat, but mislabelled: Users, Backup and
  // Plugins were all filed under monitoring.
  //
  // Feeds is the one entry web's sidebar does not carry (it folds feed config
  // into server-config, which is an open product question). Filed under Media
  // here because that is what its rows list.
  const navSections = [
    {
      title: t("admin.nav.dashboardSystem"),
      items: [
        { label: t("admin.sessions.title"), icon: "play-circle-outline", path: "/admin/sessions" },
        { label: t("admin.remoteAccess.title"), icon: "globe-outline", path: "/admin/remote-access" },
        { label: t("admin.serverConfig.title", "Server Config"), icon: "server-outline", path: "/admin/server-config" },
        { label: t("admin.nav.transcodingConfig", "Transcoding"), icon: "hardware-chip-outline", path: "/admin/transcoding" },
        { label: t("admin.systemHealth.title"), icon: "pulse-outline", path: "/admin/system-health" },
      ],
    },
    {
      title: t("admin.nav.media"),
      items: [
        { label: t("admin.nav.libraries"), icon: "library-outline", path: "/admin/libraries" },
        { label: t("admin.transcodeCache.title", "Transcode Cache"), icon: "film-outline", path: "/admin/transcode-cache" },
        { label: t("admin.artworks.title", "Artworks"), icon: "image-outline", path: "/admin/artworks" },
        { label: t("admin.nav.seekIndex"), icon: "construct-outline", path: "/admin/seek-index" },
        { label: t("common.feeds.title"), icon: "grid-outline", path: "/admin/feeds" },
      ],
    },
    {
      title: t("admin.nav.usersAccess"),
      items: [
        { label: t("admin.users.title"), icon: "people-outline", path: "/admin/users" },
        { label: t("admin.nav.mediaRequests"), icon: "mail-open-outline", path: "/admin/media-requests" },
      ],
    },
    {
      title: t("admin.nav.integrations"),
      items: [
        { label: t("admin.nav.plugins", "Plugins"), icon: "extension-puzzle-outline", path: "/admin/plugins" },
        { label: t("admin.nav.webhooks", "Webhooks"), icon: "link-outline", path: "/admin/webhooks" },
        { label: t("admin.nav.apiKeys", "API Keys"), icon: "key-outline", path: "/admin/api-keys" },
      ],
    },
    {
      title: t("admin.nav.monitoring"),
      items: [
        { label: t("admin.analytics.title"), icon: "bar-chart-outline", path: "/admin/analytics" },
        { label: t("admin.activity.title", "Activity"), icon: "analytics-outline", path: "/admin/activity" },
        { label: t("admin.nav.logs", "Logs & Debug"), icon: "document-text-outline", path: "/admin/logs" },
        { label: t("admin.nav.qoe"), icon: "speedometer-outline", path: "/admin/qoe" },
      ],
    },
    {
      title: t("admin.nav.maintenance"),
      items: [
        { label: t("admin.backup.title", "Database Backup"), icon: "cloud-download-outline", path: "/admin/backup" },
      ],
    },
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

      <NowPlayingCard theme={theme} />
      <OnlineNowCard theme={theme} />

      {/* Stats grid */}
      <View style={[styles.statsGrid, { marginTop: 20 }]}>
        <StatCard label={t("admin.dashboardStats.movies")} value={movieCount} icon="film-outline" theme={theme} />
        <StatCard label={t("admin.dashboardStats.series")} value={seriesCount} icon="tv-outline" theme={theme} />
        <StatCard label={t("admin.systemHealth.totalUsers")} value={dashboard?.totalUsers ?? 0} icon="people-outline" theme={theme} />
        <StatCard label={t("admin.systemHealth.activeStreams")} value={dashboard?.activeStreams ?? 0} icon="play-outline" theme={theme} />
        <StatCard label={t("admin.systemHealth.activeTranscodes")} value={dashboard?.activeTranscodes ?? 0} icon="swap-horizontal-outline" theme={theme} />
        <StatCard label={t("admin.systemHealth.totalMedia")} value={dashboard?.totalMediaItems ?? 0} icon="albums-outline" theme={theme} />
        <StatCard label={t("admin.dashboardStats.throughput")} value={formatBps(dashboard?.streamingBps)} icon="pulse-outline" theme={theme} />
        <StatCard label={t("admin.dashboardStats.theoretical")} value={formatBps(dashboard?.activeSessionsBps)} icon="speedometer-outline" theme={theme} />
      </View>

      <StorageDonut storage={dashboard?.storage ?? []} theme={theme} />

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
            <Text style={{ color: theme.colors.text, fontSize: 13, flex: 1, textAlign: "right" }} numberOfLines={1}>{row.value ?? "-"}</Text>
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
                <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>{t("admin.dashboardStats.cpu")}</Text>
                <Text style={{ color: getUsageColor(dashboard.cpuPercent, theme), fontSize: 13 }}>{Math.round(dashboard.cpuPercent)}%</Text>
              </View>
            )}
            {dashboard?.memoryPercent != null && (
              <View style={[styles.infoRow, { borderBottomColor: theme.colors.border }]}>
                <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>{t("admin.dashboardStats.memory")}</Text>
                <Text style={{ color: getUsageColor(dashboard.memoryPercent, theme), fontSize: 13 }}>{Math.round(dashboard.memoryPercent)}%</Text>
              </View>
            )}
          </View>
        </>
      )}

      <QuickActionsRow theme={theme} />

      <PlayActivityCard theme={theme} />
      <PeakHoursCard theme={theme} />

      <PopularMediaCard theme={theme} />
      <RecentActivityCard theme={theme} />

      {/* Navigation */}
      {navSections.map((section) => (
        <View key={section.title}>
          <Text style={[styles.sectionTitle, { color: theme.colors.textMuted }]}>{section.title}</Text>
          <View style={[styles.infoCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md, marginHorizontal: 16 }]}>
            {section.items.map((item) => (
              <Pressable key={item.path} onPress={() => router.push(item.path as never)} style={[styles.navRow, { borderBottomColor: theme.colors.border }]}>
                <Ionicons name={item.icon as never} size={20} color={theme.colors.accent} />
                <Text style={{ color: theme.colors.text, fontSize: 15, flex: 1 }}>{item.label}</Text>
                <Ionicons name="chevron-forward" size={16} color={theme.colors.textMuted} />
              </Pressable>
            ))}
          </View>
        </View>
      ))}
      {/* Plugin-contributed admin entries (slot admin.nav) - each granted
          plugin gets a menu row linking to its own dedicated screen
          (/admin/extensions/<id>), mobile parity with web. Its own block under
          Integrations, and only when there is something in it: a heading over
          an empty card reads as a section that failed to load. */}
      {pluginEntries.length > 0 ? (
        <>
      <Text style={[styles.sectionTitle, { color: theme.colors.textMuted }]}>{t("admin.nav.integrations")}</Text>
      <View style={[styles.infoCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md, marginHorizontal: 16 }]}>
        {pluginEntries.map((entry) => (
          <Pressable
            key={entry.pluginId}
            onPress={() => router.push(`/admin/extensions/${entry.pluginId}` as never)}
            style={[styles.navRow, { borderBottomColor: theme.colors.border }]}
          >
            <Ionicons name="extension-puzzle-outline" size={20} color={theme.colors.accent} />
            <Text style={{ color: theme.colors.text, fontSize: 15, flex: 1 }}>{entry.name}</Text>
            <Ionicons name="chevron-forward" size={16} color={theme.colors.textMuted} />
          </Pressable>
        ))}
      </View>
        </>
      ) : null}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  statsGrid: { flexDirection: "row", flexWrap: "wrap", paddingHorizontal: 16, gap: 12 },
  statCard: { flex: 1, minWidth: "45%", padding: 16, alignItems: "center", gap: 4 },
  statValue: { fontSize: 28, fontWeight: "700" },
  statLabel: { fontSize: 12 },
  sectionTitle: { fontSize: 13, fontWeight: "600", textTransform: "uppercase", marginTop: 20, marginBottom: 8, marginHorizontal: 16 },
  sectionHeaderRow: { flexDirection: "row", justifyContent: "space-between", alignItems: "center" },
  infoCard: { overflow: "hidden" },
  infoRow: { flexDirection: "row", alignItems: "center", gap: 12, paddingVertical: 12, paddingHorizontal: 16, borderBottomWidth: StyleSheet.hairlineWidth },
  navRow: { flexDirection: "row", alignItems: "center", gap: 12, paddingVertical: 14, paddingHorizontal: 16, borderBottomWidth: StyleSheet.hairlineWidth },
  progressBg: { height: 4, borderRadius: 2, overflow: "hidden" },
  progressFill: { height: 4, borderRadius: 2 },
  quickActionsRow: { flexDirection: "row", flexWrap: "wrap", gap: 8, marginTop: 8 },
  quickActionBtn: { flexDirection: "row", alignItems: "center", gap: 6, paddingVertical: 10, paddingHorizontal: 14 },
  storageRow: { flexDirection: "row", flexWrap: "wrap", justifyContent: "space-around", paddingVertical: 16 },
});
