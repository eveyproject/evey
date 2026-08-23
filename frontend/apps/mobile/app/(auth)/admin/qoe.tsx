import { useCallback, useState } from "react";
import { View, Text, ScrollView, Pressable, RefreshControl, ActivityIndicator, StyleSheet, Alert } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQuery, keepPreviousData } from "@tanstack/react-query";
import { eveyFetch, apiErrorMessage } from "@evey/api";
import { toast } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { formatPct, formatInt, formatRelative, qualityStatus, type HealthLevel } from "../../../src/components/admin/qoeHelpers";

// Mirrors frontend/apps/web/src/routes/_auth/admin/qoe.lazy.tsx - same
// endpoint, same fields, same health-bucket thresholds. No generated hook
// exists for this REST endpoint, so it's fetched directly via eveyFetch.

interface QoESummary {
  window: string;
  total_events: number;
  starved_events: number;
  starved_ratio: number;
  unique_sessions: number;
  unique_media: number;
}

interface QoEStarvedTitle {
  media_item_id: string;
  title: string;
  file_path: string;
  starved_events: number;
  total_events: number;
  starved_ratio: number;
  last_starved_at: string;
}

interface QoEDeviceBreakdown {
  device_class: string;
  total_events: number;
  starved_events: number;
  starved_ratio: number;
  avg_buffer_ms: number;
  avg_encoded_kbps: number;
}

interface QoEDashboard {
  summary: QoESummary;
  top_starved_titles: QoEStarvedTitle[];
  by_device: QoEDeviceBreakdown[];
}

interface EncodeOverride {
  source_class: string;
  max_height: number;
  reason: string;
  starved_ratio: number;
  samples_count: number;
  applied_at: string;
  expires_at: string;
}

type Window = "24h" | "7d";

function useQoEDashboard(window: Window) {
  return useQuery<QoEDashboard>({
    queryKey: ["admin", "qoe", "dashboard", window],
    queryFn: () =>
      eveyFetch<QoEDashboard>({ url: `/api/v1/admin/qoe/dashboard?window=${window}`, method: "GET" }),
    // 24h and 7d are separate caches; a stale one can show a ratio that's
    // mathematically impossible relative to the other window if left cached.
    staleTime: 0,
    refetchInterval: 60_000,
    refetchIntervalInBackground: false,
    placeholderData: keepPreviousData,
  });
}

const OVERRIDES_KEY = ["admin", "transcode-overrides"];

function KpiCard({ label, value, icon, warn, theme }: { label: string; value: string; icon: string; warn?: boolean; theme: ReturnType<typeof useTheme> }) {
  return (
    <View
      style={[
        styles.statCard,
        {
          backgroundColor: theme.colors.surface,
          borderRadius: theme.radii.md,
          borderColor: warn ? theme.colors.warning : "transparent",
          borderWidth: warn ? 1 : 0,
        },
      ]}
    >
      <Ionicons name={icon as never} size={22} color={warn ? theme.colors.warning : theme.colors.accent} />
      <Text style={[styles.statValue, { color: theme.colors.text }]}>{value}</Text>
      <Text style={[styles.statLabel, { color: theme.colors.textMuted }]}>{label}</Text>
    </View>
  );
}

const HEALTH_BADGE_COLOR: Record<HealthLevel, "success" | "muted" | "warning" | "error"> = {
  excellent: "success",
  good: "success",
  fair: "muted",
  marginal: "warning",
  poor: "error",
  insufficient: "muted",
};

function HealthBanner({ level, ratio, theme }: { level: HealthLevel; ratio: number; theme: ReturnType<typeof useTheme> }) {
  const { t } = useTranslation();
  const kind = HEALTH_BADGE_COLOR[level];
  const badgeColor = kind === "success" ? theme.colors.success : kind === "warning" ? theme.colors.warning : kind === "error" ? theme.colors.error : theme.colors.textMuted;
  const recoKey = "admin.qoe.health.reco" + level.charAt(0).toUpperCase() + level.slice(1);

  return (
    <View style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md, gap: 8 }]}>
      <View style={{ flexDirection: "row", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
        <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>{t("admin.qoe.health.label")}</Text>
        <View style={[styles.badge, { backgroundColor: badgeColor + "22" }]}>
          <Text style={{ color: badgeColor, fontSize: 12, fontWeight: "700" }}>{t(`admin.qoe.health.${level}`)}</Text>
        </View>
        {level !== "insufficient" && (
          <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{formatPct(ratio)}</Text>
        )}
      </View>
      <Text style={{ color: theme.colors.text, fontSize: 13 }}>{t(recoKey)}</Text>
    </View>
  );
}

