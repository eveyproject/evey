import { useState, useRef, useEffect, useCallback } from "react";
import { useTranslation } from "react-i18next";
import {
  useActivityServiceGetAnalytics,
} from "@evey/api";
import { BarChart, PieChart } from "react-gifted-charts";
import { formatBytes } from "./DashboardStatCard";

function useCssVar(name: string, fallback = "#888") {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim() || fallback;
}

import { CHART_COLORS } from "@/constants/chartColors";

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

  useEffect(() => { scrollToEnd(); }, [daily.length, scrollToEnd]);

  if (daily.length === 0) return null;

  const accent = useCssVar("--accent", "#6366f1");
  const textMuted = useCssVar("--text-muted");

  const barData = daily.map((d) => {
    const [, mm, dd] = (d.date ?? "").split("-");
    const label = `${dd}/${mm}`;
    const value = mode === "plays"
      ? (d.playCount ?? 0)
      : Math.round(parseInt(d.watchTimeSeconds ?? "0") / 3600 * 10) / 10;
    return { value, label, frontColor: accent };
  });

  const maxVal = Math.max(...barData.map((d) => d.value), 1);

  return (
    <div className="p-4 rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)] min-w-0">
      <div className="flex items-center justify-between mb-3 gap-2">
        <h3 className="text-sm font-semibold text-[var(--text)] whitespace-nowrap">
          {t("admin.dashboardStats.playActivity")}
        </h3>
        <div className="flex rounded-[var(--radius-md)] border border-[var(--border)] overflow-hidden text-xs shrink-0">
          <button
            className={`px-2 py-1 transition-colors ${mode === "plays" ? "bg-[var(--accent)] text-white" : "bg-[var(--surface)] text-[var(--text-muted)]"}`}
            onClick={() => setMode("plays")}
          >
            {t("admin.dashboardStats.plays")}
          </button>
          <button
            className={`px-2 py-1 transition-colors ${mode === "watchTime" ? "bg-[var(--accent)] text-white" : "bg-[var(--surface)] text-[var(--text-muted)]"}`}
            onClick={() => setMode("watchTime")}
          >
            {t("admin.dashboardStats.watchTime")}
          </button>
        </div>
      </div>
      <div
        className="overflow-x-scroll overflow-y-hidden"
        style={{ scrollbarWidth: "thin" }}
        ref={scrollRef}
      >
        <BarChart
          data={barData}
          barWidth={28}
          spacing={12}
          roundedTop
          roundedBottom
          noOfSections={4}
          maxValue={Math.ceil(maxVal * 1.1)}
          yAxisTextStyle={{ color: textMuted, fontSize: 11 }}
          xAxisLabelTextStyle={{ color: textMuted, fontSize: 10 }}
          yAxisLabelSuffix={mode === "watchTime" ? "h" : ""}
          hideRules
          isAnimated
          barBorderTopLeftRadius={4}
          barBorderTopRightRadius={4}
        />
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
            { value: used, color: usedColor },
            { value: free, color: freeColor },
          ];
          return (
            <div key={i} className="flex flex-col items-center gap-2">
              <PieChart
                data={pieData}
                radius={50}
                innerRadius={30}
                donut
                showText={false}
                centerLabelComponent={() => (
                  <span className="text-xs font-bold text-[var(--text)]">{pct}%</span>
                )}
              />
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

  if (hourly.length === 0) return null;

  const accent = useCssVar("--accent", "#6366f1");
  const textMuted = useCssVar("--text-muted");

  const allHours = Array.from({ length: 24 }, (_, i) => {
    const entry = hourly.find((h) => h.hour === i);
    return { value: entry?.count ?? 0, label: `${i}h`, frontColor: accent };
  });

  const maxVal = Math.max(...allHours.map((d) => d.value), 1);

  return (
    <div className="p-4 rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]">
      <h3 className="text-sm font-semibold mb-4 text-[var(--text)]">
        {t("admin.dashboardStats.peakHours")}
      </h3>
      <div className="overflow-x-auto">
        <BarChart
          data={allHours}
          barWidth={14}
          spacing={4}
          roundedTop
          noOfSections={3}
          maxValue={Math.ceil(maxVal * 1.1)}
          yAxisTextStyle={{ color: textMuted, fontSize: 11 }}
          xAxisLabelTextStyle={{ color: textMuted, fontSize: 9 }}
          hideRules
          isAnimated
          barBorderTopLeftRadius={3}
          barBorderTopRightRadius={3}
        />
      </div>
    </div>
  );
}
