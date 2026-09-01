import { useState, useCallback, useEffect, useRef } from "react";
import { Film, Download } from "lucide-react";
import { createLazyFileRoute, useNavigate } from "@tanstack/react-router";
import { useQuery, useInfiniteQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import { eveyFetch, activityServiceGetPlayHistory, activityServiceGetServerStats, adminSessionServiceGetAuditLog, formatBytes, formatDurationMs, collectAllPages, EXPORT_PAGE_SIZE, EXPORT_MAX_ROWS } from "@evey/api";
import { useActivityServiceGetServerStats } from "@evey/api";
import { Spinner } from "@/components/ui/Spinner";
import { Card } from "@/components/ui/Card";
import { EmptyState } from "@/components/ui/EmptyState";
import { FetchErrorState } from "@/components/ui/FetchErrorState";
import { PageHeader } from "@/components/ui/PageHeader";
import { AuditLogTable } from "@/features/admin/AuditLogTable";
import { WatcherFilter, type Watcher } from "@/features/admin/WatcherFilter";

// useInfiniteSentinel wires an IntersectionObserver on a bottom sentinel div to
// auto-load the next page when it scrolls into view. Returns the ref to attach.
function useInfiniteSentinel(hasNext: boolean, isFetching: boolean, fetchNext: () => void) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const obs = new IntersectionObserver(
      (entries) => {
        if (entries[0]?.isIntersecting && hasNext && !isFetching) fetchNext();
      },
      { rootMargin: "300px" },
    );
    obs.observe(el);
    return () => obs.disconnect();
  }, [hasNext, isFetching, fetchNext]);
  return ref;
}

export const Route = createLazyFileRoute("/_auth/admin/activity")({
  component: ActivityPage,
});

type Tab = "play" | "audit" | "stats";

function formatDate(d?: string): string {
  if (!d) return "-";
  return new Date(d).toLocaleString();
}