function TopStarvedList({ items, theme }: { items: QoEStarvedTitle[]; theme: ReturnType<typeof useTheme> }) {
  const { t } = useTranslation();
  if (items.length === 0) {
    return <Text style={{ color: theme.colors.textMuted, fontSize: 13, padding: 16 }}>{t("admin.qoe.topStarved.empty")}</Text>;
  }
  return (
    <View style={[styles.listCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
      {items.map((row) => (
        <View key={row.media_item_id} style={[styles.listRow, { borderBottomColor: theme.colors.border }]}>
          <View style={{ flex: 1, gap: 2 }}>
            <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "500" }} numberOfLines={1}>
              {row.title || `${row.media_item_id.slice(0, 8)}…`}
            </Text>
            {row.file_path ? (
              <Text style={{ color: theme.colors.textMuted, fontSize: 10, fontFamily: "monospace" }} numberOfLines={1}>
                {row.file_path}
              </Text>
            ) : null}
            <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
              {t("admin.qoe.col.starvedEvents")} {formatInt(row.starved_events)} / {formatInt(row.total_events)}
            </Text>
          </View>
          <View style={{ alignItems: "flex-end", gap: 2 }}>
            <Text style={{ color: theme.colors.warning, fontSize: 14, fontWeight: "700" }}>{formatPct(row.starved_ratio)}</Text>
            <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{formatRelative(row.last_starved_at, t("admin.qoe.justNow"))}</Text>
          </View>
        </View>
      ))}
    </View>
  );
}

function ByDeviceList({ items, theme }: { items: QoEDeviceBreakdown[]; theme: ReturnType<typeof useTheme> }) {
  const { t } = useTranslation();
  if (items.length === 0) {
    return <Text style={{ color: theme.colors.textMuted, fontSize: 13, padding: 16 }}>{t("admin.qoe.byDevice.empty")}</Text>;
  }
  return (
    <View style={[styles.listCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
      {items.map((row) => (
        <View key={row.device_class} style={[styles.listRow, { borderBottomColor: theme.colors.border }]}>
          <View style={{ flex: 1, gap: 2 }}>
            <Text style={{ color: theme.colors.text, fontSize: 13, fontFamily: "monospace" }} numberOfLines={1}>
              {row.device_class}
            </Text>
            <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
              {formatInt(row.total_events)} {t("admin.qoe.col.totalEvents").toLowerCase()} · {formatInt(Math.round(row.avg_buffer_ms))}ms buf · {formatInt(Math.round(row.avg_encoded_kbps))}kbps
            </Text>
          </View>
          <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "700" }}>{formatPct(row.starved_ratio)}</Text>
        </View>
      ))}
    </View>
  );
}

