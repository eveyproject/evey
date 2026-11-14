import { useState, useEffect, useMemo, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { useActivityServiceGetAnalytics } from "@evey/api";
import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip } from "recharts";

const CHART_HEIGHT = 200;

// Colours are CSS variables, not values read once from getComputedStyle: the
// chart then follows the theme as it loads or changes, like the rest of the page.
const ACCENT = "var(--accent)";
const TEXT_MUTED = "var(--text-muted)";
const GRID = "var(--border)";

// The chart fills its card. A minimum width per bar keeps the labels readable
// on a narrow screen, where the chart scrolls sideways instead of squeezing.
// recharts gets an explicit width (not ResponsiveContainer, which measures 0x0
// under jsdom and draws nothing), measured from the wrapper. A callback ref,
// not an effect on a ref object: the wrapper only mounts once the analytics
// arrive, after the first render, and an effect would never see it.
function useContainerWidth(): [(el: HTMLDivElement | null) => void, number, HTMLDivElement | null] {
  const [el, setEl] = useState<HTMLDivElement | null>(null);
  const [width, setWidth] = useState(0);
  useEffect(() => {
    if (!el) return;
    setWidth(el.clientWidth);
    if (typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver(() => setWidth(el.clientWidth));
    ro.observe(el);
    return () => ro.disconnect();
  }, [el]);
  return [setEl, width, el];
}

function chartWidth(available: number, count: number, minStep: number): number {
  return Math.max(available, count * minStep + 48);
}

function ChartCard({ title, actions, children }: { title: string; actions?: ReactNode; children: ReactNode }) {
  return (
    <div className="p-4 rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)] min-w-0">
      <div className="flex items-center justify-between gap-2 h-7 mb-3">
        <h3 className="text-sm font-semibold text-[var(--text)] truncate min-w-0">{title}</h3>
        {actions}
      </div>
      {children}
    </div>
  );
}

function ChartTooltip({ active, payload, label, format }: {
  active?: boolean;
  payload?: Array<{ value?: number }>;
  label?: string;
  format: (v: number) => string;
}) {
  if (!active || !payload?.length) return null;
  return (
    <div className="px-2.5 py-1.5 rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--surface-raised)] text-xs shadow-[var(--shadow-md)]">
      <div className="text-[var(--text-muted)]">{label}</div>
      <div className="font-semibold text-[var(--text)] tabular-nums">{format(payload[0].value ?? 0)}</div>
    </div>
  );
}

const axisTick = { fill: TEXT_MUTED, fontSize: 11 };

export function PlayActivityChart() {
  const { t } = useTranslation();
  const [mode, setMode] = useState<"plays" | "watchTime">("plays");
  const [scrollRef, available, scrollEl] = useContainerWidth();
  const { data } = useActivityServiceGetAnalytics({
    query: { refetchInterval: 60_000, refetchIntervalInBackground: false },
  });
  const daily = useMemo(() => data?.dailyPlays ?? [], [data?.dailyPlays]);

  const barData = useMemo(() => daily.map((d) => {
    const parts = (d.date ?? "").split("-");
    const label = parts.length >= 3 ? `${parts[2]}/${parts[1]}` : (d.date ?? "");
    const value = mode === "plays"
      ? (d.playCount ?? 0)
      : Math.round(parseInt(d.watchTimeSeconds ?? "0") / 3600 * 10) / 10;
    return { value, label };
  }), [daily, mode]);

  const width = chartWidth(available, barData.length, 36);
  // On a narrow screen the chart scrolls: show the latest days first.
  useEffect(() => {
    if (scrollEl) scrollEl.scrollLeft = scrollEl.scrollWidth;
  }, [scrollEl, width]);

  if (daily.length === 0) return null;

  const format = (v: number) => (mode === "watchTime" ? `${v} h` : `${v}`);
  const toggle = (value: "plays" | "watchTime", label: string) => (
    <button
      type="button"
      className={`px-2.5 py-1 rounded-[var(--radius-sm)] transition-colors ${mode === value ? "bg-[var(--accent)] text-[var(--filter-active-text)]" : "text-[var(--text-muted)] hover:text-[var(--text)]"}`}
      onClick={() => setMode(value)}
      aria-label={label}
      aria-pressed={mode === value}
    >
      {label}
    </button>
  );

  return (
    <ChartCard
      title={t("admin.dashboardStats.playActivity")}
      actions={
        <div className="flex p-0.5 gap-0.5 rounded-[var(--radius-md)] bg-[var(--surface-raised)] text-xs shrink-0">
          {toggle("plays", t("admin.dashboardStats.plays"))}
          {toggle("watchTime", t("admin.dashboardStats.watchTime"))}
        </div>
      }
    >
      <div
        className="overflow-x-auto overflow-y-hidden"
        style={{ scrollbarWidth: "thin" }}
        ref={scrollRef}
        role="img"
        aria-label={t("admin.dashboardStats.playActivity")}
      >
        <BarChart width={width} height={CHART_HEIGHT} data={barData} barCategoryGap="25%" margin={{ top: 4, right: 4, bottom: 0, left: 0 }}>
          <CartesianGrid vertical={false} stroke={GRID} strokeDasharray="3 3" />
          <XAxis dataKey="label" tick={{ ...axisTick, fontSize: 10 }} axisLine={false} tickLine={false} interval="preserveStartEnd" minTickGap={8} />
          <YAxis
            tick={axisTick}
            axisLine={false}
            tickLine={false}
            width={36}
            allowDecimals={mode === "watchTime"}
            domain={[0, "auto"]}
            tickFormatter={format}
          />
          <Tooltip cursor={{ fill: "var(--surface-hover)" }} content={<ChartTooltip format={format} />} isAnimationActive={false} />
          <Bar dataKey="value" fill={ACCENT} radius={[4, 4, 0, 0]} maxBarSize={32} isAnimationActive={false} />
        </BarChart>
      </div>
    </ChartCard>
  );
}

export function PeakHours() {
  const { t } = useTranslation();
  const [scrollRef, available] = useContainerWidth();
  const { data } = useActivityServiceGetAnalytics({
    query: { refetchInterval: 60_000, refetchIntervalInBackground: false },
  });
  const hourly = useMemo(() => data?.hourlyPlays ?? [], [data?.hourlyPlays]);

  const allHours = useMemo(() => Array.from({ length: 24 }, (_, i) => {
    const entry = hourly.find((h) => h.hour === i);
    return { value: entry?.count ?? 0, label: `${i}h` };
  }), [hourly]);

  if (hourly.length === 0) return null;

  const format = (v: number) => `${v}`;

  return (
    <ChartCard title={t("admin.dashboardStats.peakHours")}>
      <div
        className="overflow-x-auto overflow-y-hidden"
        style={{ scrollbarWidth: "thin" }}
        ref={scrollRef}
        role="img"
        aria-label={t("admin.dashboardStats.peakHours")}
      >
        <BarChart width={chartWidth(available, allHours.length, 20)} height={CHART_HEIGHT} data={allHours} barCategoryGap="20%" margin={{ top: 4, right: 4, bottom: 0, left: 0 }}>
          <CartesianGrid vertical={false} stroke={GRID} strokeDasharray="3 3" />
          <XAxis dataKey="label" tick={{ ...axisTick, fontSize: 10 }} axisLine={false} tickLine={false} interval="preserveStartEnd" minTickGap={6} />
          <YAxis tick={axisTick} axisLine={false} tickLine={false} width={36} allowDecimals={false} domain={[0, "auto"]} />
          <Tooltip cursor={{ fill: "var(--surface-hover)" }} content={<ChartTooltip format={format} />} isAnimationActive={false} />
          <Bar dataKey="value" fill={ACCENT} radius={[3, 3, 0, 0]} maxBarSize={24} isAnimationActive={false} />
        </BarChart>
      </div>
    </ChartCard>
  );
}
