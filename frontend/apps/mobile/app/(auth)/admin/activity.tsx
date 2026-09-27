import { View, Text, FlatList, Pressable, ActivityIndicator, StyleSheet, Share, Alert } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { Stack, router, useLocalSearchParams } from "expo-router";
import { useState, useCallback, useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import {
  useActivityServiceGetPlayHistory,
  useActivityServiceGetServerStats,
  useAdminSessionServiceGetAuditLog,
  activityServiceGetPlayHistory,
  activityServiceGetServerStats,
  adminSessionServiceGetAuditLog,
  collectAllPages,
  EXPORT_PAGE_SIZE,
  EXPORT_MAX_ROWS,
  eveyFetch,
  apiErrorMessage,
} from "@evey/api";
import { toast } from "@evey/store";
// Types from generated schemas - not re-exported from @evey/api index
interface PlayEntry {
  id?: string;
  mediaItemId?: string;
  mediaType?: string;
  mediaTitle?: string;
  durationMs?: string;
  positionMs?: string;
  deviceName?: string;
  clientName?: string;
  startedAt?: string;
  // Who watched, resolved server-side. An admin reading the server-wide
  // history needs a name, not a profile id.
  profileName?: string;
  username?: string;
}
interface AuditEntry {
  id?: string;
  username?: string;
  profileName?: string;
  action?: string;
  resourceType?: string;
  resourceId?: string;
  resourceName?: string;
  ipAddress?: string;
  deviceName?: string;
  userAgent?: string;
  details?: string;
  createdAt?: string;
}
import { useTheme } from "../../../src/providers/ThemeProvider";
import { LoadFailed } from "../../../src/components/LoadFailed";
import { WatcherChips, type Watcher } from "../../../src/features/admin/WatcherChips";
import { formatBytes, formatDurationMs } from "@evey/api";

type Tab = "play" | "audit" | "stats";

// The media page a row opens, when it is about a media item that has one.
function playEntryLink(e: PlayEntry): string | null {
  return e.mediaItemId && e.mediaType !== "MEDIA_ITEM_TYPE_TRACK" ? `/media/${e.mediaItemId}` : null;
}
function auditEntryLink(e: AuditEntry): string | null {
  return (e.resourceType === "media" || e.resourceType === "media_item") && e.resourceId ? `/media/${e.resourceId}` : null;
}

function timeAgo(d?: string): string {
  if (!d) return "-";
  const diff = Date.now() - new Date(d).getTime();
  const mins = Math.floor(diff / 60000);
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return `${hrs}h ago`;
  const days = Math.floor(hrs / 24);
  return `${days}d ago`;
}

function PlayHistoryTab({ watcher }: { watcher: Watcher }) {
  const theme = useTheme();
  const { t } = useTranslation();
  const PAGE_SIZE = 25;
  const [page, setPage] = useState(1);

  const { data, isLoading, isFetching, isError, refetch } = useActivityServiceGetPlayHistory({
    "pagination.page": page,
    "pagination.pageSize": PAGE_SIZE,
    ...(watcher.userId ? { userId: watcher.userId } : {}),
    ...(watcher.profileId ? { profileId: watcher.profileId } : {}),
  });
  // Pages are accumulated locally, like history.tsx. Reading straight off
  // `data` meant a failed page N emptied the whole list: the query returns
  // undefined on error, so everything already scrolled through vanished and
  // the screen fell back to its empty state.
  //
  // appliedPage makes the append idempotent. The effect depends on `data`, and
  // any render that hands back a fresh object for the SAME page would append it
  // again - a duplicated page at best, an endless render loop at worst.
  const [entries, setEntries] = useState<PlayEntry[]>([]);
  const appliedPage = useRef(0);
  useEffect(() => {
    const fresh = data?.entries;
    if (!fresh || appliedPage.current === page) return;
    appliedPage.current = page;
    setEntries((prev) => (page === 1 ? fresh : [...prev, ...fresh]));
  }, [data, page]);
  const total = data?.pagination?.totalItems ?? 0;
  const hasMore = page * PAGE_SIZE < total;

  const renderItem = useCallback(({ item }: { item: PlayEntry }) => {
    const pos = parseInt(item.positionMs ?? "0");
    const dur = parseInt(item.durationMs ?? "1");
    const pct = dur > 0 ? Math.round((pos / dur) * 100) : 0;
    const link = playEntryLink(item);

    return (
      <Pressable
        disabled={!link}
        onPress={() => { if (link) router.push(link as never); }}
        accessibilityRole={link ? "link" : undefined}
        style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
      >
        <View style={{ flex: 1, gap: 4 }}>
          <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600" }} numberOfLines={1}>
            {item.mediaTitle ?? "-"}
          </Text>
          <Text style={{ color: theme.colors.text, fontSize: 12, fontWeight: "600" }} numberOfLines={1}>
            {item.profileName || t("admin.activity.unknownProfile", "Deleted profile")}
            {item.username ? ` · ${item.username}` : ""}
          </Text>
          <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
            {item.deviceName ?? item.clientName ?? "-"} · {timeAgo(item.startedAt)}
          </Text>
          <View style={{ flexDirection: "row", alignItems: "center", gap: 8, marginTop: 2 }}>
            <View style={[styles.progressBar, { backgroundColor: theme.colors.border }]}>
              <View style={[styles.progressFill, { width: `${pct}%`, backgroundColor: theme.colors.accent }]} />
            </View>
            <Text style={{ color: theme.colors.textMuted, fontSize: 11, width: 32, textAlign: "right" }}>{pct}%</Text>
            <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{formatDurationMs(item.durationMs) || "0min"}</Text>
          </View>
        </View>
        {link ? <Ionicons name="chevron-forward" size={16} color={theme.colors.textMuted} /> : null}
      </Pressable>
    );
  }, [theme, t]);

  const loadMore = useCallback(() => {
    if (hasMore) setPage((p) => p + 1);
  }, [hasMore]);

  return (
    <FlatList
      data={entries}
      keyExtractor={(item, index) => item.id ?? String(index)}
      renderItem={renderItem}
      contentContainerStyle={{ padding: 16, gap: 8 }}
      refreshing={isFetching}
      onRefresh={() => {
        // Reset to the first page, else a refresh while scrolled past page 1
        // re-fetches the current page and appends it again (duplicate rows).
        appliedPage.current = -1;
        if (page === 1) void refetch();
        else setPage(1);
      }}
      onEndReached={loadMore}
      onEndReachedThreshold={0.5}
      ListFooterComponent={hasMore ? <ActivityIndicator color={theme.colors.accent} style={{ paddingVertical: 16 }} /> : null}
      ListEmptyComponent={
        isLoading ? null : isError ? (
          <LoadFailed onRetry={() => void refetch()} />
        ) : (
          <View style={{ alignItems: "center", paddingTop: 60 }}>
            <Ionicons name="film-outline" size={48} color={theme.colors.textMuted} />
            <Text style={{ color: theme.colors.textMuted, marginTop: 12 }}>{t("admin.activity.noPlayHistory", "No play history")}</Text>
          </View>
        )
      }
    />
  );
}

function AuditLogTab({ watcher }: { watcher: Watcher }) {
  const theme = useTheme();
  const { t } = useTranslation();
  const PAGE_SIZE = 25;
  const [page, setPage] = useState(0);

  const { data, isLoading, isFetching, isError, refetch } = useAdminSessionServiceGetAuditLog({
    limit: PAGE_SIZE,
    offset: page * PAGE_SIZE,
    ...(watcher.userId ? { userId: watcher.userId } : {}),
    ...(watcher.profileId ? { profileId: watcher.profileId } : {}),
  });
  // Same accumulation as the play-history tab above: this one pages by offset
  // and starts at 0, so the sentinel has to sit below it.
  const [entries, setEntries] = useState<AuditEntry[]>([]);
  const appliedPage = useRef(-1);
  useEffect(() => {
    const fresh = data?.entries;
    if (!fresh || appliedPage.current === page) return;
    appliedPage.current = page;
    setEntries((prev) => (page === 0 ? fresh : [...prev, ...fresh]));
  }, [data, page]);
  const total = data?.total ?? 0;
  const hasMore = (page + 1) * PAGE_SIZE < total;

  const renderItem = useCallback(({ item }: { item: AuditEntry }) => (
    <Pressable
      disabled={!auditEntryLink(item)}
      onPress={() => { const link = auditEntryLink(item); if (link) router.push(link as never); }}
      accessibilityRole={auditEntryLink(item) ? "link" : undefined}
      style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}
    >
      <View style={{ flex: 1, gap: 2 }}>
        <View style={{ flexDirection: "row", alignItems: "center", gap: 8 }}>
          <View style={[styles.badge, { backgroundColor: theme.colors.accent + "20" }]}>
            <Text style={{ color: theme.colors.accent, fontSize: 11, fontWeight: "600" }}>{item.action ?? "-"}</Text>
          </View>
          <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{timeAgo(item.createdAt)}</Text>
        </View>
        <Text style={{ color: theme.colors.text, fontSize: 13, marginTop: 4 }} numberOfLines={2}>
          {item.username ?? "system"}
          {/* The profile is what tells two people on one household account
              apart; server-side actions carry none. */}
          {item.profileName ? ` (${item.profileName})` : ""}
          {item.resourceType ? ` · ${item.resourceType}` : ""}
          {item.resourceName ? ` ${item.resourceName}` : ""}
        </Text>
        {(item.ipAddress || item.deviceName || item.userAgent) ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 11, marginTop: 1 }} numberOfLines={1}>
            {[item.ipAddress, item.deviceName || item.userAgent].filter(Boolean).join(" · ")}
          </Text>
        ) : null}
        {item.details ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginTop: 2 }} numberOfLines={2}>{item.details}</Text>
        ) : null}
      </View>
      {auditEntryLink(item) ? <Ionicons name="chevron-forward" size={16} color={theme.colors.textMuted} /> : null}
    </Pressable>
  ), [theme]);

  const loadMore = useCallback(() => {
    if (hasMore) setPage((p) => p + 1);
  }, [hasMore]);

  return (
    <FlatList
      data={entries}
      keyExtractor={(item, index) => item.id ?? String(index)}
      renderItem={renderItem}
      contentContainerStyle={{ padding: 16, gap: 8 }}
      refreshing={isFetching}
      onRefresh={() => {
        // Reset to the first page, else a refresh while scrolled past page 0
        // re-fetches the current page and appends it again (duplicate rows).
        appliedPage.current = -1;
        if (page === 0) void refetch();
        else setPage(0);
      }}
      onEndReached={loadMore}
      onEndReachedThreshold={0.5}
      ListFooterComponent={hasMore ? <ActivityIndicator color={theme.colors.accent} style={{ paddingVertical: 16 }} /> : null}
      ListEmptyComponent={
        isLoading ? null : isError ? (
          <LoadFailed onRetry={() => void refetch()} />
        ) : (
          <View style={{ alignItems: "center", paddingTop: 60 }}>
            <Ionicons name="document-text-outline" size={48} color={theme.colors.textMuted} />
            <Text style={{ color: theme.colors.textMuted, marginTop: 12 }}>{t("admin.activity.noAuditEntries", "No audit log entries")}</Text>
          </View>
        )
      }
    />
  );
}

