import { createLazyFileRoute } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { eveyFetch } from "@evey/api";
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
  return useQuery<AnalyticsData>({
    queryKey: ["admin", "analytics"],
    queryFn: () =>
      eveyFetch<AnalyticsData>({
        url: "/api/v1/admin/analytics",
        method: "GET",
      }),
    refetchInterval: 60_000,
    refetchIntervalInBackground: false,
  });
}

function formatDuration(seconds: number): string {
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
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

  const maxPlays = Math.max(...items.map((i) => i.play_count), 1);

  return (
    <div
      className="overflow-x-auto rounded-[var(--radius-lg)] border border-[var(--border)]"
    >
      <table className="w-full text-sm">
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
              <td className="px-4 py-3 font-medium text-[var(--text)]">
                {item.title}
              </td>
              <td className="px-4 py-3 min-w-[160px]">
                <div className="flex items-center gap-2">
                  <div
                    className="h-1.5 flex-1 rounded-full bg-[var(--border)]"
                  >
                    <div
                      className="h-1.5 rounded-full bg-[var(--accent)]"
                      style={{
                        width: `${Math.round((item.play_count / maxPlays) * 100)}%`,
                      }}
                    />
                  </div>
                  <span className="text-xs w-8 text-right text-[var(--text-muted)]">
                    {item.play_count}
                  </span>
                </div>
              </td>
              <td className="px-4 py-3 text-xs text-[var(--text-muted)]">
                {formatDuration(item.total_duration_seconds)}
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
      <table className="w-full text-sm">
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
              <td className="px-4 py-3 font-medium text-[var(--text)]">
                {entry.username}
              </td>
              <td className="px-4 py-3 text-[var(--text)]">
                {entry.media_title}
              </td>
              <td className="px-4 py-3 text-xs text-[var(--text-muted)]">
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
            <StatCard label={t("admin.analytics.playsToday")} value={data.total_plays_today} />
            <StatCard label={t("admin.analytics.playsThisWeek")} value={data.total_plays_this_week} />
            <StatCard label={t("admin.analytics.activeUsersToday")} value={data.active_users_today} />
          </div>

          <div>
            <h2 className="text-sm font-semibold uppercase tracking-wide mb-3 text-[var(--text-muted)]">
              {t("admin.analytics.topMedia")}
            </h2>
            <TopMediaTable items={data.top_media} />
          </div>

          <div>
            <h2 className="text-sm font-semibold uppercase tracking-wide mb-3 text-[var(--text-muted)]">
              {t("admin.analytics.recentActivity")}
            </h2>
            <RecentActivityTable entries={data.recent_activity} />
          </div>
        </>
      ) : null}
    </div>
  );
}
