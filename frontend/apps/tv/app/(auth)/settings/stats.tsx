import { useMemo } from "react";
import { View, Text, ScrollView, StyleSheet } from "react-native";
import { Stack, router } from "expo-router";
import { useTranslation } from "react-i18next";
import { useActivityServiceGetUserStats } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { FocusableCard } from "../../../src/components/FocusableCard";
import { ScreenState } from "../../../src/components/ScreenState";
import { DEEP_SCREEN_GUTTER } from "../../../src/lib/tvLayout";

// "My statistics", the TV counterpart of web's /profile/stats and mobile's
// settings/stats. Same endpoint, same numbers, same i18n keys.
//
// The one deliberate difference is the genre breakdown: web and mobile draw a
// pie of eight slices, which is unreadable across a living room and would cost
// the TV app a charting dependency it does not otherwise carry. Proportional
// bars with the genre spelled out say the same thing at three metres.

const MS_PER_HOUR = 3_600_000;
const MS_PER_MINUTE = 60_000;

function formatWatchTime(ms: string | undefined): string {
  const total = parseInt(ms ?? "0", 10);
  const h = Math.floor(total / MS_PER_HOUR);
  const m = Math.floor((total % MS_PER_HOUR) / MS_PER_MINUTE);
  return h > 0 ? `${h}h ${m}m` : `${m}m`;
}

function StatCard({ label, value }: { label: string; value: string | number }) {
  const theme = useTheme();
  return (
    <View style={[styles.statCard, { backgroundColor: theme.colors.surface, borderColor: theme.colors.border }]}>
      <Text style={[styles.statValue, { color: theme.colors.text }]}>{value}</Text>
      <Text style={[styles.statLabel, { color: theme.colors.textMuted }]}>{label}</Text>
    </View>
  );
}

