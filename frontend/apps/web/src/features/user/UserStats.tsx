import { useTranslation } from "react-i18next";
import { Link } from "@tanstack/react-router";
import { useActivityServiceGetUserStats } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { BarChart, Bar, XAxis, YAxis, PieChart, Pie, Cell } from "recharts";
import { Spinner } from "@/components/ui/Spinner";
import { EmptyState } from "@/components/ui/EmptyState";
import { FetchErrorState } from "@/components/ui/FetchErrorState";
import { BarChart3 } from "lucide-react";

import { CHART_COLORS as GENRE_COLORS } from "@/constants/chartColors";

import { useCssVar } from "@/hooks/useCssVar";
import { MS_PER_HOUR, MS_PER_MINUTE } from "@/lib/time";

function formatWatchTime(ms: string | undefined): string {
  const total = parseInt(ms ?? "0");
  const h = Math.floor(total / MS_PER_HOUR);
  const m = Math.floor((total % MS_PER_HOUR) / MS_PER_MINUTE);
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

function formatHours(ms: string | undefined): string {
  const h = parseInt(ms ?? "0") / MS_PER_HOUR;
  if (h >= 1) return `${Math.round(h * 10) / 10}h`;
  const m = Math.round(parseInt(ms ?? "0") / MS_PER_MINUTE);
  return `${m}m`;
}

function StatCard({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="flex flex-col items-center justify-center p-5 rounded-[var(--radius-lg)] gap-1 bg-[var(--surface)] border border-[var(--border)]">
      <span className="text-2xl font-bold text-[var(--text)]">{value}</span>
      <span className="text-xs font-medium uppercase tracking-wide text-[var(--text-muted)]">{label}</span>
    </div>
  );
}

function buildMonthlyData(byMonth: { year?: number; month?: number; totalTimeMs?: string }[]) {
  const now = new Date();
  const result = [];
  for (let i = 11; i >= 0; i--) {
    const d = new Date(now.getFullYear(), now.getMonth() - i, 1);
    const y = d.getFullYear();
    const m = d.getMonth() + 1;
    const label = d.toLocaleDateString(undefined, { month: "short" });
    const entry = byMonth.find((e) => e.year === y && e.month === m);
    const hours = parseInt(entry?.totalTimeMs ?? "0") / MS_PER_HOUR;
    result.push({ value: Math.round(hours * 10) / 10, label });
  }
  return result;
}

export function UserStats() {
  const { t } = useTranslation();
  const profileId = useAuthStore((s) => s.profileId);
  const { data, isLoading, isError, error, refetch } = useActivityServiceGetUserStats(profileId ?? "", undefined, {
    query: { enabled: !!profileId },
  });
  // Hooks must run on every render - keep them above the early returns below
  // (they used to sit after them, a conditional-hook bug).
  const accent = useCssVar("--accent", "#4f46e5");
  const textMuted = useCssVar("--text-muted");
  // Genres arrive already folded and labelled by the server (GetUserStats):
  // it is the only side that knows every stored spelling of a genre, so a
  // second table here could only disagree with it - which is exactly how the
  // pie ended up with a "Drama" slice next to a "Drame" one.

  if (isLoading) {
    return (
      <div className="flex justify-center py-16">
        <Spinner size="lg" />
      </div>
    );
  }

  // !data merged "nothing watched yet" with "the request failed".
  if (isError) {
    return <FetchErrorState error={error} onRetry={() => void refetch()} />;
  }

  if (!data || (data.totalPlays ?? 0) === 0) {
    return <EmptyState icon={<BarChart3 size={48} strokeWidth={1.5} />} title={t("common.userStats.noData")} />;
  }

  const genres = [...(data.genres ?? [])].sort((a, b) => parseInt(b.totalTimeMs ?? "0") - parseInt(a.totalTimeMs ?? "0"));
  const pieData = genres.slice(0, 8).map((g, i) => ({
    value: parseInt(g.totalTimeMs ?? "0"),
    color: GENRE_COLORS[i % GENRE_COLORS.length],
    text: g.genre ?? "",
    percent: Math.round(g.percent ?? 0),
  }));

  const monthBarData = buildMonthlyData(data.byMonth ?? []);
  const monthMax = Math.max(...monthBarData.map((d) => d.value), 0.1);

  const mostPlayed = data.mostPlayed ?? [];
  const favoriteGenre = genres.length > 0 ? genres[0].genre ?? "-" : "-";

  return (
    <div className="flex flex-col gap-6">
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
        <StatCard label={t("common.userStats.totalWatchTime")} value={formatWatchTime(data.totalWatchTimeMs)} />
        <StatCard label={t("common.userStats.totalPlays")} value={data.totalPlays ?? 0} />
        <StatCard label={t("common.userStats.uniqueItems")} value={data.uniqueItemsWatched ?? 0} />
        <StatCard label={t("common.userStats.favoriteGenre")} value={favoriteGenre} />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        {pieData.length > 0 && (
          <div className="p-4 rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]">
            <h3 className="text-sm font-semibold mb-4 text-[var(--text)]">
              {t("common.userStats.genreBreakdown")}
            </h3>
            <div className="flex items-center justify-center gap-6">
              <div role="img" aria-label={t("common.userStats.genreBreakdown")}>
              <PieChart width={160} height={160}>
                <Pie
                  data={pieData}
                  dataKey="value"
                  innerRadius={40}
                  outerRadius={70}
                  startAngle={90}
                  endAngle={-270}
                  stroke="none"
                  isAnimationActive={false}
                >
                  {pieData.map((entry) => (
                    <Cell key={entry.text} fill={entry.color} />
                  ))}
                </Pie>
              </PieChart>
              </div>
              <div className="flex flex-col gap-1.5">
                {pieData.map((d) => (
                  <div key={d.text} className="flex items-center gap-2 text-xs">
                    <span className="w-2.5 h-2.5 rounded-full shrink-0" style={{ backgroundColor: d.color }} />
                    <span className="text-[var(--text)]">{d.text}</span>
                    <span className="text-[var(--text-muted)]">{d.percent}%</span>
                  </div>
                ))}
              </div>
            </div>
          </div>
        )}

        <div className="p-4 rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]">
          <h3 className="text-sm font-semibold mb-4 text-[var(--text)]">
            {t("common.userStats.monthlyActivity")}
          </h3>
          <div className="overflow-x-auto overflow-y-visible" role="img" aria-label={t("common.userStats.monthlyActivity")}>
            <BarChart width={Math.max(monthBarData.length * 30 + 44, 320)} height={200} data={monthBarData} barCategoryGap={8}>
              <XAxis dataKey="label" tick={{ fill: textMuted, fontSize: 10 }} axisLine={false} tickLine={false} interval={0} />
              <YAxis
                tick={{ fill: textMuted, fontSize: 11 }}
                axisLine={false}
                tickLine={false}
                width={34}
                tickCount={5}
                domain={[0, Math.ceil(monthMax * 1.1) || 1]}
                tickFormatter={(v: number) => `${v}h`}
              />
              <Bar dataKey="value" fill={accent} radius={[4, 4, 0, 0]} maxBarSize={20} isAnimationActive={false} />
            </BarChart>
          </div>
        </div>
      </div>

      {mostPlayed.length > 0 && (
        <div>
          <h3 className="text-sm font-semibold mb-3 text-[var(--text)]">
            {t("common.userStats.mostPlayed")}
          </h3>
          <div className="flex flex-col gap-1">
            {mostPlayed.map((m, i) => (
              <Link
                key={m.mediaItemId ?? i}
                to="/media/$mediaId"
                params={{ mediaId: m.mediaItemId ?? "" }}
                className="flex items-center gap-3 px-3 py-2 rounded-[var(--radius-md)] hover:bg-[var(--surface-raised)] transition-colors-fast"
              >
                <span className="text-sm font-mono w-6 text-center text-[var(--accent)]">
                  {i + 1}
                </span>
                <span className="text-sm flex-1 truncate text-[var(--text)]">
                  {m.title ?? "Unknown"}
                </span>
                <span className="text-xs text-[var(--text-muted)]">
                  {m.playCount ?? 0} {t("common.userStats.playsLabel")}
                </span>
                <span className="text-xs text-[var(--text-muted)]">
                  {formatHours(m.totalTimeMs)}
                </span>
              </Link>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
