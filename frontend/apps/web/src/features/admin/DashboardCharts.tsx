import { useState, useRef, useEffect, useCallback, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useActivityServiceGetAnalytics, formatBytes } from "@evey/api";
import { BarChart, Bar, XAxis, YAxis, Cell, PieChart, Pie } from "recharts";

import { useCssVar } from "@/hooks/useCssVar";
import { CHART_COLORS } from "@/constants/chartColors";

const CHART_HEIGHT = 200;

// recharts is driven with an explicit width (not ResponsiveContainer): a fixed
// width per bar keeps the horizontal-scroll behaviour these charts had with
// gifted-charts, and it renders under jsdom (ResponsiveContainer measures 0x0
// there and draws nothing).
function barChartWidth(count: number, step: number, pad = 48) {
  return Math.max(count * step + pad, 240);
}

export function PlayActivityChart() {
  const { t } = useTranslation();
  const [mode, setMode] = useState<"plays" | "watchTime">("plays");
  const scrollRef = useRef<HTMLDivElement>(null);
  const scrollToEnd = useCallback(() => {
    const el = scrollRef.current;
    if (el) el.scrollLeft = el.scrollWidth;
  }, []);
  const { data } = useActivityServiceGetAnalytics({
    query: { refetchInterval: 60_000, refetchIntervalInBackground: false },
  });
  const daily = data?.dailyPlays ?? [];
  const accent = useCssVar("--accent", "#4f46e5");
  const textMuted = useCssVar("--text-muted");

  useEffect(() => { scrollToEnd(); }, [daily.length, scrollToEnd]);

  const barData = useMemo(() => daily.map((d) => {
    const parts = (d.date ?? "").split("-");
    const label = parts.length >= 3 ? `${parts[2]}/${parts[1]}` : (d.date ?? "");
    const value = mode === "plays"
      ? (d.playCount ?? 0)
      : Math.round(parseInt(d.watchTimeSeconds ?? "0") / 3600 * 10) / 10;
    return { value, label };
  }), [daily, mode]);

  const maxVal = Math.max(...barData.map((d) => d.value), 1);

  if (daily.length === 0) return null;

  return (
    <div className="p-4 rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)] min-w-0">
      <div className="flex items-center justify-between mb-3 gap-2">
        <h3 className="text-sm font-semibold text-[var(--text)] truncate min-w-0">
          {t("admin.dashboardStats.playActivity")}
        </h3>
        <div className="flex rounded-[var(--radius-md)] border border-[var(--border)] overflow-hidden text-xs shrink-0">
          <button
            className={`px-2 py-1 transition-colors ${mode === "plays" ? "bg-[var(--accent)] text-[var(--filter-active-text)]" : "bg-[var(--surface)] text-[var(--text-muted)]"}`}
            onClick={() => setMode("plays")}
            aria-label={t("admin.dashboardStats.plays")}
            aria-pressed={mode === "plays"}
          >
            {t("admin.dashboardStats.plays")}
          </button>
          <button
            className={`px-2 py-1 transition-colors ${mode === "watchTime" ? "bg-[var(--accent)] text-[var(--filter-active-text)]" : "bg-[var(--surface)] text-[var(--text-muted)]"}`}
            onClick={() => setMode("watchTime")}
            aria-label={t("admin.dashboardStats.watchTime")}
            aria-pressed={mode === "watchTime"}
          >
            {t("admin.dashboardStats.watchTime")}
          </button>
        </div>
      </div>
      <div className="overflow-x-scroll overflow-y-hidden" style={{ scrollbarWidth: "thin" }} ref={scrollRef}>
        <BarChart width={barChartWidth(barData.length, 40)} height={CHART_HEIGHT} data={barData} barCategoryGap={12}>
          <XAxis dataKey="label" tick={{ fill: textMuted, fontSize: 10 }} axisLine={false} tickLine={false} interval={0} />
          <YAxis
            tick={{ fill: textMuted, fontSize: 11 }}
            axisLine={false}
            tickLine={false}
            width={34}
            tickCount={5}
            domain={[0, Math.ceil(maxVal * 1.1)]}
            tickFormatter={(v: number) => (mode === "watchTime" ? `${v}h` : `${v}`)}
          />
          <Bar dataKey="value" fill={accent} radius={[4, 4, 0, 0]} maxBarSize={28} isAnimationActive={false} />
        </BarChart>
      </div>
    </div>
  );
}

