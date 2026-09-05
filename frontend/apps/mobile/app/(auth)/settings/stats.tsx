import { View, Text, ScrollView, Pressable, ActivityIndicator, StyleSheet, Dimensions } from "react-native";
import { Stack, router } from "expo-router";
import { useTranslation } from "react-i18next";
import { useActivityServiceGetUserStats } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { PieChart, BarChart } from "react-native-gifted-charts";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { chartColorAt } from "../../../src/lib/chartPalette";
import { LoadFailed } from "../../../src/components/LoadFailed";

function formatWatchTime(ms: string | undefined): string {
  const total = parseInt(ms ?? "0");
  const h = Math.floor(total / 3600000);
  const m = Math.floor((total % 3600000) / 60000);
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

function formatHours(ms: string | undefined): string {
  const h = parseInt(ms ?? "0") / 3600000;
  if (h >= 1) return `${Math.round(h * 10) / 10}h`;
  const m = Math.round(parseInt(ms ?? "0") / 60000);
  return `${m}m`;
}

function buildMonthlyData(byMonth: { year?: number; month?: number; totalTimeMs?: string }[], accent: string) {
  const now = new Date();
  const result = [];
  for (let i = 11; i >= 0; i--) {
    const d = new Date(now.getFullYear(), now.getMonth() - i, 1);
    const y = d.getFullYear();
    const m = d.getMonth() + 1;
    const label = d.toLocaleDateString(undefined, { month: "short" });
    const entry = byMonth.find((e) => e.year === y && e.month === m);
    const hours = parseInt(entry?.totalTimeMs ?? "0") / 3600000;
    result.push({ value: Math.round(hours * 10) / 10, label, frontColor: accent });
  }
  return result;
}

export default function StatsScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const profileId = useAuthStore((s) => s.profileId);
  const screenWidth = Dimensions.get("window").width;

  const { data, isLoading, isError, refetch } = useActivityServiceGetUserStats(profileId ?? "", undefined, {
    query: { enabled: !!profileId },
  });

  const s = sStyles(theme);

  if (isLoading) {
    return (
      <View style={[s.container, { justifyContent: "center", alignItems: "center" }]}>
        <ActivityIndicator size="large" color={theme.colors.accent} />
      </View>
    );
  }

  if (isError) {
    return (
      <View style={[s.container, { justifyContent: "center", alignItems: "center" }]}>
        <LoadFailed onRetry={() => void refetch()} />
      </View>
    );
  }

  if (!data || (data.totalPlays ?? 0) === 0) {
    return (
      <View style={[s.container, { justifyContent: "center", alignItems: "center" }]}>
        <Ionicons name="bar-chart-outline" size={48} color={theme.colors.textMuted} />
        <Text style={{ color: theme.colors.textMuted, fontSize: 16, marginTop: theme.spacing.md }}>
          {t("common.userStats.noData")}
        </Text>
      </View>
    );
  }

  // Genres arrive already folded and labelled by the server (GetUserStats):
  // it is the only side that knows every stored spelling of a genre, so a
  // second table here could only disagree with it - which is exactly how the
  // pie ended up with a "Drama" slice next to a "Drame" one.
  const genres = [...(data.genres ?? [])].sort((a, b) => parseInt(b.totalTimeMs ?? "0") - parseInt(a.totalTimeMs ?? "0"));
  const pieData = genres.slice(0, 8).map((g, i) => ({
    value: parseInt(g.totalTimeMs ?? "0") || 1,
    color: chartColorAt(theme, i),
    text: g.genre ?? "",
    percent: Math.round(g.percent ?? 0),
  }));

  const monthBarData = buildMonthlyData(data.byMonth ?? [], theme.colors.accent);
  const monthMax = Math.max(...monthBarData.map((d) => d.value), 0.1);

  const mostPlayed = data.mostPlayed ?? [];
  const favoriteGenre = genres.length > 0 ? genres[0].genre ?? "-" : "-";

  const barWidth = Math.max(14, Math.floor((screenWidth - 120) / 12) - 6);

  return (
    <ScrollView style={s.container} contentContainerStyle={{ paddingBottom: theme.spacing.xl }}>
      <Stack.Screen
        options={{
          headerShown: true,
          title: t("common.userStats.title"),
          headerStyle: { backgroundColor: theme.colors.surface },
          headerTintColor: theme.colors.text,
        }}
      />

      {/* Stat cards */}
      <View style={s.cardsRow}>
        <View style={s.card}>
          <Text style={s.cardValue}>{formatWatchTime(data.totalWatchTimeMs)}</Text>
          <Text style={s.cardLabel}>{t("common.userStats.totalWatchTime")}</Text>
        </View>
        <View style={s.card}>
          <Text style={s.cardValue}>{data.totalPlays ?? 0}</Text>
          <Text style={s.cardLabel}>{t("common.userStats.totalPlays")}</Text>
        </View>
      </View>
      <View style={s.cardsRow}>
        <View style={s.card}>
          <Text style={s.cardValue}>{data.uniqueItemsWatched ?? 0}</Text>
          <Text style={s.cardLabel}>{t("common.userStats.uniqueItems")}</Text>
        </View>
        <View style={s.card}>
          <Text style={s.cardValue} numberOfLines={1}>{favoriteGenre}</Text>
          <Text style={s.cardLabel}>{t("common.userStats.favoriteGenre")}</Text>
        </View>
      </View>

      {/* Genre breakdown */}
      {pieData.length > 0 && (
        <View style={s.section}>
          <Text style={s.sectionTitle}>{t("common.userStats.genreBreakdown")}</Text>
          <View style={{ alignItems: "center", marginBottom: theme.spacing.md }}>
            <PieChart
              data={pieData}
              radius={70}
              innerRadius={40}
              donut
              showText={false}
            />
          </View>
          <View style={{ gap: 6 }}>
            {pieData.map((d) => (
              <View key={d.text} style={s.legendRow}>
                <View style={[s.legendDot, { backgroundColor: d.color }]} />
                <Text style={s.legendText}>{d.text}</Text>
                <Text style={s.legendPercent}>{d.percent}%</Text>
              </View>
            ))}
          </View>
        </View>
      )}

      {/* Monthly activity */}
      <View style={s.section}>
        <Text style={s.sectionTitle}>{t("common.userStats.monthlyActivity")}</Text>
        <ScrollView horizontal showsHorizontalScrollIndicator={false}>
          <BarChart
            data={monthBarData}
            barWidth={barWidth}
            spacing={6}
            roundedTop
            noOfSections={4}
            maxValue={Math.ceil(monthMax * 1.1) || 1}
            yAxisTextStyle={{ color: theme.colors.textMuted, fontSize: 10 }}
            xAxisLabelTextStyle={{ color: theme.colors.textMuted, fontSize: 9 }}
            yAxisLabelSuffix="h"
            hideRules
            isAnimated
            barBorderTopLeftRadius={4}
            barBorderTopRightRadius={4}
            yAxisColor={theme.colors.border}
            xAxisColor={theme.colors.border}
          />
        </ScrollView>
      </View>

      {/* Most played */}
      {mostPlayed.length > 0 && (
        <View style={{ marginHorizontal: theme.spacing.md, marginTop: theme.spacing.md }}>
          <Text style={[s.sectionTitle, { marginHorizontal: 0, marginBottom: theme.spacing.sm }]}>
            {t("common.userStats.mostPlayed")}
          </Text>
          {mostPlayed.map((m, i) => (
            <Pressable
              key={m.mediaItemId ?? i}
              onPress={() => { if (m.mediaItemId) router.push(`/media/${m.mediaItemId}` as never); }}
              style={s.mostPlayedRow}
              accessibilityRole="button"
            >
              <Text style={s.mpRank}>{i + 1}</Text>
              <Text style={s.mpTitle} numberOfLines={1}>{m.title ?? "Unknown"}</Text>
              <Text style={s.mpMeta}>{m.playCount ?? 0} {t("common.userStats.playsLabel")}</Text>
              <Text style={s.mpMeta}>{formatHours(m.totalTimeMs)}</Text>
            </Pressable>
          ))}
        </View>
      )}
    </ScrollView>
  );
}

