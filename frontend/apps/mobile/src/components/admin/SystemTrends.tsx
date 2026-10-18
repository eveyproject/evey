import { useEffect, useMemo, useState } from "react";
import { View, Text, Pressable, StyleSheet, useWindowDimensions } from "react-native";
import { useTranslation } from "react-i18next";
import { keepPreviousData } from "@tanstack/react-query";
import { LineChart } from "react-native-gifted-charts";
import { useActivityServiceListServerMetrics } from "@evey/api";
import type { V1ServerMetric } from "@evey/api";
import type { useTheme } from "../../providers/ThemeProvider";
import { formatBytesPerSecond } from "./DiskIO";

type Theme = ReturnType<typeof useTheme>;
export type TrendRange = "24h" | "7d" | "30d";

const RANGE_HOURS: Record<TrendRange, number> = { "24h": 24, "7d": 7 * 24, "30d": 30 * 24 };
const RANGES: TrendRange[] = ["24h", "7d", "30d"];
const CHART_HEIGHT = 120;
// Categorical slots in fixed order, the web's --chart-1/2/3.
const SERIES_COLORS = ["#c81e1e", "#f59e0b", "#10b981"];

type Unit = "percent" | "ms" | "bps" | "bytesps" | "count";

// null = not measured: the chart skips the point instead of drawing a zero.
export interface Point {
  t: number;
  cpu: number;
  iowait: number | null;
  memory: number;
  diskAwait: number | null;
  diskUtil: number | null;
  diskRead: number | null;
  diskWrite: number | null;
  netRx: number | null;
  netTx: number | null;
  streaming: number;
  storage: number | null;
  streams: number;
  transcodes: number;
}

type SeriesKey = Exclude<keyof Point, "t">;

interface ChartDef {
  id: string;
  titleKey: string;
  unit: Unit;
  series: { key: SeriesKey; labelKey: string }[];
}

// Same charts as the web page, one per unit.
export const CHARTS: ChartDef[] = [
  { id: "cpu", titleKey: "admin.systemHealth.trendsCpu", unit: "percent", series: [
    { key: "cpu", labelKey: "admin.systemHealth.trendsCpuSeries" },
    { key: "iowait", labelKey: "admin.systemHealth.trendsIowaitSeries" },
  ] },
  { id: "memory", titleKey: "admin.systemHealth.trendsMemory", unit: "percent", series: [{ key: "memory", labelKey: "admin.systemHealth.trendsMemorySeries" }] },
  { id: "diskAwait", titleKey: "admin.systemHealth.trendsDiskAwait", unit: "ms", series: [{ key: "diskAwait", labelKey: "admin.systemHealth.trendsDiskAwaitSeries" }] },
  { id: "diskUtil", titleKey: "admin.systemHealth.trendsDiskUtil", unit: "percent", series: [{ key: "diskUtil", labelKey: "admin.systemHealth.trendsDiskUtilSeries" }] },
  { id: "diskRate", titleKey: "admin.systemHealth.trendsDiskThroughput", unit: "bytesps", series: [
    { key: "diskRead", labelKey: "admin.systemHealth.trendsRead" },
    { key: "diskWrite", labelKey: "admin.systemHealth.trendsWrite" },
  ] },
  { id: "network", titleKey: "admin.systemHealth.trendsNetwork", unit: "bps", series: [
    { key: "netRx", labelKey: "admin.systemHealth.trendsNetRx" },
    { key: "netTx", labelKey: "admin.systemHealth.trendsNetTx" },
    { key: "streaming", labelKey: "admin.systemHealth.trendsStreaming" },
  ] },
  { id: "streams", titleKey: "admin.systemHealth.trendsStreams", unit: "count", series: [
    { key: "streams", labelKey: "admin.systemHealth.trendsStreamsSeries" },
    { key: "transcodes", labelKey: "admin.systemHealth.trendsTranscodesSeries" },
  ] },
  { id: "storage", titleKey: "admin.systemHealth.trendsStorage", unit: "percent", series: [{ key: "storage", labelKey: "admin.systemHealth.trendsStorageSeries" }] },
];

function num(v: string | number | undefined): number {
  const n = typeof v === "string" ? parseInt(v, 10) : (v ?? 0);
  return Number.isFinite(n) ? n : 0;
}

// A row written before migration 000178 has no disk, network, iowait or
// storage reading: those columns defaulted to 0. A measured row always moves
// some network bytes, so "every one of them is zero" means "never measured".
function measuredIO(m: V1ServerMetric): boolean {
  return [m.iowaitPercent, m.diskAwaitMs, m.diskUtilPercent, m.diskReadBps, m.diskWriteBps, m.networkRxBps, m.networkTxBps, m.storageTotalBytes]
    .some((v) => num(v) !== 0);
}