function EncodeOverridesCard({ theme }: { theme: ReturnType<typeof useTheme> }) {
  const { t } = useTranslation();
  const { data, refetch } = useQuery<{ overrides: EncodeOverride[] }>({
    queryKey: OVERRIDES_KEY,
    queryFn: () => eveyFetch<{ overrides: EncodeOverride[] }>({ url: "/api/v1/admin/transcode/calibration/overrides", method: "GET" }),
    // Worker runs every 30 min; occasional viewing doesn't need sub-second freshness.
    staleTime: 60_000,
  });
  const [resetting, setResetting] = useState<string | null>(null);
  const overrides = data?.overrides ?? [];

  const resetOverride = useCallback(async (sourceClass: string) => {
    setResetting(sourceClass);
    try {
      await eveyFetch<void>({ url: `/api/v1/admin/transcode/calibration/overrides?source_class=${encodeURIComponent(sourceClass)}`, method: "DELETE" });
      await refetch();
    } catch (err) {
      toast.error(apiErrorMessage(err as Error, t, t("admin.qoe.overrides.reset")));
    } finally {
      setResetting(null);
    }
  }, [refetch, t]);

  const overridesTitle = t("admin.qoe.overrides.title", "Automatic quality limits");
  const overridesHelp = t(
    "admin.qoe.overrides.help",
    "Players continuously report how playback is going - buffer level, stalls and bandwidth (an industry standard called CMCD). When one kind of source keeps stalling viewers, Evey lowers its quality limit for a while to stop the stalls, then lifts it again automatically.",
  );

  return (
    <View style={{ gap: 8 }}>
      <View style={{ gap: 2 }}>
        <View style={{ flexDirection: "row", alignItems: "center", gap: 6, marginHorizontal: 16, marginTop: 20, marginBottom: 8 }}>
          <Text style={[styles.sectionTitle, { color: theme.colors.textMuted, marginHorizontal: 0, marginTop: 0, marginBottom: 0 }]}>
            {overridesTitle}
          </Text>
          <Pressable onPress={() => Alert.alert(overridesTitle, overridesHelp)} hitSlop={8} accessibilityRole="button" accessibilityLabel={overridesHelp}>
            <Ionicons name="information-circle-outline" size={15} color={theme.colors.textMuted} />
          </Pressable>
        </View>
        <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginHorizontal: 16, marginTop: -4 }}>
          {t("admin.qoe.overrides.subtitle", "Automatic adjustments based on how playback actually goes (buffering, throughput), on top of the calibrated ceiling.")}
        </Text>
      </View>
      <View style={{ marginHorizontal: 16 }}>
        {overrides.length === 0 ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 13, padding: 4 }}>
            {t("admin.qoe.overrides.empty", "No active automatic limits - the full calibrated quality ceiling applies to every kind of source.")}
          </Text>
        ) : (
          <View style={[styles.listCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
            {overrides.map((o) => (
              <View key={o.source_class} style={[styles.listRow, { borderBottomColor: theme.colors.border }]}>
                <View style={{ flex: 1, gap: 2 }}>
                  <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600" }}>{o.source_class}</Text>
                  <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
                    {o.max_height}p · {formatPct(o.starved_ratio)} · {t("admin.qoe.overrides.col.samples", "Samples")} {o.samples_count}
                  </Text>
                  {o.reason ? (
                    <Text style={{ color: theme.colors.textMuted, fontSize: 11 }} numberOfLines={2}>
                      {o.reason}
                    </Text>
                  ) : null}
                </View>
                <Pressable
                  disabled={resetting === o.source_class}
                  onPress={() => resetOverride(o.source_class)}
                  style={[styles.smallBtn, { backgroundColor: theme.colors.bg, opacity: resetting === o.source_class ? 0.6 : 1 }]}
                >
                  {resetting === o.source_class ? (
                    <ActivityIndicator size="small" color={theme.colors.text} />
                  ) : (
                    <Text style={{ color: theme.colors.text, fontSize: 12, fontWeight: "600" }}>{t("admin.qoe.overrides.reset", "Reset")}</Text>
                  )}
                </Pressable>
              </View>
            ))}
          </View>
        )}
      </View>
    </View>
  );
}