const sStyles = (t: ReturnType<typeof useTheme>) =>
  StyleSheet.create({
    container: { flex: 1, backgroundColor: t.colors.bg },
    cardsRow: {
      flexDirection: "row",
      gap: t.spacing.sm,
      marginHorizontal: t.spacing.md,
      marginTop: t.spacing.sm,
    },
    card: {
      flex: 1,
      backgroundColor: t.colors.surface,
      borderRadius: t.radii.md,
      padding: t.spacing.md,
      alignItems: "center",
      justifyContent: "center",
      borderWidth: StyleSheet.hairlineWidth,
      borderColor: t.colors.border,
    },
    cardValue: { color: t.colors.text, fontSize: 20, fontWeight: "700" },
    cardLabel: {
      color: t.colors.textMuted,
      fontSize: 11,
      fontWeight: "600",
      textTransform: "uppercase",
      marginTop: 4,
      textAlign: "center",
    },
    section: {
      backgroundColor: t.colors.surface,
      borderRadius: t.radii.md,
      marginHorizontal: t.spacing.md,
      marginTop: t.spacing.md,
      padding: t.spacing.md,
      borderWidth: StyleSheet.hairlineWidth,
      borderColor: t.colors.border,
    },
    sectionTitle: {
      color: t.colors.text,
      fontSize: 14,
      fontWeight: "600",
      marginBottom: t.spacing.md,
    },
    legendRow: { flexDirection: "row", alignItems: "center", gap: 8 },
    legendDot: { width: 10, height: 10, borderRadius: 5 },
    legendText: { color: t.colors.text, fontSize: 13, flex: 1 },
    legendPercent: { color: t.colors.textMuted, fontSize: 12 },
    mostPlayedRow: {
      flexDirection: "row",
      alignItems: "center",
      gap: 10,
      paddingVertical: 10,
      paddingHorizontal: t.spacing.sm,
      borderRadius: t.radii.sm,
    },
    mpRank: { color: t.colors.accent, fontSize: 14, fontFamily: "monospace", width: 24, textAlign: "center" },
    mpTitle: { color: t.colors.text, fontSize: 14, flex: 1 },
    mpMeta: { color: t.colors.textMuted, fontSize: 12 },
  });
