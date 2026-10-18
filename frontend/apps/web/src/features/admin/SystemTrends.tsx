import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { keepPreviousData } from "@tanstack/react-query";
import { useActivityServiceListServerMetrics } from "@evey/api";
import type { V1ServerMetric } from "@evey/api";
import { CartesianGrid, Legend, Line, LineChart, ReferenceLine, Tooltip, XAxis, YAxis } from "recharts";

import { useCssVar } from "@/hooks/useCssVar";
import { formatBps } from "./DashboardStatCard";
import { formatBytesPerSecond } from "./DiskIO";

export type TrendRange = "24h" | "7d" | "30d";

const RANGE_HOURS: Record<TrendRange, number> = { "24h": 24, "7d": 7 * 24, "30d": 30 * 24 };
const RANGES: TrendRange[] = ["24h", "7d", "30d"];

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

// recharts draws nothing under a ResponsiveContainer in jsdom, and the rest of
// the admin drives it with explicit sizes; the card measures itself instead.
function useMeasuredWidth<T extends HTMLElement>(fallback: number) {
  const ref = useRef<T>(null);
  const [width, setWidth] = useState(fallback);
  useEffect(() => {
    const el = ref.current;
    if (!el || typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver((entries) => {
      const w = entries[0]?.contentRect.width;
      if (w && w > 0) setWidth(Math.floor(w));
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  return { ref, width };
}

type Unit = "percent" | "ms" | "bps" | "bytesps" | "count";

// null = not measured: the chart leaves a gap instead of drawing a zero.
interface Point {
  t: number;
  cpu: number;
  iowait: number | null;
  memory: number;
  diskAwait: number | null;
  diskUtil: number | null;
  diskBusiest: string;
  diskRead: number | null;
  diskWrite: number | null;
  netRx: number | null;
  netTx: number | null;
  streaming: number;
  storage: number | null;
  streams: number;
  transcodes: number;
}

type SeriesKey = Exclude<keyof Point, "t" | "diskBusiest">;

interface SeriesDef {
  key: SeriesKey;
  labelKey: string;
}

interface ChartDef {
  id: string;
  titleKey: string;
  unit: Unit;
  series: SeriesDef[];
  // The disk wait chart carries the disk behind each point in its tooltip.
  diskLabel?: boolean;
}

// One chart per unit: two measures of different scales never share an axis.
const CHARTS: ChartDef[] = [
  { id: "cpu", titleKey: "admin.systemHealth.trendsCpu", unit: "percent", series: [
    { key: "cpu", labelKey: "admin.systemHealth.trendsCpuSeries" },
    { key: "iowait", labelKey: "admin.systemHealth.trendsIowaitSeries" },
  ] },
  { id: "memory", titleKey: "admin.systemHealth.trendsMemory", unit: "percent", series: [
    { key: "memory", labelKey: "admin.systemHealth.trendsMemorySeries" },
  ] },
  { id: "diskAwait", titleKey: "admin.systemHealth.trendsDiskAwait", unit: "ms", diskLabel: true, series: [
    { key: "diskAwait", labelKey: "admin.systemHealth.trendsDiskAwaitSeries" },
  ] },
  { id: "diskUtil", titleKey: "admin.systemHealth.trendsDiskUtil", unit: "percent", diskLabel: true, series: [
    { key: "diskUtil", labelKey: "admin.systemHealth.trendsDiskUtilSeries" },
  ] },
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
  { id: "storage", titleKey: "admin.systemHealth.trendsStorage", unit: "percent", series: [
    { key: "storage", labelKey: "admin.systemHealth.trendsStorageSeries" },
  ] },
];

function num(v: string | number | undefined): number {
  const n = typeof v === "string" ? parseInt(v) : (v ?? 0);
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
        diskBusiest: m.diskBusiest ?? "",
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

export function formatValue(unit: Unit, v: number): string {
  switch (unit) {
    case "percent": return `${v.toFixed(1)} %`;
    case "ms": return `${v.toFixed(1)} ms`;
    case "bps": return formatBps(Math.round(v));
    case "bytesps": return formatBytesPerSecond(Math.round(v));
    case "count": return `${Math.round(v)}`;
  }
}

// formatAxis keeps the y labels short enough for the axis gutter: units live
// in the title and the tooltip.
export function formatAxis(unit: Unit, v: number): string {
  switch (unit) {
    case "percent":
    case "ms":
    case "count":
      return `${Math.round(v)}`;
    case "bps":
      if (v >= 1e9) return `${(v / 1e9).toFixed(1)} Gb`;
      if (v >= 1e6) return `${Math.round(v / 1e6)} Mb`;
      if (v >= 1e3) return `${Math.round(v / 1e3)} kb`;
      return `${Math.round(v)}`;
    case "bytesps":
      if (v >= 1e9) return `${(v / 1e9).toFixed(1)} GB`;
      if (v >= 1e6) return `${(v / 1e6).toFixed(1)} MB`;
      if (v >= 1e3) return `${Math.round(v / 1e3)} kB`;
      return `${Math.round(v)}`;
  }
}

// Tick labels follow the window: a day reads by the hour, a month by the day.
export function formatTick(range: TrendRange, t: number): string {
  const d = new Date(t);
  const hm = `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
  const dm = `${String(d.getDate()).padStart(2, "0")}/${String(d.getMonth() + 1).padStart(2, "0")}`;
  if (range === "24h") return hm;
  if (range === "7d") return `${dm} ${hm}`;
  return dm;
}

interface TrendTooltipProps {
  active?: boolean;
  payload?: { payload: Point }[];
  chart: ChartDef;
  t: (key: string) => string;
}

function TrendTooltip({ active, payload, chart, t }: TrendTooltipProps) {
  if (!active || !payload || payload.length === 0) return null;
  const p = payload[0].payload;
  return (
    <div className="rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--surface-raised)] px-3 py-2 text-xs shadow-md">
      <div className="text-[var(--text-muted)] mb-1">{new Date(p.t).toLocaleString()}</div>
      {chart.series.map((s) => (
        <div key={s.key} className="flex justify-between gap-4 text-[var(--text)]">
          <span>{t(s.labelKey)}</span>
          <span className="tabular-nums font-medium">{((v) => (v == null ? "-" : formatValue(chart.unit, v)))(p[s.key])}</span>
        </div>
      ))}
      {chart.diskLabel && p.diskBusiest && (
        <div className="text-[var(--text-muted)] mt-1">{p.diskBusiest}</div>
      )}
    </div>
  );
}

interface TrendChartProps {
  chart: ChartDef;
  points: Point[];
  range: TrendRange;
  colors: string[];
  idleAwaitMs?: number;
  saturationAwaitMs?: number;
}

const CHART_HEIGHT = 180;

function TrendChart({ chart, points, range, colors, idleAwaitMs, saturationAwaitMs }: TrendChartProps) {
  const { t } = useTranslation();
  const { ref, width } = useMeasuredWidth<HTMLDivElement>(560);
  const border = useCssVar("--border", "#444");
  const textMuted = useCssVar("--text-muted", "#888");
  const danger = useCssVar("--color-danger", "#dc2626");
  const yDomain: [number, number | "auto"] = chart.unit === "percent" ? [0, 100] : [0, "auto"];
  return (
    <div
      ref={ref}
      className="p-4 rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)] min-w-0"
      data-testid={`trend-${chart.id}`}
    >
      <h3 className="text-sm font-semibold text-[var(--text)] mb-2">{t(chart.titleKey)}</h3>
      <div role="img" aria-label={t(chart.titleKey)}>
        <LineChart width={Math.max(width - 32, 240)} height={CHART_HEIGHT} data={points} margin={{ top: 8, right: 12, bottom: 0, left: 0 }}>
          <CartesianGrid stroke={border} strokeWidth={1} vertical={false} />
          <XAxis
            dataKey="t"
            type="number"
            domain={["dataMin", "dataMax"]}
            scale="time"
            tickFormatter={(v: number) => formatTick(range, v)}
            tick={{ fontSize: 11, fill: textMuted }}
            axisLine={{ stroke: border }}
            tickLine={false}
            minTickGap={48}
          />
          <YAxis
            domain={yDomain}
            width={56}
            tick={{ fontSize: 11, fill: textMuted }}
            axisLine={false}
            tickLine={false}
            allowDecimals={chart.unit !== "count"}
            tickFormatter={(v: number) => formatAxis(chart.unit, v)}
          />
          <Tooltip content={<TrendTooltip chart={chart} t={t} />} cursor={{ stroke: textMuted, strokeWidth: 1 }} />
          {chart.series.length > 1 && (
            <Legend iconType="plainline" wrapperStyle={{ fontSize: 11 }} formatter={(key: string) => t(chart.series.find((s) => s.key === key)?.labelKey ?? key)} />
          )}
          {chart.id === "diskAwait" && idleAwaitMs !== undefined && idleAwaitMs > 0 && (
            <ReferenceLine y={idleAwaitMs} stroke={textMuted} strokeDasharray="4 4" label={{ value: t("admin.systemHealth.trendsDiskAwaitIdle"), position: "insideTopLeft", fontSize: 10, fill: textMuted }} />
          )}
          {chart.id === "diskAwait" && saturationAwaitMs !== undefined && saturationAwaitMs > 0 && (
            <ReferenceLine y={saturationAwaitMs} stroke={danger} strokeDasharray="4 4" label={{ value: t("admin.systemHealth.trendsDiskAwaitSaturation"), position: "insideTopRight", fontSize: 10, fill: danger }} />
          )}
          {chart.series.map((s, i) => (
            <Line key={s.key} type="monotone" dataKey={s.key} name={s.key} stroke={colors[i % colors.length]} strokeWidth={2} dot={false} activeDot={{ r: 4 }} isAnimationActive={false} />
          ))}
        </LineChart>
      </div>
    </div>
  );
}

// SystemTrends is the history the sampler writes once a minute, folded by the
// server into a few hundred points per window: the live cards above say what
// the machine does now, this says what it did over the week.
export function SystemTrends() {
  const { t } = useTranslation();
  const [range, setRange] = useState<TrendRange>("7d");
  const end = useWindowEnd(range);
  const from = useMemo(() => new Date(end - RANGE_HOURS[range] * 3600_000).toISOString(), [end, range]);
  const to = useMemo(() => new Date(end).toISOString(), [end]);
  const { data, isLoading, isFetching, isError } = useActivityServiceListServerMetrics(
    { from, to },
    { query: { refetchInterval: 60_000, refetchIntervalInBackground: false, placeholderData: keepPreviousData } },
  );
  const points = useMemo(() => toPoints(data?.metrics), [data?.metrics]);
  // Categorical slots in fixed order: a series keeps its colour whatever the
  // chart's neighbours do.
  const colors = [useCssVar("--chart-1", "#c81e1e"), useCssVar("--chart-2", "#f59e0b"), useCssVar("--chart-3", "#10b981")];
  const bucketMinutes = Math.max(1, Math.round((data?.intervalSeconds ?? 60) / 60));

  return (
    <div className="flex flex-col gap-3" data-testid="system-trends">
      <div className="flex flex-wrap items-center gap-3">
        <div className="flex rounded-[var(--radius-md)] border border-[var(--border)] overflow-hidden text-xs" role="group" aria-label={t("admin.systemHealth.trendsRangeLabel")}>
          {RANGES.map((r) => (
            <button
              key={r}
              type="button"
              className={`px-3 py-1 transition-colors ${range === r ? "bg-[var(--accent)] text-[var(--filter-active-text)]" : "bg-[var(--surface)] text-[var(--text-muted)]"}`}
              onClick={() => setRange(r)}
              aria-pressed={range === r}
            >
              {t(`admin.systemHealth.trendsRange_${r}`)}
            </button>
          ))}
        </div>
        {data?.intervalSeconds ? (
          <span className="text-xs text-[var(--text-muted)]">{t("admin.systemHealth.trendsBucket", { minutes: bucketMinutes })}</span>
        ) : null}
      </div>
      {isError ? (
        <p className="text-sm text-[var(--color-danger)]">{t("admin.systemHealth.trendsError")}</p>
      ) : !isLoading && points.length < 2 ? (
        <p className="text-sm text-[var(--text-muted)]">{t("admin.systemHealth.trendsEmpty")}</p>
      ) : (
        <div className={`grid grid-cols-1 lg:grid-cols-2 gap-4 transition-opacity ${isFetching && !isLoading ? "opacity-60" : ""}`}>
          {CHARTS.map((chart) => (
            <TrendChart
              key={chart.id}
              chart={chart}
              points={points}
              range={range}
              colors={colors}
              idleAwaitMs={data?.idleDiskAwaitMs}
              saturationAwaitMs={data?.saturationAwaitMs}
            />
          ))}
        </div>
      )}
      <p className="text-xs text-[var(--text-muted)]">{t("admin.systemHealth.trendsMachineNote")}</p>
    </div>
  );
}
