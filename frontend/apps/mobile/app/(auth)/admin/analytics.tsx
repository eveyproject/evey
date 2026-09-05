import { useState, useCallback } from "react";
import { View, Text, ScrollView, RefreshControl, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { LoadFailed } from "../../../src/components/LoadFailed";
import { useActivityServiceGetAnalytics, formatDurationSec } from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";

interface TopMedia {
  mediaItemId?: string;
  title?: string;
  playCount?: number;
  totalDurationSeconds?: string;
}

interface RecentActivity {
  userId?: string;
  username?: string;
  mediaTitle?: string;
  playedAt?: string;
}

function formatDate(d?: string): string {
  if (!d) return "-";
  return new Date(d).toLocaleString();
}

function StatCard({ label, value, icon, theme }: { label: string; value: number; icon: string; theme: ReturnType<typeof useTheme> }) {
  return (
    <View style={[styles.statCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
      <Ionicons name={icon as never} size={24} color={theme.colors.accent} />
      <Text style={[styles.statValue, { color: theme.colors.text }]}>{value}</Text>
      <Text style={[styles.statLabel, { color: theme.colors.textMuted }]}>{label}</Text>
    </View>
  );
}

function TopMediaList({ items, theme }: { items: TopMedia[]; theme: ReturnType<typeof useTheme> }) {
  const { t } = useTranslation();
  // Bar reflects watch time, not play count: a media replayed briefly many
  // times shouldn't outrank one watched once, in full, for hours.
  const maxDuration = Math.max(...items.map((i) => parseInt(i.totalDurationSeconds ?? "0", 10)), 1);

  if (items.length === 0) {
    return (
      <View style={{ alignItems: "center", paddingVertical: 24 }}>
        <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>{t("admin.analytics.noPlays")}</Text>
      </View>
    );
  }

  return (
    <View style={[styles.listCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
      {items.map((item, i) => {
        const duration = parseInt(item.totalDurationSeconds ?? "0", 10);
        return (
          <View key={item.mediaItemId ?? i} style={[styles.listRow, { borderBottomColor: theme.colors.border }]}>
            <View style={{ flex: 1, gap: 4 }}>
              <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "500" }} numberOfLines={1}>
                {item.title ?? "-"}
              </Text>
              <View style={{ flexDirection: "row", alignItems: "center", gap: 8 }}>
                <View style={[styles.barBg, { backgroundColor: theme.colors.border }]}>
                  <View style={[styles.barFill, { backgroundColor: theme.colors.accent, width: `${Math.round((duration / maxDuration) * 100)}%` }]} />
                </View>
                <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
                  {formatDurationSec(item.totalDurationSeconds) || "0min"}
                </Text>
              </View>
            </View>
            <Text style={{ color: theme.colors.textMuted, fontSize: 11, width: 40, textAlign: "right" }}>
              {item.playCount ?? 0}
            </Text>
          </View>
        );
      })}
    </View>
  );
}

function RecentActivityList({ entries, theme }: { entries: RecentActivity[]; theme: ReturnType<typeof useTheme> }) {
  const { t } = useTranslation();

  if (entries.length === 0) {
    return (
      <View style={{ alignItems: "center", paddingVertical: 24 }}>
        <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>{t("admin.analytics.noActivity")}</Text>
      </View>
    );
  }

  return (
    <View style={[styles.listCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
      {entries.map((entry, i) => (
        <View key={`${entry.userId}-${entry.playedAt}-${i}`} style={[styles.listRow, { borderBottomColor: theme.colors.border }]}>
          <Ionicons name="person-outline" size={16} color={theme.colors.textMuted} />
          <View style={{ flex: 1, gap: 2 }}>
            <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "500" }} numberOfLines={1}>
              {entry.username ?? "-"}
            </Text>
            <Text style={{ color: theme.colors.textMuted, fontSize: 12 }} numberOfLines={1}>
              {entry.mediaTitle ?? "-"}
            </Text>
          </View>
          <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
            {formatDate(entry.playedAt)}
          </Text>
        </View>
      ))}
    </View>
  );
}

export default function AdminAnalyticsScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const [refreshing, setRefreshing] = useState(false);

  const { data, isLoading, refetch } = useActivityServiceGetAnalytics({
    query: { refetchInterval: 60_000, refetchIntervalInBackground: false },
  });

  const onRefresh = useCallback(async () => {
    setRefreshing(true);
    await refetch();
    setRefreshing(false);
  }, [refetch]);

  return (
    <ScrollView
      style={{ flex: 1, backgroundColor: theme.colors.bg }}
      contentContainerStyle={{ paddingBottom: 40 }}
      refreshControl={<RefreshControl refreshing={refreshing} onRefresh={onRefresh} tintColor={theme.colors.accent} />}
    >
      <Stack.Screen options={{
        headerShown: true,
        title: t("admin.analytics.title"),
        headerStyle: { backgroundColor: theme.colors.surface },
        headerTintColor: theme.colors.text,
      }} />

      {isLoading ? (
        <View style={{ alignItems: "center", paddingTop: 60 }}>
          <Ionicons name="hourglass-outline" size={32} color={theme.colors.textMuted} />
        </View>
      ) : data ? (
        <>
          <View style={styles.statsGrid}>
            <StatCard label={t("admin.analytics.playsToday")} value={data.totalPlaysToday ?? 0} icon="play-outline" theme={theme} />
            <StatCard label={t("admin.analytics.playsThisWeek")} value={data.totalPlaysThisWeek ?? 0} icon="calendar-outline" theme={theme} />
            <StatCard label={t("admin.analytics.activeUsersToday")} value={data.activeUsersToday ?? 0} icon="people-outline" theme={theme} />
          </View>

          <Text style={[styles.sectionTitle, { color: theme.colors.textMuted }]}>
            {t("admin.analytics.topMedia")}
          </Text>
          <View style={{ marginHorizontal: 16 }}>
            <TopMediaList items={data.topMedia ?? []} theme={theme} />
          </View>

          <Text style={[styles.sectionTitle, { color: theme.colors.textMuted }]}>
            {t("admin.analytics.recentActivity")}
          </Text>
          <View style={{ marginHorizontal: 16 }}>
            <RecentActivityList entries={data.recentActivity ?? []} theme={theme} />
          </View>
        </>
      ) : (
        // This screen carried its own copy of the failure block, without a way
        // to retry - the one case LoadFailed's own comment says to migrate when
        // next touched.
        <LoadFailed onRetry={() => void refetch()} message={t("admin.analytics.failedLoad")} />
      )}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  statsGrid: { flexDirection: "row", flexWrap: "wrap", padding: 16, gap: 12 },
  statCard: { flex: 1, minWidth: "45%", padding: 16, alignItems: "center", gap: 4 },
  statValue: { fontSize: 28, fontWeight: "700" },
  statLabel: { fontSize: 12, textAlign: "center" },
  sectionTitle: { fontSize: 13, fontWeight: "600", textTransform: "uppercase", marginTop: 20, marginBottom: 8, marginHorizontal: 16 },
  listCard: { overflow: "hidden" },
  listRow: { flexDirection: "row", alignItems: "center", gap: 12, paddingVertical: 12, paddingHorizontal: 16, borderBottomWidth: StyleSheet.hairlineWidth },
  barBg: { flex: 1, height: 4, borderRadius: 2 },
  barFill: { height: 4, borderRadius: 2 },
});
