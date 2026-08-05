import { createLazyFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useActivityServiceGetAnalytics, formatDurationSec } from "@evey/api";
import { Spinner } from "@/components/ui/Spinner";
import { PageHeader } from "@/components/ui/PageHeader";

export const Route = createLazyFileRoute("/_auth/admin/analytics")({
  component: AnalyticsPage,
});

interface TopMediaItem {
  media_item_id: string;
  title: string;
  play_count: number;
  total_duration_seconds: number;
}

interface RecentActivityEntry {
  user_id: string;
  username: string;
  media_title: string;
  played_at: string;
}

interface AnalyticsData {
  top_media: TopMediaItem[];
  recent_activity: RecentActivityEntry[];
  total_plays_today: number;
  total_plays_this_week: number;
  active_users_today: number;
}

function useAnalytics() {
  return useActivityServiceGetAnalytics({
    query: {
      refetchInterval: 60_000,
      refetchIntervalInBackground: false,
      select: (d): AnalyticsData => ({
        top_media: (d.topMedia ?? []).map((m) => ({
          media_item_id: m.mediaItemId ?? "",
          title: m.title ?? "",
          play_count: m.playCount ?? 0,
          total_duration_seconds: Number(m.totalDurationSeconds ?? 0),
        })),
        recent_activity: (d.recentActivity ?? []).map((a) => ({
          user_id: a.userId ?? "",
          username: a.username ?? "",
          media_title: a.mediaTitle ?? "",
          played_at: a.playedAt ?? "",
        })),
        total_plays_today: d.totalPlaysToday ?? 0,
        total_plays_this_week: d.totalPlaysThisWeek ?? 0,
        active_users_today: d.activeUsersToday ?? 0,
      }),
    },
  });
}

function formatDate(d: string): string {
  return new Date(d).toLocaleString();
}

function StatCard({ label, value }: { label: string; value: number }) {
  return (
    <div
      className="flex flex-col items-center justify-center p-6 rounded-[var(--radius-lg)] gap-1 bg-[var(--surface)] border border-[var(--border)]"
    >
      <span className="text-3xl font-bold text-[var(--text)]">
        {value}
      </span>
      <span className="text-xs font-semibold uppercase tracking-wide text-[var(--text-muted)]">
        {label}
      </span>
    </div>
  );
}

function TopMediaTable({ items }: { items: TopMediaItem[] }) {
  const { t } = useTranslation();

  if (items.length === 0) {
    return (
      <div className="py-8 text-center text-sm text-[var(--text-muted)]">
        {t("admin.analytics.noPlays")}
      </div>
    );
  }

  const maxDuration = Math.max(...items.map((i) => i.total_duration_seconds), 1);

  return (
    <div
      className="overflow-x-auto rounded-[var(--radius-lg)] border border-[var(--border)]"
    >
      <table className="w-full text-sm table-cards">
        <thead>
          <tr className="bg-[var(--surface)] border-b border-[var(--border)]">
            {[t("admin.analytics.tableHeaders.title"), t("admin.analytics.tableHeaders.plays"), t("admin.analytics.tableHeaders.totalWatchTime")].map((h) => (
              <th
                key={h}
                className="px-4 py-2 text-left font-medium text-[var(--text-muted)]"
              >
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {items.map((item) => (
            <tr key={item.media_item_id} className="border-b border-[var(--border)]">
              <td data-label={t("admin.analytics.tableHeaders.title")} className="px-4 py-3 font-medium text-[var(--text)]">
                {item.title}
              </td>
              <td data-label={t("admin.analytics.tableHeaders.plays")} className="px-4 py-3 text-xs text-[var(--text-muted)]">
                {item.play_count}
              </td>
              <td data-label={t("admin.analytics.tableHeaders.totalWatchTime")} className="px-4 py-3 min-w-[200px]">
                <div className="flex items-center gap-2">
                  <div className="h-1.5 flex-1 rounded-full bg-[var(--border)]">
                    <div
                      className="h-1.5 rounded-full bg-[var(--accent)]"
                      style={{
                        width: `${Math.round((item.total_duration_seconds / maxDuration) * 100)}%`,
                      }}
                    />
                  </div>
                  <span className="text-xs w-16 text-right text-[var(--text-muted)] tabular-nums">
                    {formatDurationSec(item.total_duration_seconds) || "0min"}
                  </span>
                </div>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function RecentActivityTable({ entries }: { entries: RecentActivityEntry[] }) {
  const { t } = useTranslation();

  if (entries.length === 0) {
    return (
      <div className="py-8 text-center text-sm text-[var(--text-muted)]">
        {t("admin.analytics.noActivity")}
      </div>
    );
  }

  return (
    <div
      className="overflow-x-auto rounded-[var(--radius-lg)] border border-[var(--border)]"
    >
      <table className="w-full text-sm table-cards">
        <thead>
          <tr className="bg-[var(--surface)] border-b border-[var(--border)]">
            {[t("admin.analytics.tableHeaders.user"), t("admin.analytics.tableHeaders.media"), t("admin.analytics.tableHeaders.when")].map((h) => (
              <th
                key={h}
                className="px-4 py-2 text-left font-medium text-[var(--text-muted)]"
              >
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {entries.map((entry, i) => (
            <tr key={`${entry.user_id}-${entry.played_at}-${i}`} className="border-b border-[var(--border)]">
              <td data-label={t("admin.analytics.tableHeaders.user")} className="px-4 py-3 font-medium text-[var(--text)]">
                {entry.username}
              </td>
              <td data-label={t("admin.analytics.tableHeaders.media")} className="px-4 py-3 text-[var(--text)]">
                {entry.media_title}
              </td>
              <td data-label={t("admin.analytics.tableHeaders.when")} className="px-4 py-3 text-xs text-[var(--text-muted)]">
                {formatDate(entry.played_at)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function AnalyticsPage() {
  const { t } = useTranslation();
  const { data, isLoading, error } = useAnalytics();

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t("admin.analytics.title")} />

      {isLoading ? (
        <div className="flex justify-center py-16">
          <Spinner size="lg" />
        </div>
      ) : error ? (
        <div className="py-8 text-center text-sm text-[var(--text-muted)]">
          {t("admin.analytics.failedLoad")}
        </div>
      ) : data ? (
        <>
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
            <StatCard label={t("admin.analytics.playsToday")} value={data.total_plays_today ?? 0} />
            <StatCard label={t("admin.analytics.playsThisWeek")} value={data.total_plays_this_week ?? 0} />
            <StatCard label={t("admin.analytics.activeUsersToday")} value={data.active_users_today ?? 0} />
          </div>

          <div>
            <h2 className="text-sm font-semibold uppercase tracking-wide mb-3 text-[var(--text-muted)]">
              {t("admin.analytics.topMedia")}
            </h2>
            <TopMediaTable items={data.top_media ?? []} />
          </div>

          <div>
            <h2 className="text-sm font-semibold uppercase tracking-wide mb-3 text-[var(--text-muted)]">
              {t("admin.analytics.recentActivity")}
            </h2>
            <RecentActivityTable entries={data.recent_activity ?? []} />
          </div>
        </>
      ) : null}
    </div>
  );
}