export default function StatsScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const profileId = useAuthStore((s) => s.profileId);
  const { data, isLoading, isError, refetch } = useActivityServiceGetUserStats(profileId ?? "", undefined, {
    query: { enabled: !!profileId },
  });

  const genres = useMemo(
    () =>
      [...(data?.genres ?? [])].sort(
        (a, b) => parseInt(b.totalTimeMs ?? "0", 10) - parseInt(a.totalTimeMs ?? "0", 10),
      ),
    [data?.genres],
  );

  const header = (
    <Stack.Screen options={{ title: t("common.userStats.title", "My statistics") }} />
  );

  // A failed request used to fall through to the "nothing watched yet" empty
  // state below, which reads as "you have no history" instead of "the server
  // did not answer" - and offers no way to try again.
  if (isLoading || isError) {
    return <ScreenState loading={isLoading} error={isError} onRetry={() => void refetch()} />;
  }

  // Even an empty state keeps a focusable target: a screen the D-pad cannot
  // leave is the one failure a TV app must never ship.
  if (!data || (data.totalPlays ?? 0) === 0) {
    return (
      <View style={[styles.center, { backgroundColor: theme.colors.bg }]}>
        {header}
        <Text style={[styles.empty, { color: theme.colors.textMuted }]}>
          {t("common.userStats.noData", "Nothing watched yet")}
        </Text>
        <FocusableCard
          onPress={() => router.back()}
          hasTVPreferredFocus
          style={[styles.backBtn, { backgroundColor: theme.colors.surface }]}
          accessibilityLabel={t("common.back", "Back")}
        >
          <Text style={{ color: theme.colors.text, fontSize: 18 }}>{t("common.back", "Back")}</Text>
        </FocusableCard>
      </View>
    );
  }

  const favoriteGenre = genres.length > 0 ? genres[0].genre ?? "-" : "-";
  const topGenreMs = parseInt(genres[0]?.totalTimeMs ?? "0", 10) || 1;
  const mostPlayed = data.mostPlayed ?? [];

  return (
    <ScrollView style={{ flex: 1, backgroundColor: theme.colors.bg }} contentContainerStyle={{ padding: 48 }}>
      {header}
      <Text style={[styles.title, { color: theme.colors.text }]}>{t("common.userStats.title", "My statistics")}</Text>

      <View style={styles.statGrid}>
        <StatCard label={t("common.userStats.totalWatchTime")} value={formatWatchTime(data.totalWatchTimeMs)} />
        <StatCard label={t("common.userStats.totalPlays")} value={data.totalPlays ?? 0} />
        <StatCard label={t("common.userStats.uniqueItems")} value={data.uniqueItemsWatched ?? 0} />
        <StatCard label={t("common.userStats.favoriteGenre")} value={favoriteGenre} />
      </View>

      {genres.length > 0 && (
        <View style={{ marginTop: 48 }}>
          <Text style={[styles.sectionTitle, { color: theme.colors.text }]}>
            {t("common.userStats.genreBreakdown")}
          </Text>
          {genres.slice(0, 8).map((g) => {
            const ms = parseInt(g.totalTimeMs ?? "0", 10);
            return (
              <View
                key={g.genre}
                style={styles.genreRow}
                accessible={true}
                accessibilityLabel={`${g.genre}, ${formatWatchTime(g.totalTimeMs)}`}
              >
                <Text style={[styles.genreName, { color: theme.colors.text }]} numberOfLines={1}>{g.genre}</Text>
                <View style={[styles.genreTrack, { backgroundColor: theme.colors.surface }]}>
                  <View
                    style={[
                      styles.genreFill,
                      { width: `${Math.max(2, (ms / topGenreMs) * 100)}%`, backgroundColor: theme.colors.accent },
                    ]}
                  />
                </View>
                <Text style={[styles.genreTime, { color: theme.colors.textMuted }]}>{formatWatchTime(g.totalTimeMs)}</Text>
              </View>
            );
          })}
        </View>
      )}

      {mostPlayed.length > 0 && (
        <View style={{ marginTop: 48 }}>
          <Text style={[styles.sectionTitle, { color: theme.colors.text }]}>
            {t("common.userStats.mostPlayed")}
          </Text>
          {mostPlayed.map((m, i) => (
            <FocusableCard
              key={m.mediaItemId ?? i}
              onPress={() => m.mediaItemId && router.push(`/media/${m.mediaItemId}`)}
              style={[styles.playedRow, { backgroundColor: theme.colors.surface }]}
              accessibilityLabel={`${m.title ?? ""}, ${m.playCount ?? 0} ${t("common.userStats.playsLabel")}`}
            >
              <Text style={{ color: theme.colors.text, fontSize: 18, flex: 1 }} numberOfLines={1}>{m.title}</Text>
              <Text style={{ color: theme.colors.textMuted, fontSize: 17 }}>
                {m.playCount ?? 0} {t("common.userStats.playsLabel")}
              </Text>
            </FocusableCard>
          ))}
        </View>
      )}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  center: { flex: 1, justifyContent: "center", alignItems: "center", gap: 16 },
  empty: { fontSize: 21 },
  backBtn: { paddingVertical: 10, paddingHorizontal: 36, borderRadius: 8 },
  title: { fontSize: 40, fontWeight: "700", marginBottom: 32 },
  statGrid: { flexDirection: "row", gap: 16 },
  statCard: {
    flex: 1,
    alignItems: "center",
    justifyContent: "center",
    paddingVertical: 28,
    borderRadius: 12,
    borderWidth: 1,
    gap: 8,
  },
  statValue: { fontSize: 28, fontWeight: "700" },
  statLabel: { fontSize: 15, textTransform: "uppercase", letterSpacing: 1 },
  sectionTitle: { fontSize: 24, fontWeight: "600", marginBottom: 20 },
  genreRow: { flexDirection: "row", alignItems: "center", gap: 14, marginBottom: 14 },
  genreName: { fontSize: 18, width: 260 },
  genreTrack: { flex: 1, height: 18, borderRadius: 9, overflow: "hidden" },
  genreFill: { height: "100%", borderRadius: 9 },
  genreTime: { fontSize: 17, width: 110, textAlign: "right" },
  playedRow: {
    flexDirection: "row",
    alignItems: "center",
    gap: 16,
    paddingVertical: 12,
    paddingHorizontal: DEEP_SCREEN_GUTTER,
    borderRadius: 10,
    marginBottom: 12,
  },
});