export default function AdminQoEScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const [window, setWindow] = useState<Window>("24h");
  const [refreshing, setRefreshing] = useState(false);

  const { data, isLoading, isError, error, refetch } = useQoEDashboard(window);

  const onRefresh = useCallback(async () => {
    setRefreshing(true);
    await refetch();
    setRefreshing(false);
  }, [refetch]);

  const summary = data?.summary;
  const level = qualityStatus(summary?.starved_ratio ?? 0, summary?.total_events ?? 0);

  return (
    <ScrollView
      style={{ flex: 1, backgroundColor: theme.colors.bg }}
      contentContainerStyle={{ paddingBottom: 40 }}
      refreshControl={<RefreshControl refreshing={refreshing} onRefresh={onRefresh} tintColor={theme.colors.accent} />}
    >
      <Stack.Screen options={{
        headerShown: true,
        title: t("admin.qoe.title"),
        headerStyle: { backgroundColor: theme.colors.surface },
        headerTintColor: theme.colors.text,
      }} />

      <View style={{ flexDirection: "row", gap: 8, padding: 16, paddingBottom: 0 }}>
        <Pressable
          onPress={() => setWindow("24h")}
          style={[styles.filterChip, { backgroundColor: window === "24h" ? theme.colors.accent : theme.colors.surfaceRaised, borderRadius: theme.radii.md }]}
        >
          <Text style={{ color: window === "24h" ? theme.colors.onAccent : theme.colors.textMuted, fontSize: 13, fontWeight: "600" }}>
            {t("admin.qoe.window24h")}
          </Text>
        </Pressable>
        <Pressable
          onPress={() => setWindow("7d")}
          style={[styles.filterChip, { backgroundColor: window === "7d" ? theme.colors.accent : theme.colors.surfaceRaised, borderRadius: theme.radii.md }]}
        >
          <Text style={{ color: window === "7d" ? theme.colors.onAccent : theme.colors.textMuted, fontSize: 13, fontWeight: "600" }}>
            {t("admin.qoe.window7d")}
          </Text>
        </Pressable>
      </View>

      {isLoading && !data ? (
        <View style={{ alignItems: "center", paddingTop: 40 }}>
          <ActivityIndicator color={theme.colors.accent} size="large" />
          <Text style={{ color: theme.colors.textMuted, marginTop: 8, fontSize: 13 }}>{t("admin.qoe.loading")}</Text>
        </View>
      ) : isError ? (
        <View style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md, margin: 16 }]}>
          <Text style={{ color: theme.colors.error, fontSize: 13 }}>
            {t("admin.qoe.loadError")}: {error instanceof Error ? error.message : "unknown"}
          </Text>
        </View>
      ) : data ? (
        <>
          <View style={styles.statsGrid}>
            <KpiCard label={t("admin.qoe.kpi.events")} value={formatInt(summary?.total_events)} icon="speedometer-outline" theme={theme} />
            <KpiCard
              label={t("admin.qoe.kpi.starvedRatio")}
              value={formatPct(summary?.starved_ratio ?? 0)}
              icon="alert-circle-outline"
              warn={(summary?.starved_ratio ?? 0) > 0.05}
              theme={theme}
            />
            <KpiCard label={t("admin.qoe.kpi.sessions")} value={formatInt(summary?.unique_sessions)} icon="people-outline" theme={theme} />
            <KpiCard label={t("admin.qoe.kpi.uniqueMedia")} value={formatInt(summary?.unique_media)} icon="film-outline" theme={theme} />
          </View>

          <View style={{ marginHorizontal: 16 }}>
            <HealthBanner level={level} ratio={summary?.starved_ratio ?? 0} theme={theme} />
          </View>

          <Text style={[styles.sectionTitle, { color: theme.colors.textMuted }]}>{t("admin.qoe.topStarved.title")}</Text>
          <View style={{ marginHorizontal: 16 }}>
            <TopStarvedList items={data.top_starved_titles ?? []} theme={theme} />
          </View>

          <Text style={[styles.sectionTitle, { color: theme.colors.textMuted }]}>{t("admin.qoe.byDevice.title")}</Text>
          <View style={{ marginHorizontal: 16 }}>
            <ByDeviceList items={data.by_device ?? []} theme={theme} />
          </View>

          <EncodeOverridesCard theme={theme} />
        </>
      ) : null}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  statsGrid: { flexDirection: "row", flexWrap: "wrap", padding: 16, gap: 12 },
  statCard: { flex: 1, minWidth: "45%", padding: 16, alignItems: "center", gap: 4 },
  statValue: { fontSize: 22, fontWeight: "700" },
  statLabel: { fontSize: 12, textAlign: "center" },
  card: { padding: 12 },
  badge: { paddingHorizontal: 8, paddingVertical: 2, borderRadius: 999 },
  sectionTitle: { fontSize: 13, fontWeight: "600", textTransform: "uppercase", marginTop: 20, marginBottom: 8, marginHorizontal: 16 },
  listCard: { overflow: "hidden" },
  listRow: { flexDirection: "row", alignItems: "flex-start", gap: 12, paddingVertical: 12, paddingHorizontal: 16, borderBottomWidth: StyleSheet.hairlineWidth },
  filterChip: { paddingHorizontal: 12, paddingVertical: 8 },
  smallBtn: { paddingHorizontal: 12, paddingVertical: 8, borderRadius: 8, alignItems: "center", justifyContent: "center", minWidth: 60 },
});
