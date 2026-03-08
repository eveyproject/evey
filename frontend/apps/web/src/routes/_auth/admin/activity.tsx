import { useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import {
  useActivityServiceGetPlayHistory,
  useActivityServiceGetLoginHistory,
  useActivityServiceGetServerStats,
} from "@evey/api";
import { Spinner } from "@/components/ui/Spinner";
import { Card } from "@/components/ui/Card";
import { Badge } from "@/components/ui/Badge";

export const Route = createFileRoute("/_auth/admin/activity")({
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
  if (!d) return "—";
  return new Date(d).toLocaleString();
}

function PlayHistoryTab() {
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
              {["Title", "Device", "Duration", "Progress", "Started"].map((h) => (
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
                <td
                  colSpan={5}
                  className="px-4 py-8 text-center"
                  style={{ color: "var(--text-muted)" }}
                >
                  No play history.
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
                      {e.mediaTitle ?? "—"}
                    </td>
                    <td className="px-4 py-3 text-xs" style={{ color: "var(--text-muted)" }}>
                      {e.deviceName ?? e.clientName ?? "—"}
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
        <span>{total} total entries</span>
        <div className="flex gap-2">
          <button
            disabled={page === 1}
            onClick={() => setPage((p) => Math.max(1, p - 1))}
            className="px-3 py-1 rounded-[var(--radius-md)] disabled:opacity-40"
            style={{ border: "1px solid var(--border)", color: "var(--text)" }}
          >
            Prev
          </button>
          <button
            disabled={page * PAGE_SIZE >= total}
            onClick={() => setPage((p) => p + 1)}
            className="px-3 py-1 rounded-[var(--radius-md)] disabled:opacity-40"
            style={{ border: "1px solid var(--border)", color: "var(--text)" }}
          >
            Next
          </button>
        </div>
      </div>
    </div>
  );
}

function LoginHistoryTab() {
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
              {["User", "IP", "Status", "User Agent", "Timestamp"].map((h) => (
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
                <td
                  colSpan={5}
                  className="px-4 py-8 text-center"
                  style={{ color: "var(--text-muted)" }}
                >
                  No login history.
                </td>
              </tr>
            ) : (
              entries.map((e) => (
                <tr key={e.id} style={{ borderBottom: "1px solid var(--border)" }}>
                  <td className="px-4 py-3 font-medium" style={{ color: "var(--text)" }}>
                    {e.username ?? "—"}
                  </td>
                  <td className="px-4 py-3 font-mono text-xs" style={{ color: "var(--text-muted)" }}>
                    {e.ipAddress ?? "—"}
                  </td>
                  <td className="px-4 py-3">
                    {e.success ? (
                      <Badge variant="success">OK</Badge>
                    ) : (
                      <Badge variant="error">Failed</Badge>
                    )}
                  </td>
                  <td className="px-4 py-3 text-xs max-w-xs truncate" style={{ color: "var(--text-muted)" }}>
                    {e.userAgent ?? "—"}
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
        <span>{total} total entries</span>
        <div className="flex gap-2">
          <button
            disabled={page === 1}
            onClick={() => setPage((p) => Math.max(1, p - 1))}
            className="px-3 py-1 rounded-[var(--radius-md)] disabled:opacity-40"
            style={{ border: "1px solid var(--border)", color: "var(--text)" }}
          >
            Prev
          </button>
          <button
            disabled={page * PAGE_SIZE >= total}
            onClick={() => setPage((p) => p + 1)}
            className="px-3 py-1 rounded-[var(--radius-md)] disabled:opacity-40"
            style={{ border: "1px solid var(--border)", color: "var(--text)" }}
          >
            Next
          </button>
        </div>
      </div>
    </div>
  );
}

function StatsTab() {
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
          ["Total Plays", stats?.totalPlays ?? 0],
          ["Total Users", stats?.totalUsers ?? 0],
          ["Total Media", stats?.totalMediaItems ?? 0],
          ["Bandwidth", formatBytes(stats?.totalBandwidthBytes)],
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
            Popular Media
          </h3>
          <div className="flex flex-col gap-2">
            {stats.popularMedia.map((m) => (
              <div
                key={m.mediaItemId}
                className="flex items-center justify-between text-sm py-1"
                style={{ borderBottom: "1px solid var(--border)" }}
              >
                <span style={{ color: "var(--text)" }}>{m.title ?? "—"}</span>
                <span className="text-xs" style={{ color: "var(--text-muted)" }}>
                  {m.playCount} plays
                </span>
              </div>
            ))}
          </div>
        </Card>
      )}

      {stats?.activeUsers && stats.activeUsers.length > 0 && (
        <Card>
          <h3 className="font-semibold mb-4" style={{ color: "var(--text)" }}>
            Most Active Users
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
                  <span>{u.playCount} plays</span>
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
  const [tab, setTab] = useState<Tab>("play");

  const tabs: Array<{ id: Tab; label: string }> = [
    { id: "play", label: "Play History" },
    { id: "logins", label: "Login History" },
    { id: "stats", label: "Server Stats" },
  ];

  return (
    <div className="flex flex-col gap-6">
      <h1 className="text-2xl font-bold" style={{ color: "var(--text)" }}>
        Activity
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