type BgTask = { name: string; state: string; detail?: string; processed: number; lastActiveAt?: string };

// Mirrors the web BackgroundTasksCard: shows what the background workers are
// doing right now so an operator understands why the CPU isn't idle. Polls
// every 5s for a live-ish view.
function BackgroundTasksCard() {
  const theme = useTheme();
  const { t, i18n } = useTranslation();
  const { data } = useQuery<BgTask[]>({
    queryKey: ["/api/v1/admin/background-tasks"],
    queryFn: () => eveyFetch<BgTask[]>({ url: "/api/v1/admin/background-tasks", method: "GET" }),
    refetchInterval: 5000,
    refetchIntervalInBackground: false,
  });
  if (!data || data.length === 0) return null;

  return (
    <View style={[styles.section, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
      <Text style={[styles.sectionTitle, { color: theme.colors.text }]}>
        {t("admin.activity.backgroundTasks", "Background tasks")}
      </Text>
      {data.map((task) => {
        const running = task.state === "running";
        const label = t(`admin.activity.bgTask.${task.name}`, task.name);
        const helpKey = `admin.activity.bgTaskHelp.${task.name}`;
        const help = i18n.exists(helpKey) ? t(helpKey) : "";
        return (
          <View key={task.name} style={[styles.listRow, { borderBottomColor: theme.colors.border }]}>
            <View style={{ flexDirection: "row", alignItems: "center", gap: 8, flex: 1 }}>
              <View
                style={{
                  width: 8,
                  height: 8,
                  borderRadius: 4,
                  backgroundColor: running ? theme.colors.success : theme.colors.border,
                }}
              />
              <Text style={{ color: theme.colors.text, fontSize: 13, flexShrink: 1 }} numberOfLines={1}>
                {label}
                {running && task.detail ? ` · ${task.detail}` : ""}
              </Text>
              {help ? (
                <Pressable
                  onPress={() => Alert.alert(label, help)}
                  hitSlop={8}
                  accessibilityRole="button"
                  accessibilityLabel={t("admin.activity.bgTaskHelpLabel", { name: label })}
                >
                  <Ionicons name="information-circle-outline" size={15} color={theme.colors.textMuted} />
                </Pressable>
              ) : null}
              <View style={{ flex: 1 }} />
            </View>
            <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
              {running ? t("admin.activity.bgRunning", "running") : t("admin.activity.bgIdle", "idle")}
              {" · "}
              {t("admin.activity.bgProcessed", "{{n}} done", { n: task.processed })}
            </Text>
          </View>
        );
      })}
    </View>
  );
}

function StatsTab() {
  const theme = useTheme();
  const { t } = useTranslation();
  const { data, isLoading, refetch } = useActivityServiceGetServerStats();

  if (isLoading) {
    return (
      <View style={{ alignItems: "center", paddingTop: 60 }}>
        <ActivityIndicator color={theme.colors.accent} />
      </View>
    );
  }

  const cards: Array<{ label: string; value: string | number; icon: string }> = [
    { label: t("admin.activity.stats.totalPlays", "Total Plays"), value: data?.totalPlays ?? 0, icon: "play-outline" },
    { label: t("admin.activity.stats.totalPlayTime", "Play Time"), value: formatDurationMs(data?.totalPlayTimeMs) || "0min", icon: "time-outline" },
    { label: t("admin.activity.stats.totalUsers", "Users"), value: data?.totalUsers ?? 0, icon: "people-outline" },
    { label: t("admin.activity.stats.totalMedia", "Media Items"), value: data?.totalMediaItems ?? 0, icon: "film-outline" },
    { label: t("admin.activity.stats.bandwidth", "Bandwidth"), value: formatBytes(data?.totalBandwidthBytes), icon: "cloud-download-outline" },
    { label: t("admin.activity.stats.storage", "Storage"), value: formatBytes(data?.totalStorageBytes), icon: "server-outline" },
  ];

  return (
    <FlatList
      data={[1]}
      keyExtractor={() => "stats"}
      refreshing={false}
      onRefresh={refetch}
      renderItem={() => (
        <View style={{ padding: 16, gap: 16 }}>
          <View style={styles.statsGrid}>
            {cards.map((card) => (
              <View key={card.label} style={[styles.statCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
                <Ionicons name={card.icon as never} size={24} color={theme.colors.accent} />
                <Text style={[styles.statValue, { color: theme.colors.text }]}>{card.value}</Text>
                <Text style={[styles.statLabel, { color: theme.colors.textMuted }]}>{card.label}</Text>
              </View>
            ))}
          </View>

          <BackgroundTasksCard />

          {data?.popularMedia && data.popularMedia.length > 0 && (
            <View style={[styles.section, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
              <Text style={[styles.sectionTitle, { color: theme.colors.text }]}>
                {t("admin.activity.popularMedia", "Popular Media")}
              </Text>
              {data.popularMedia.map((m) => (
                <View key={m.mediaItemId} style={[styles.listRow, { borderBottomColor: theme.colors.border }]}>
                  <Text style={{ color: theme.colors.text, fontSize: 13, flex: 1 }} numberOfLines={1}>{m.title ?? "-"}</Text>
                  <Text style={{ color: theme.colors.textMuted, fontSize: 12, textAlign: "right" }}>
                    {t("admin.activity.plays", { count: m.playCount ?? 0, defaultValue: "{{count}} plays" })}
                    {(m.uniqueViewers ?? 0) > 0 ? `\n${t("admin.activity.viewers", { count: m.uniqueViewers, defaultValue: "{{count}} viewers" })}` : ""}
                  </Text>
                </View>
              ))}
            </View>
          )}

          {data?.activeUsers && data.activeUsers.length > 0 && (
            <View style={[styles.section, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
              <Text style={[styles.sectionTitle, { color: theme.colors.text }]}>
                {t("admin.activity.mostActiveUsers", "Most Active Users")}
              </Text>
              {data.activeUsers.map((u) => (
                <View key={u.userId} style={[styles.listRow, { borderBottomColor: theme.colors.border }]}>
                  <Text style={{ color: theme.colors.text, fontSize: 13, flex: 1 }} numberOfLines={1}>{u.username}</Text>
                  <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
                    {u.playCount} · {formatDurationMs(u.totalWatchTimeMs) || "0min"}
                  </Text>
                </View>
              ))}
            </View>
          )}
        </View>
      )}
    />
  );
}

export default function AdminActivityScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const [tab, setTab] = useState<Tab>("play");
  // Deep link from the Users screen ("History" on a profile), mirroring the
  // web's /admin/activity?profileId=. Landing on the unfiltered list and
  // making the admin pick the same profile again is what the button exists to
  // avoid. Initial state only: WatcherChips owns the filter from then on.
  const { profileId: linkedProfileId, userId: linkedUserId } =
    useLocalSearchParams<{ profileId?: string; userId?: string }>();
  const [watcher, setWatcher] = useState<Watcher>(() => ({
    ...(linkedUserId ? { userId: linkedUserId } : {}),
    ...(linkedProfileId ? { profileId: linkedProfileId } : {}),
  }));
  const watcherKey = `${watcher.userId ?? ""}|${watcher.profileId ?? ""}`;
  const [exporting, setExporting] = useState(false);

  const handleExport = useCallback(async () => {
    setExporting(true);
    try {
      const timestamp = new Date().toISOString().slice(0, 10);
      let data: unknown;
      let filename: string;
      if (tab === "play") {
        // Both exports used to ask for one page of 10000 rows. Neither
        // endpoint has ever served that - the server reduces it to 500 - so
        // the file was the first 500 rows wearing the name of the whole log.
        const collected = await collectAllPages(async (pageIndex) => {
          const page = await activityServiceGetPlayHistory({
            "pagination.page": pageIndex + 1,
            "pagination.pageSize": EXPORT_PAGE_SIZE,
            ...(watcher.userId ? { userId: watcher.userId } : {}),
            ...(watcher.profileId ? { profileId: watcher.profileId } : {}),
          });
          return { rows: page.entries ?? [], total: page.pagination?.totalItems };
        });
        if (collected.truncated) toast.error(t("admin.activity.exportTruncated", { max: EXPORT_MAX_ROWS, total: collected.total }));
        data = { entries: collected.rows, total: collected.total };
        filename = `play-history-${timestamp}.json`;
      } else if (tab === "audit") {
        const collected = await collectAllPages(async (pageIndex) => {
          const page = await adminSessionServiceGetAuditLog({
            limit: EXPORT_PAGE_SIZE,
            offset: pageIndex * EXPORT_PAGE_SIZE,
            ...(watcher.userId ? { userId: watcher.userId } : {}),
            ...(watcher.profileId ? { profileId: watcher.profileId } : {}),
          });
          return { rows: page.entries ?? [], total: page.total };
        });
        if (collected.truncated) toast.error(t("admin.activity.exportTruncated", { max: EXPORT_MAX_ROWS, total: collected.total }));
        data = { entries: collected.rows, total: collected.total };
        filename = `audit-log-${timestamp}.json`;
      } else {
        data = await activityServiceGetServerStats();
        filename = `server-stats-${timestamp}.json`;
      }
      await Share.share({ title: filename, message: JSON.stringify(data, null, 2) });
    } catch (err) {
      toast.error(apiErrorMessage(err as Error, t, t("common.export", "Export JSON")));
    }
    setExporting(false);
  }, [tab, t, watcher.userId, watcher.profileId]);

  const tabs: Array<{ id: Tab; label: string }> = [
    { id: "play", label: t("admin.activity.tabs.playHistory", "Play History") },
    { id: "audit", label: t("admin.audit.title", "Audit Log") },
    { id: "stats", label: t("admin.activity.tabs.serverStats", "Stats") },
  ];

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <Stack.Screen options={{
        headerShown: true,
        title: t("admin.activity.title", "Activity"),
        headerStyle: { backgroundColor: theme.colors.surface },
        headerTintColor: theme.colors.text,
        headerRight: () => (
          <Pressable
            onPress={handleExport}
            disabled={exporting}
            style={{ marginRight: 8, opacity: exporting ? 0.5 : 1 }}
            accessibilityRole="button"
            accessibilityLabel={t("common.export", "Export JSON")}
          >
            {exporting ? (
              <ActivityIndicator size="small" color={theme.colors.accent} />
            ) : (
              <Ionicons name="download-outline" size={22} color={theme.colors.accent} />
            )}
          </Pressable>
        ),
      }} />

      <View style={[styles.tabBar, { backgroundColor: theme.colors.surface, borderBottomColor: theme.colors.border }]}>
        {tabs.map(({ id, label }) => (
          <Pressable
            key={id}
            onPress={() => setTab(id)}
            style={[
              styles.tabBtn,
              {
                backgroundColor: tab === id ? theme.colors.accent : "transparent",
                borderRadius: theme.radii.sm,
              },
            ]}
          >
            <Text style={{ color: tab === id ? theme.colors.onAccent : theme.colors.textMuted, fontSize: 13, fontWeight: "600" }}>
              {label}
            </Text>
          </Pressable>
        ))}
      </View>

      {tab !== "stats" && <WatcherChips value={watcher} onChange={setWatcher} />}

      {/* Both tabs accumulate pages in local state. Keying them on the watcher
          remounts on a filter change, so the previous profile's rows cannot
          survive into the new list. */}
      {tab === "play" && <PlayHistoryTab key={watcherKey} watcher={watcher} />}
      {tab === "audit" && <AuditLogTab key={watcherKey} watcher={watcher} />}
      {tab === "stats" && <StatsTab />}
    </View>
  );
}

const styles = StyleSheet.create({
  tabBar: { flexDirection: "row", padding: 8, gap: 4, borderBottomWidth: StyleSheet.hairlineWidth },
  tabBtn: { flex: 1, paddingVertical: 8, alignItems: "center" },
  card: { padding: 14, gap: 8, flexDirection: "row", alignItems: "center" },
  badge: { paddingHorizontal: 8, paddingVertical: 2, borderRadius: 4 },
  progressBar: { flex: 1, height: 4, borderRadius: 2, overflow: "hidden" },
  progressFill: { height: 4, borderRadius: 2 },
  statsGrid: { flexDirection: "row", flexWrap: "wrap", gap: 12 },
  statCard: { flex: 1, minWidth: "45%", padding: 16, alignItems: "center", gap: 4 },
  statValue: { fontSize: 28, fontWeight: "700" },
  statLabel: { fontSize: 12, textAlign: "center" },
  section: { overflow: "hidden", padding: 16 },
  sectionTitle: { fontSize: 15, fontWeight: "600", marginBottom: 12 },
  listRow: { flexDirection: "row", justifyContent: "space-between", alignItems: "center", paddingVertical: 10, borderBottomWidth: StyleSheet.hairlineWidth },
});