export function StorageDonut({ storage }: { storage: { path?: string; totalBytes?: string; usedBytes?: string; freeBytes?: string }[] }) {
  const { t } = useTranslation();

  if (!storage || storage.length === 0) return null;

  const usedColor = CHART_COLORS[0];
  const freeColor = CHART_COLORS[2];

  return (
    <div className="p-4 rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]">
      <h3 className="text-sm font-semibold mb-4 text-[var(--text)]">
        {t("admin.dashboardStats.storage")}
      </h3>
      <div className="flex items-center justify-center gap-8 flex-wrap">
        {storage.map((s, i) => {
          const used = parseInt(s.usedBytes ?? "0");
          const total = parseInt(s.totalBytes ?? "1");
          const free = total - used;
          const pct = total > 0 ? Math.round((used / total) * 100) : 0;
          const pieData = [
            { name: "used", value: used, color: usedColor },
            { name: "free", value: free, color: freeColor },
          ];
          return (
            <div key={i} className="flex flex-col items-center gap-2">
              <div className="relative" style={{ width: 100, height: 100 }}>
                <PieChart width={100} height={100}>
                  <Pie
                    data={pieData}
                    dataKey="value"
                    innerRadius={30}
                    outerRadius={50}
                    startAngle={90}
                    endAngle={-270}
                    stroke="none"
                    isAnimationActive={false}
                  >
                    {pieData.map((entry) => (
                      <Cell key={entry.name} fill={entry.color} />
                    ))}
                  </Pie>
                </PieChart>
                <span className="absolute inset-0 flex items-center justify-center text-xs font-bold text-[var(--text)]">
                  {pct}%
                </span>
              </div>
              <span className="text-xs text-[var(--text-muted)] truncate max-w-[140px]" title={s.path}>{s.path}</span>
              <span className="text-xs text-[var(--text-muted)]">{formatBytes(s.usedBytes)} / {formatBytes(s.totalBytes)}</span>
            </div>
          );
        })}
      </div>
      <div className="flex items-center justify-center gap-4 mt-3 text-xs">
        <div className="flex items-center gap-1.5">
          <span className="w-2.5 h-2.5 rounded-full" style={{ backgroundColor: usedColor }} />
          <span className="text-[var(--text-muted)]">{t("admin.dashboardStats.used")}</span>
        </div>
        <div className="flex items-center gap-1.5">
          <span className="w-2.5 h-2.5 rounded-full" style={{ backgroundColor: freeColor }} />
          <span className="text-[var(--text-muted)]">{t("admin.dashboardStats.free")}</span>
        </div>
      </div>
    </div>
  );
}

export function PeakHours() {
  const { t } = useTranslation();
  const { data } = useActivityServiceGetAnalytics({
    query: { refetchInterval: 60_000, refetchIntervalInBackground: false },
  });
  const hourly = data?.hourlyPlays ?? [];
  const accent = useCssVar("--accent", "#4f46e5");
  const textMuted = useCssVar("--text-muted");

  const allHours = useMemo(() => Array.from({ length: 24 }, (_, i) => {
    const entry = hourly.find((h) => h.hour === i);
    return { value: entry?.count ?? 0, label: `${i}h` };
  }), [hourly]);

  const maxVal = Math.max(...allHours.map((d) => d.value), 1);

  if (hourly.length === 0) return null;

  return (
    <div className="p-4 rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]">
      <h3 className="text-sm font-semibold mb-4 text-[var(--text)]">
        {t("admin.dashboardStats.peakHours")}
      </h3>
      <div className="overflow-x-auto">
        <BarChart width={barChartWidth(allHours.length, 22)} height={CHART_HEIGHT} data={allHours} barCategoryGap={4}>
          <XAxis dataKey="label" tick={{ fill: textMuted, fontSize: 9 }} axisLine={false} tickLine={false} interval={0} />
          <YAxis
            tick={{ fill: textMuted, fontSize: 11 }}
            axisLine={false}
            tickLine={false}
            width={30}
            tickCount={4}
            domain={[0, Math.ceil(maxVal * 1.1)]}
          />
          <Bar dataKey="value" fill={accent} radius={[3, 3, 0, 0]} maxBarSize={14} isAnimationActive={false} />
        </BarChart>
      </div>
    </div>
  );
}