export function toPoints(metrics: V1ServerMetric[] | undefined): Point[] {
  return (metrics ?? [])
    .map((m) => {
      const total = num(m.storageTotalBytes);
      const io = measuredIO(m);
      const orGap = (v: number) => (io ? v : null);
      return {
        t: Date.parse(m.timestamp ?? ""),
        cpu: m.cpuPercent ?? 0,
        iowait: orGap(m.iowaitPercent ?? 0),
        memory: m.memoryPercent ?? 0,
        diskAwait: orGap(m.diskAwaitMs ?? 0),
        diskUtil: orGap(m.diskUtilPercent ?? 0),
        diskRead: orGap(num(m.diskReadBps)),
        diskWrite: orGap(num(m.diskWriteBps)),
        netRx: orGap(num(m.networkRxBps)),
        netTx: orGap(num(m.networkTxBps)),
        streaming: num(m.bandwidthBps),
        // No disk has a zero total: the sampler could not read it.
        storage: total > 0 ? (num(m.storageUsedBytes) / total) * 100 : null,
        streams: m.activeStreams ?? 0,
        transcodes: m.activeTranscodes ?? 0,
      };
    })
    .filter((p) => Number.isFinite(p.t));
}

export function formatTick(range: TrendRange, t: number): string {
  const d = new Date(t);
  const hm = `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
  const dm = `${String(d.getDate()).padStart(2, "0")}/${String(d.getMonth() + 1).padStart(2, "0")}`;
  if (range === "24h") return hm;
  if (range === "7d") return `${dm} ${hm}`;
  return dm;
}

// formatAxis keeps the y labels short: a phone's chart has no room for units.
export function formatAxis(unit: Unit, v: number): string {
  switch (unit) {
    case "percent":
    case "ms":
    case "count":
      return `${Math.round(v)}`;
    case "bps": {
      if (v >= 1e9) return `${(v / 1e9).toFixed(1)}G`;
      if (v >= 1e6) return `${(v / 1e6).toFixed(0)}M`;
      if (v >= 1e3) return `${(v / 1e3).toFixed(0)}k`;
      return `${Math.round(v)}`;
    }
    case "bytesps":
      return formatBytesPerSecond(Math.round(v)).replace(/\/s$/, "");
  }
}

// The window's end moves once a minute, like the sampler: a fresh Date on
// every render would change the query key and refetch on each paint.
function useWindowEnd(range: TrendRange): number {
  const [end, setEnd] = useState(() => Date.now());
  useEffect(() => {
    setEnd(Date.now());
    const id = setInterval(() => setEnd(Date.now()), 60_000);
    return () => clearInterval(id);
  }, [range]);
  return end;
}

interface TrendChartProps {
  chart: ChartDef;
  points: Point[];
  range: TrendRange;
  width: number;
  theme: Theme;
  idleAwaitMs?: number;
  saturationAwaitMs?: number;
}

function TrendChart({ chart, points: allPoints, range, width, theme, idleAwaitMs, saturationAwaitMs }: TrendChartProps) {
  const { t } = useTranslation();
  // The chart library draws no gap: an unmeasured point is left out rather
  // than drawn as a zero (29 days of idle disks before migration 000178).
  const points = allPoints.filter((p) => chart.series.every((s) => p[s.key] != null));
  if (points.length < 2) return null;
  // A label every quarter of the window keeps the axis readable on a phone.
  const labelEvery = Math.max(1, Math.floor(points.length / 4));
  const series = chart.series.map((s) => points.map((p, i) => ({
    value: p[s.key] ?? 0,
    label: i % labelEvery === 0 && i < points.length - labelEvery / 2 ? formatTick(range, p.t) : undefined,
  })));
  const maxValue = chart.unit === "percent"
    ? 100
    : Math.max(1, ...series.flat().map((d) => d.value), saturationAwaitMs && chart.id === "diskAwait" ? saturationAwaitMs * 1.1 : 0);
  const plotWidth = Math.max(120, width - 40);
  const spacing = points.length > 1 ? plotWidth / (points.length - 1) : plotWidth;
  const refLines = chart.id === "diskAwait" && idleAwaitMs && saturationAwaitMs
    ? {
      showReferenceLine1: true,
      referenceLine1Position: idleAwaitMs,
      referenceLine1Config: { color: theme.colors.textMuted, dashWidth: 4, dashGap: 4, thickness: 1, labelText: t("admin.systemHealth.trendsDiskAwaitIdle"), labelTextStyle: { color: theme.colors.textMuted, fontSize: 9 } },
      showReferenceLine2: true,
      referenceLine2Position: saturationAwaitMs,
      referenceLine2Config: { color: theme.colors.danger, dashWidth: 4, dashGap: 4, thickness: 1, labelText: t("admin.systemHealth.trendsDiskAwaitSaturation"), labelTextStyle: { color: theme.colors.danger, fontSize: 9 } },
    }
    : {};
  return (
    <View style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]} testID={`trend-${chart.id}`}>
      <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600" }}>{t(chart.titleKey)}</Text>
      {chart.series.length > 1 && (
        <View style={styles.legend}>
          {chart.series.map((s, i) => (
            <View key={s.key} style={styles.legendItem}>
              <View style={[styles.swatch, { backgroundColor: SERIES_COLORS[i] }]} />
              <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{t(s.labelKey)}</Text>
            </View>
          ))}
        </View>
      )}
      <LineChart
        data={series[0]}
        data2={series[1]}
        data3={series[2]}
        color={SERIES_COLORS[0]}
        color2={SERIES_COLORS[1]}
        color3={SERIES_COLORS[2]}
        width={plotWidth}
        height={CHART_HEIGHT}
        maxValue={maxValue}
        noOfSections={4}
        spacing={spacing}
        initialSpacing={0}
        endSpacing={0}
        adjustToWidth
        thickness={2}
        hideDataPoints
        curved
        isAnimated={false}
        rulesColor={theme.colors.border}
        yAxisColor="transparent"
        xAxisColor={theme.colors.border}
        yAxisLabelWidth={40}
        yAxisTextStyle={{ color: theme.colors.textMuted, fontSize: 10 }}
        xAxisLabelTextStyle={{ color: theme.colors.textMuted, fontSize: 9 }}
        formatYLabel={(label: string) => formatAxis(chart.unit, Number(label))}
        {...refLines}
      />
    </View>
  );
}

// SystemTrends is the history the sampler writes once a minute, folded by the
// server into a few hundred points per window (mirror of the web section).
export function SystemTrends({ theme }: { theme: Theme }) {
  const { t } = useTranslation();
  const { width: screenWidth } = useWindowDimensions();
  const [range, setRange] = useState<TrendRange>("7d");
  const end = useWindowEnd(range);
  const from = useMemo(() => new Date(end - RANGE_HOURS[range] * 3600_000).toISOString(), [end, range]);
  const to = useMemo(() => new Date(end).toISOString(), [end]);
  const { data, isLoading, isFetching, isError } = useActivityServiceListServerMetrics(
    { from, to },
    { query: { refetchInterval: 60_000, refetchIntervalInBackground: false, placeholderData: keepPreviousData } },
  );
  const points = useMemo(() => toPoints(data?.metrics), [data?.metrics]);
  const bucketMinutes = Math.max(1, Math.round((data?.intervalSeconds ?? 60) / 60));
  const cardWidth = screenWidth - 32 - 32;

  return (
    <View style={{ marginHorizontal: 16, gap: 8 }} testID="system-trends">
      <View style={styles.rangeRow}>
        <View style={[styles.rangeGroup, { borderColor: theme.colors.border }]}>
          {RANGES.map((r) => (
            <Pressable
              key={r}
              onPress={() => setRange(r)}
              accessibilityRole="button"
              accessibilityState={{ selected: range === r }}
              style={[styles.rangeButton, range === r && { backgroundColor: theme.colors.accent }]}
            >
              <Text style={{ color: range === r ? "#fff" : theme.colors.textMuted, fontSize: 12 }}>{t(`admin.systemHealth.trendsRange_${r}`)}</Text>
            </Pressable>
          ))}
        </View>
        {data?.intervalSeconds ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{t("admin.systemHealth.trendsBucket", { minutes: bucketMinutes })}</Text>
        ) : null}
      </View>
      {isError ? (
        <Text style={{ color: theme.colors.danger, fontSize: 13 }}>{t("admin.systemHealth.trendsError")}</Text>
      ) : !isLoading && points.length < 2 ? (
        <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>{t("admin.systemHealth.trendsEmpty")}</Text>
      ) : (
        <View style={{ gap: 8, opacity: isFetching && !isLoading ? 0.6 : 1 }}>
          {CHARTS.map((chart) => (
            <TrendChart
              key={chart.id}
              chart={chart}
              points={points}
              range={range}
              width={cardWidth}
              theme={theme}
              idleAwaitMs={data?.idleDiskAwaitMs}
              saturationAwaitMs={data?.saturationAwaitMs}
            />
          ))}
        </View>
      )}
      <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{t("admin.systemHealth.trendsMachineNote")}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  card: { padding: 16, gap: 8, overflow: "hidden" },
  legend: { flexDirection: "row", flexWrap: "wrap", gap: 12 },
  legendItem: { flexDirection: "row", alignItems: "center", gap: 4 },
  swatch: { width: 10, height: 2, borderRadius: 1 },
  rangeRow: { flexDirection: "row", alignItems: "center", flexWrap: "wrap", gap: 12 },
  rangeGroup: { flexDirection: "row", borderWidth: 1, borderRadius: 8, overflow: "hidden" },
  rangeButton: { paddingVertical: 6, paddingHorizontal: 12 },
});
