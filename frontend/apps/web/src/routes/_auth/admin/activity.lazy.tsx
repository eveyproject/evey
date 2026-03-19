import { useState } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import {
  useActivityServiceGetPlayHistory,
  useActivityServiceGetLoginHistory,
  useActivityServiceGetServerStats,
} from "@evey/api";
import { Spinner } from "@/components/ui/Spinner";
import { Card } from "@/components/ui/Card";
import { Badge } from "@/components/ui/Badge";
import { Pagination } from "@/components/ui/Pagination";
import { EmptyState } from "@/components/ui/EmptyState";

export const Route = createLazyFileRoute("/_auth/admin/activity")({
  component: ActivityPage,
});

type Tab = "play" | "logins" | "stats";

function formatBytes(bytes?: string): string {
  const n = parseInt(bytes ?? "0");
  if (n > 1e9) return `${(n / 1e9).toFixed(1)} GB`;
  if (n > 1e6) return `${(n / 1e6).toFixed(1)} MB`;
  if (n > 1e3) return `${(n / 1e3).toFixed(1)} KB`;
  return `${n} B`;
}

function formatMs(ms?: string): string {
  const n = parseInt(ms ?? "0");
  if (!n) return "0m";
  const h = Math.floor(n / 3600000);
  const m = Math.floor((n % 3600000) / 60000);
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

function formatDate(d?: string): string {
  if (!d) return "\u2014";
  return new Date(d).toLocaleString();
}

function PlayHistoryTab() {
  const { t } = useTranslation();
  const PAGE_SIZE = 25;
  const [page, setPage] = useState(1);
  const { data, isLoading } = useActivityServiceGetPlayHistory({
    "pagination.page": page,
    "pagination.pageSize": PAGE_SIZE,
  });
  const entries = data?.entries ?? [];
  const total = data?.pagination?.totalItems ?? 0;

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
        className="overflow-x-auto rounded-[var(--radius-lg)]"
        style={{ border: "1px solid var(--border)" }}
      >
        <table className="w-full text-sm">
          <thead>
            <tr style={{ backgroundColor: "var(--surface)", borderBottom: "1px solid var(--border)" }}>
              {[t("admin.activity.playHeaders.title"), t("admin.activity.playHeaders.device"), t("admin.activity.playHeaders.duration"), t("admin.activity.playHeaders.progress"), t("admin.activity.playHeaders.started")].map((h) => (
                <th
                  key={h}
                  className="px-4 py-2 text-left font-medium"
                  style={{ color: "var(--text-muted)" }}
                >
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {entries.length === 0 ? (
              <tr>
                <td colSpan={5}>
                  <EmptyState icon="media" title={t("admin.activity.noPlayHistory")} />
                </td>
              </tr>
            ) : (
              entries.map((e) => {
                const pos = parseInt(e.positionMs ?? "0");
                const dur = parseInt(e.durationMs ?? "1");
                const pct = dur > 0 ? Math.round((pos / dur) * 100) : 0;
                return (
                  <tr key={e.id} style={{ borderBottom: "1px solid var(--border)" }}>
                    <td className="px-4 py-3" style={{ color: "var(--text)" }}>
                      {e.mediaTitle ?? "\u2014"}
                    </td>
                    <td className="px-4 py-3 text-xs" style={{ color: "var(--text-muted)" }}>
                      {e.deviceName ?? e.clientName ?? "\u2014"}
                    </td>
                    <td className="px-4 py-3" style={{ color: "var(--text-muted)" }}>
                      {formatMs(e.durationMs)}
                    </td>
                    <td className="px-4 py-3 min-w-[100px]">
                      <div className="flex items-center gap-2">
                        <div
                          className="h-1.5 flex-1 rounded-full"
                          style={{ backgroundColor: "var(--border)" }}
                        >
                          <div
                            className="h-1.5 rounded-full"
                            style={{ width: `${pct}%`, backgroundColor: "var(--accent)" }}
                          />
                        </div>
                        <span className="text-xs w-8 text-right" style={{ color: "var(--text-muted)" }}>
                          {pct}%
                        </span>
                      </div>
                    </td>
                    <td className="px-4 py-3 text-xs" style={{ color: "var(--text-muted)" }}>
                      {formatDate(e.startedAt)}
                    </td>
                  </tr>
                );
              })
            )}
          </tbody>
        </table>
      </div>
      <div className="flex items-center justify-between text-sm" style={{ color: "var(--text-muted)" }}>
        <span>{t("admin.activity.totalEntries", { count: total })}</span>
        <Pagination
          page={page}
          totalPages={Math.max(1, Math.ceil(total / PAGE_SIZE))}
          onPageChange={setPage}
        />
      </div>
    </div>
  );
}

function LoginHistoryTab() {
  const { t } = useTranslation();
  const PAGE_SIZE = 25;
  const [page, setPage] = useState(1);
  const { data, isLoading } = useActivityServiceGetLoginHistory({
    "pagination.page": page,
    "pagination.pageSize": PAGE_SIZE,
  });
  const entries = data?.entries ?? [];
  const total = data?.pagination?.totalItems ?? 0;

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
        className="overflow-x-auto rounded-[var(--radius-lg)]"
        style={{ border: "1px solid var(--border)" }}
      >
        <table className="w-full text-sm">
          <thead>
            <tr style={{ backgroundColor: "var(--surface)", borderBottom: "1px solid var(--border)" }}>
              {[t("admin.activity.loginHeaders.user"), t("admin.activity.loginHeaders.ip"), t("admin.activity.loginHeaders.status"), t("admin.activity.loginHeaders.userAgent"), t("admin.activity.loginHeaders.timestamp")].map((h) => (
                <th
                  key={h}
                  className="px-4 py-2 text-left font-medium"
                  style={{ color: "var(--text-muted)" }}
                >
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {entries.length === 0 ? (
              <tr>
                <td colSpan={5}>
                  <EmptyState icon="history" title={t("admin.activity.noLoginHistory")} />
                </td>
              </tr>
            ) : (
              entries.map((e) => (
                <tr key={e.id} style={{ borderBottom: "1px solid var(--border)" }}>
                  <td className="px-4 py-3 font-medium" style={{ color: "var(--text)" }}>
                    {e.username ?? "\u2014"}
                  </td>
                  <td className="px-4 py-3 font-mono text-xs" style={{ color: "var(--text-muted)" }}>
                    {e.ipAddress ?? "\u2014"}
                  </td>
                  <td className="px-4 py-3">
                    {e.success ? (
                      <Badge variant="success">{t("admin.activity.ok")}</Badge>
                    ) : (
                      <Badge variant="error">{t("admin.activity.failed")}</Badge>
                    )}
                  </td>
                  <td className="px-4 py-3 text-xs max-w-xs truncate" style={{ color: "var(--text-muted)" }}>
                    {e.userAgent ?? "\u2014"}
                  </td>
                  <td className="px-4 py-3 text-xs" style={{ color: "var(--text-muted)" }}>
                    {formatDate(e.timestamp)}
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
      <div className="flex items-center justify-between text-sm" style={{ color: "var(--text-muted)" }}>
        <span>{t("admin.activity.totalEntries", { count: total })}</span>
        <Pagination
          page={page}
          totalPages={Math.max(1, Math.ceil(total / PAGE_SIZE))}
          onPageChange={setPage}
        />
      </div>
    </div>
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
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
        {[
          [t("admin.activity.stats.totalPlays"), stats?.totalPlays ?? 0],
          [t("admin.activity.stats.totalUsers"), stats?.totalUsers ?? 0],
          [t("admin.activity.stats.totalMedia"), stats?.totalMediaItems ?? 0],
          [t("admin.activity.stats.bandwidth"), formatBytes(stats?.totalBandwidthBytes)],
        ].map(([label, value]) => (
          <Card key={String(label)} className="flex flex-col gap-1">
            <span className="text-xs font-medium uppercase tracking-wide" style={{ color: "var(--text-muted)" }}>
              {label}
            </span>
            <span className="text-2xl font-bold" style={{ color: "var(--text)" }}>
              {String(value)}
            </span>
          </Card>
        ))}
      </div>

      {stats?.popularMedia && stats.popularMedia.length > 0 && (
        <Card>
          <h3 className="font-semibold mb-4" style={{ color: "var(--text)" }}>
            {t("admin.activity.popularMedia")}
          </h3>
          <div className="flex flex-col gap-2">
            {stats.popularMedia.map((m) => (
              <div
                key={m.mediaItemId}
                className="flex items-center justify-between text-sm py-1"
                style={{ borderBottom: "1px solid var(--border)" }}
              >
                <span style={{ color: "var(--text)" }}>{m.title ?? "\u2014"}</span>
                <span className="text-xs" style={{ color: "var(--text-muted)" }}>
                  {t("admin.activity.plays", { count: m.playCount })}
                </span>
              </div>
            ))}
          </div>
        </Card>
      )}

      {stats?.activeUsers && stats.activeUsers.length > 0 && (
        <Card>
          <h3 className="font-semibold mb-4" style={{ color: "var(--text)" }}>
            {t("admin.activity.mostActiveUsers")}
          </h3>
          <div className="flex flex-col gap-2">
            {stats.activeUsers.map((u) => (
              <div
                key={u.userId}
                className="flex items-center justify-between text-sm py-1"
                style={{ borderBottom: "1px solid var(--border)" }}
              >
                <span style={{ color: "var(--text)" }}>{u.username}</span>
                <div className="flex gap-4 text-xs" style={{ color: "var(--text-muted)" }}>
                  <span>{t("admin.activity.plays", { count: u.playCount })}</span>
                  <span>{formatMs(u.totalWatchTimeMs)}</span>
                </div>
              </div>
            ))}
          </div>
        </Card>
      )}
    </div>
  );
}

function ActivityPage() {
  const { t } = useTranslation();
  const [tab, setTab] = useState<Tab>("play");

  const tabs: Array<{ id: Tab; label: string }> = [
    { id: "play", label: t("admin.activity.tabs.playHistory") },
    { id: "logins", label: t("admin.activity.tabs.loginHistory") },
    { id: "stats", label: t("admin.activity.tabs.serverStats") },
  ];

  return (
    <div className="flex flex-col gap-6">
      <h1 className="text-2xl font-bold" style={{ color: "var(--text)" }}>
        {t("admin.activity.title")}
      </h1>

      <div className="flex gap-1 p-1 rounded-[var(--radius-md)] self-start"
        style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)" }}>
        {tabs.map(({ id, label }) => (
          <button
            key={id}
            onClick={() => setTab(id)}
            className="px-4 py-1.5 text-sm rounded-[var(--radius-sm)] font-medium transition-colors"
            style={{
              backgroundColor: tab === id ? "var(--accent)" : "transparent",
              color: tab === id ? "white" : "var(--text-muted)",
            }}
          >
            {label}
          </button>
        ))}
      </div>

      {tab === "play" && <PlayHistoryTab />}
      {tab === "logins" && <LoginHistoryTab />}
      {tab === "stats" && <StatsTab />}
    </div>
  );
}