function PlayHistoryTab({ watcher }: { watcher: Watcher }) {
  const { t } = useTranslation();
  const PAGE_SIZE = 25;
  // The watcher belongs in the key, not just the params: without it a switch
  // from one profile to another would serve the previous profile's cached rows.
  const { data, isLoading, isError, error, refetch, fetchNextPage, hasNextPage, isFetchingNextPage } = useInfiniteQuery({
    queryKey: ["play-history-infinite", watcher.userId ?? "", watcher.profileId ?? ""],
    initialPageParam: "" as string,
    queryFn: ({ pageParam, signal }) =>
      activityServiceGetPlayHistory(
        {
          "pagination.pageSize": PAGE_SIZE,
          ...(pageParam ? { "pagination.cursor": pageParam } : {}),
          ...(watcher.userId ? { userId: watcher.userId } : {}),
          ...(watcher.profileId ? { profileId: watcher.profileId } : {}),
        },
        signal,
      ),
    getNextPageParam: (last) => last.pagination?.nextCursor || undefined,
  });
  const entries = data?.pages.flatMap((p) => p.entries ?? []) ?? [];
  const total = data?.pages[0]?.pagination?.totalItems ?? 0;
  const sentinelRef = useInfiniteSentinel(hasNextPage, isFetchingNextPage, fetchNextPage);

  if (isLoading) {
    return (
      <div className="flex justify-center py-16">
        <Spinner size="lg" />
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <div
        className="overflow-x-auto rounded-[var(--radius-lg)] border border-[var(--border)]"
      >
        <table className="w-full text-sm table-cards">
          <thead>
            <tr className="bg-[var(--surface)] border-b border-[var(--border)]">
              {[t("admin.activity.playHeaders.title"), t("admin.activity.playHeaders.who"), t("admin.activity.playHeaders.device"), t("admin.activity.playHeaders.duration"), t("admin.activity.playHeaders.progress"), t("admin.activity.playHeaders.started")].map((h) => (
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
            {isError && entries.length === 0 ? (
              // A failed page after some rows are already shown must not wipe
              // them: only an empty-and-failed table is an error state.
              <tr>
                <td colSpan={6}>
                  <FetchErrorState error={error} onRetry={() => void refetch()} />
                </td>
              </tr>
            ) : entries.length === 0 ? (
              <tr>
                <td colSpan={6}>
                  <EmptyState icon={<Film size={48} strokeWidth={1.5} />} title={t("admin.activity.noPlayHistory")} />
                </td>
              </tr>
            ) : (
              entries.map((e) => {
                const pos = parseInt(e.positionMs ?? "0");
                const dur = parseInt(e.durationMs ?? "1");
                const pct = dur > 0 ? Math.round((pos / dur) * 100) : 0;
                return (
                  <tr key={e.id} className="border-b border-[var(--border)]">
                    <td data-label={t("admin.activity.playHeaders.title")} className="px-4 py-3 text-[var(--text)]">
                      {e.mediaTitle ?? "-"}
                    </td>
                    <td data-label={t("admin.activity.playHeaders.who")} className="px-4 py-3">
                      <span className="text-[var(--text)]">{e.profileName || t("admin.activity.unknownProfile")}</span>
                      {e.username && (
                        <span className="block text-xs text-[var(--text-muted)]">{e.username}</span>
                      )}
                    </td>
                    <td data-label={t("admin.activity.playHeaders.device")} className="px-4 py-3 text-xs text-[var(--text-muted)]">
                      {e.deviceName ?? e.clientName ?? "-"}
                    </td>
                    <td data-label={t("admin.activity.playHeaders.duration")} className="px-4 py-3 text-[var(--text-muted)]">
                      {formatDurationMs(e.durationMs) || "0min"}
                    </td>
                    <td data-label={t("admin.activity.playHeaders.progress")} className="px-4 py-3 min-w-[100px]">
                      <div className="flex items-center gap-2">
                        <div
                          className="h-1.5 flex-1 rounded-full bg-[var(--border)]"
                        >
                          <div
                            className="h-1.5 rounded-full bg-[var(--accent)]"
                            style={{ width: `${pct}%` }}
                          />
                        </div>
                        <span className="text-xs w-8 text-right text-[var(--text-muted)]">
                          {pct}%
                        </span>
                      </div>
                    </td>
                    <td data-label={t("admin.activity.playHeaders.started")} className="px-4 py-3 text-xs text-[var(--text-muted)]">
                      {formatDate(e.startedAt)}
                    </td>
                  </tr>
                );
              })
            )}
          </tbody>
        </table>
      </div>
      <div className="text-sm text-[var(--text-muted)]">
        {t("admin.activity.totalEntries", { count: total })}
      </div>
      <div ref={sentinelRef} className="h-1" />
      {isFetchingNextPage && (
        <div className="flex justify-center py-4">
          <Spinner size="md" />
        </div>
      )}
    </div>
  );
}

function AuditTab({ watcher }: { watcher: Watcher }) {
  const PAGE_SIZE = 25;
  const { data, isLoading, isError, error, refetch, fetchNextPage, hasNextPage, isFetchingNextPage } = useInfiniteQuery({
    queryKey: ["audit-log-infinite", watcher.userId ?? "", watcher.profileId ?? ""],
    initialPageParam: 0,
    queryFn: ({ pageParam, signal }) => adminSessionServiceGetAuditLog(
      {
        limit: PAGE_SIZE,
        offset: pageParam,
        ...(watcher.userId ? { userId: watcher.userId } : {}),
        ...(watcher.profileId ? { profileId: watcher.profileId } : {}),
      },
      signal,
    ),
    getNextPageParam: (last, allPages) => {
      const fetched = allPages.reduce((n, p) => n + (p.entries?.length ?? 0), 0);
      return fetched < (last.total ?? 0) ? fetched : undefined;
    },
  });
  const entries = data?.pages.flatMap((p) => p.entries ?? []) ?? [];
  const total = data?.pages[0]?.total ?? 0;
  const sentinelRef = useInfiniteSentinel(hasNextPage, isFetchingNextPage, fetchNextPage);

  if (isLoading) {
    return (
      <div className="flex justify-center py-16">
        <Spinner size="lg" />
      </div>
    );
  }

  // AuditLogTable is presentational and shows "no entries" for an empty list,
  // so the failure has to be told apart here, before it is handed down.
  if (isError && entries.length === 0) {
    return <FetchErrorState error={error} onRetry={() => void refetch()} />;
  }

  return (
    <div className="flex flex-col gap-2">
      <AuditLogTable entries={entries} total={total} />
      <div ref={sentinelRef} className="h-1" />
      {isFetchingNextPage && (
        <div className="flex justify-center py-4">
          <Spinner size="md" />
        </div>
      )}
    </div>
  );
}

type BgTask = { name: string; state: string; detail?: string; processed: number; lastActiveAt?: string };

// BackgroundTasksCard shows what the background workers are doing right now
// (keyframe/level backfill, complexity classify, intro detection) so an operator
// understands why the CPU isn't idle. Polls every 5s for a live-ish view.
function BackgroundTasksCard() {
  const { t } = useTranslation();
  const { data } = useQuery<BgTask[]>({
    queryKey: ["/api/v1/admin/background-tasks"],
    queryFn: () => eveyFetch<BgTask[]>({ url: "/api/v1/admin/background-tasks", method: "GET" }),
    refetchInterval: 5000,
  });
  if (!data || data.length === 0) return null;
  return (
    <Card>
      <h3 className="font-semibold mb-4 text-[var(--text)]">
        {t("admin.activity.backgroundTasks", "Background tasks")}
      </h3>
      <div className="flex flex-col gap-2">
        {data.map((task) => {
          const running = task.state === "running";
          return (
            <div key={task.name} className="flex items-center justify-between text-sm py-1 border-b border-[var(--border)] last:border-b-0">
              <div className="flex min-w-0 items-center gap-2">
                <span className={`inline-block h-2 w-2 shrink-0 rounded-full ${running ? "animate-pulse bg-[var(--success,#16a34a)]" : "bg-[var(--border)]"}`} />
                <span className="text-[var(--text)]">{t(`admin.activity.bgTask.${task.name}`, task.name)}</span>
                {running && task.detail && (
                  <span className="truncate text-xs text-[var(--text-muted)]">{task.detail}</span>
                )}
              </div>
              <div className="flex shrink-0 gap-4 text-xs text-[var(--text-muted)]">
                <span>{running ? t("admin.activity.bgRunning", "running") : t("admin.activity.bgIdle", "idle")}</span>
                <span>{t("admin.activity.bgProcessed", "{{n}} done", { n: task.processed })}</span>
              </div>
            </div>
          );
        })}
      </div>
    </Card>
  );
}

function StatsTab() {
  const { t } = useTranslation();
  const { data, isLoading } = useActivityServiceGetServerStats();

  if (isLoading) {
    return (
      <div className="flex justify-center py-16">
        <Spinner size="lg" />
      </div>
    );
  }

  const stats = data;

  return (
    <div className="flex flex-col gap-6">
      <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-4">
        {[
          [t("admin.activity.stats.totalPlays"), stats?.totalPlays ?? 0],
          [t("admin.activity.stats.totalPlayTime"), formatDurationMs(stats?.totalPlayTimeMs) || "0min"],
          [t("admin.activity.stats.totalUsers"), stats?.totalUsers ?? 0],
          [t("admin.activity.stats.totalMedia"), stats?.totalMediaItems ?? 0],
          [t("admin.activity.stats.bandwidth"), formatBytes(stats?.totalBandwidthBytes)],
          [t("admin.activity.stats.storage"), formatBytes(stats?.totalStorageBytes)],
        ].map(([label, value]) => (
          <Card key={String(label)} className="flex flex-col gap-1">
            <span className="text-xs font-medium uppercase tracking-wide text-[var(--text-muted)]">
              {label}
            </span>
            <span className="text-2xl font-bold text-[var(--text)]">
              {String(value)}
            </span>
          </Card>
        ))}
      </div>

      <BackgroundTasksCard />

      {stats?.popularMedia && stats.popularMedia.length > 0 && (
        <Card>
          <h3 className="font-semibold mb-4 text-[var(--text)]">
            {t("admin.activity.popularMedia")}
          </h3>
          <div className="flex flex-col gap-2">
            {stats.popularMedia.map((m) => (
              <div
                key={m.mediaItemId}
                className="flex items-center justify-between text-sm py-1 border-b border-[var(--border)]"
              >
                <span className="text-[var(--text)]">{m.title ?? "-"}</span>
                <div className="flex gap-4 text-xs text-[var(--text-muted)]">
                  <span>{t("admin.activity.plays", { count: m.playCount })}</span>
                  <span>{t("admin.activity.viewers", { count: m.uniqueViewers })}</span>
                </div>
              </div>
            ))}
          </div>
        </Card>
      )}

      {stats?.activeUsers && stats.activeUsers.length > 0 && (
        <Card>
          <h3 className="font-semibold mb-4 text-[var(--text)]">
            {t("admin.activity.mostActiveUsers")}
          </h3>
          <div className="flex flex-col gap-2">
            {stats.activeUsers.map((u) => (
              <div
                key={u.userId}
                className="flex items-center justify-between text-sm py-1 border-b border-[var(--border)]"
              >
                <span className="text-[var(--text)]">{u.username}</span>
                <div className="flex gap-4 text-xs text-[var(--text-muted)]">
                  <span>{t("admin.activity.plays", { count: u.playCount })}</span>
                  <span>{formatDurationMs(u.totalWatchTimeMs) || "0min"}</span>
                </div>
              </div>
            ))}
          </div>
        </Card>
      )}
    </div>
  );
}

function downloadJSON(data: unknown, filename: string) {
  const blob = new Blob([JSON.stringify(data, null, 2)], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

function ActivityPage() {
  const { t } = useTranslation();
  // The watcher lives in the URL: the users panel deep-links straight to one
  // profile's history, and a reload has to land on the same filter. The tab is
  // ordinary local state seeded from the link - nothing deep-links to a tab.
  const search = Route.useSearch();
  const navigate = useNavigate();
  const [tab, setTab] = useState<Tab>(search.tab ?? "play");
  const watcher: Watcher = { userId: search.userId, profileId: search.profileId };
  const setWatcher = (w: Watcher) =>
    void navigate({ to: "/admin/activity", search: { ...search, userId: w.userId, profileId: w.profileId } });
  const [exporting, setExporting] = useState(false);

  const handleExport = useCallback(async () => {
    setExporting(true);
    try {
      const timestamp = new Date().toISOString().slice(0, 10);
      if (tab === "play") {
        // Both exports used to ask for one page of 10000 rows. Neither
        // endpoint has ever served that - the server reduces it to 500 - so
        // the file was the first 500 rows wearing the name of the whole log.
        const { rows, total, truncated } = await collectAllPages(async (pageIndex) => {
          const page = await activityServiceGetPlayHistory({
            "pagination.page": pageIndex + 1,
            "pagination.pageSize": EXPORT_PAGE_SIZE,
            ...(watcher.userId ? { userId: watcher.userId } : {}),
            ...(watcher.profileId ? { profileId: watcher.profileId } : {}),
          });
          return { rows: page.entries ?? [], total: page.pagination?.totalItems };
        });
        if (truncated) toast.error(t("admin.activity.exportTruncated", { max: EXPORT_MAX_ROWS, total }));
        downloadJSON({ entries: rows, total }, `play-history-${timestamp}.json`);
      } else if (tab === "audit") {
        const { rows, total, truncated } = await collectAllPages(async (pageIndex) => {
          const page = await adminSessionServiceGetAuditLog({
            limit: EXPORT_PAGE_SIZE,
            offset: pageIndex * EXPORT_PAGE_SIZE,
            ...(watcher.userId ? { userId: watcher.userId } : {}),
            ...(watcher.profileId ? { profileId: watcher.profileId } : {}),
          });
          return { rows: page.entries ?? [], total: page.total };
        });
        if (truncated) toast.error(t("admin.activity.exportTruncated", { max: EXPORT_MAX_ROWS, total }));
        downloadJSON({ entries: rows, total }, `audit-log-${timestamp}.json`);
      } else {
        const data = await activityServiceGetServerStats();
        downloadJSON(data, `server-stats-${timestamp}.json`);
      }
    } catch (err) {
      // The spinner stopping with no file and no message is indistinguishable
      // from the browser blocking the download.
      console.warn("activity export failed", err);
      toast.error(t("common.error"));
    }
    setExporting(false);
  }, [tab, t, watcher.userId, watcher.profileId]);

  const tabs: Array<{ id: Tab; label: string }> = [
    { id: "play", label: t("admin.activity.tabs.playHistory") },
    { id: "audit", label: t("admin.audit.title") },
    { id: "stats", label: t("admin.activity.tabs.serverStats") },
  ];

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t("admin.activity.title")} />

      <div className="flex items-center gap-3">
        <div className="flex gap-1 p-1 rounded-[var(--radius-md)] bg-[var(--surface)] border border-[var(--border)]">
          {tabs.map(({ id, label }) => (
          <button
            key={id}
            onClick={() => setTab(id)}
            // Stable hook for the e2e spec - see photos-view-all.
            data-testid={`activity-tab-${id}`}
            className="px-4 py-1.5 text-sm rounded-[var(--radius-sm)] font-medium transition-colors-fast"
            style={{
              backgroundColor: tab === id ? "var(--accent)" : "transparent",
              color: tab === id ? "white" : "var(--text-muted)",
            }}
            aria-pressed={tab === id}
          >
            {label}
          </button>
        ))}
        </div>
        <button
          onClick={handleExport}
          disabled={exporting}
          className="flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-[var(--radius-sm)] text-[var(--text-muted)] hover:text-[var(--text)] hover:bg-[var(--surface)] transition-colors-fast disabled:opacity-50"
          title={t("common.export") || "Export JSON"}
          aria-label={t("common.export", "Export JSON")}
        >
          <Download size={14}  aria-hidden="true" />
          JSON
        </button>
        {tab !== "stats" && (
          <div className="ml-auto">
            <WatcherFilter value={watcher} onChange={setWatcher} />
          </div>
        )}
      </div>

      {tab === "play" && <PlayHistoryTab watcher={watcher} />}
      {tab === "audit" && <AuditTab watcher={watcher} />}
      {tab === "stats" && <StatsTab />}
    </div>
  );
}
