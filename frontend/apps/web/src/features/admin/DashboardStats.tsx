import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import {
  useAdminServiceGetAuditLog,
  useLibraryServiceListLibraries,
  useLibraryServiceScanLibrary,
  useLibraryServiceListMediaItems,
  useActivityServiceGetServerStats,
} from "@evey/api";
import { toast } from "@evey/store";
import type { V1DashboardInfo } from "@evey/api/src/generated/admin.schemas";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { Spinner } from "@/components/ui/Spinner";

function getUsageColor(pct: number): string {
  if (pct > 85) return "text-red-400";
  if (pct > 70) return "text-orange-400";
  return "text-green-400";
}

function formatBytes(bytes: string | undefined) {
  const n = parseInt(bytes ?? "0");
  if (n > 1e9) return `${(n / 1e9).toFixed(1)} GB`;
  if (n > 1e6) return `${(n / 1e6).toFixed(1)} MB`;
  return `${n} B`;
}

// SVG icons for stat cards
function MovieIcon() {
  return (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
      <rect x={2} y={4} width={20} height={16} rx={2} />
      <path strokeLinecap="round" d="M10 9l5 3-5 3V9z" />
    </svg>
  );
}
function SeriesIcon() {
  return (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
      <rect x={2} y={3} width={20} height={14} rx={2} />
      <path strokeLinecap="round" d="M8 21h8M12 17v4" />
    </svg>
  );
}
function UsersIcon() {
  return (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" d="M15 19.128a9.38 9.38 0 002.625.372 9.337 9.337 0 004.121-.952 4.125 4.125 0 00-7.533-2.493M15 19.128v-.003c0-1.113-.285-2.16-.786-3.07M15 19.128v.106A12.318 12.318 0 018.624 21c-2.331 0-4.512-.645-6.374-1.766l-.001-.109a6.375 6.375 0 0111.964-3.07M12 6.375a3.375 3.375 0 11-6.75 0 3.375 3.375 0 016.75 0zm8.25 2.25a2.625 2.625 0 11-5.25 0 2.625 2.625 0 015.25 0z" />
    </svg>
  );
}
function StreamIcon() {
  return (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" d="M3.75 13.5l10.5-11.25L12 10.5h8.25L9.75 21.75 12 13.5H3.75z" />
    </svg>
  );
}
function CpuIcon() {
  return (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
      <rect x={4} y={4} width={16} height={16} rx={2} />
      <rect x={8} y={8} width={8} height={8} rx={1} />
      <path strokeLinecap="round" d="M9 1v3m6-3v3M9 20v3m6-3v3M1 9h3m-3 6h3M20 9h3m-3 6h3" />
    </svg>
  );
}
function MemoryIcon() {
  return (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
      <rect x={3} y={6} width={18} height={12} rx={2} />
      <path strokeLinecap="round" d="M7 6V4m4 2V4m4 2V4M7 18v2m4-2v2m4-2v2" />
    </svg>
  );
}
function StorageIcon() {
  return (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" d="M20 7l-8-4-8 4m16 0l-8 4m8-4v10l-8 4m0-10L4 7m8 4v10M4 7v10l8 4" />
    </svg>
  );
}

interface StatCardProps {
  icon: React.ReactNode;
  label: string;
  value: string | number;
  valueClassName?: string;
}

function StatCard({ icon, label, value, valueClassName }: StatCardProps) {
  return (
    <div
      className="flex items-center gap-3 p-4 rounded-[var(--radius-lg)]"
      style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)" }}
    >
      <div
        className="flex items-center justify-center w-10 h-10 rounded-[var(--radius-md)]"
        style={{ backgroundColor: "var(--surface-raised)", color: "var(--accent)" }}
      >
        {icon}
      </div>
      <div className="flex flex-col min-w-0">
        <span className={`text-2xl font-bold leading-tight ${valueClassName ?? ""}`} style={valueClassName ? undefined : { color: "var(--text)" }}>
          {value}
        </span>
        <span className="text-xs font-medium uppercase tracking-wide" style={{ color: "var(--text-muted)" }}>
          {label}
        </span>
      </div>
    </div>
  );
}

function QuickActions() {
  const queryClient = useQueryClient();
  const { data: libData } = useLibraryServiceListLibraries();
  const libraries = libData?.libraries ?? [];
  const scanLib = useLibraryServiceScanLibrary({
    mutation: {
      onSuccess: () => {
        queryClient.invalidateQueries({ queryKey: ["/api/v1/media"] });
        toast.success("Scan started");
      },
      onError: (err: Error) => toast.error(err.message || "Scan failed"),
    },
  });
  const [scanningAll, setScanningAll] = useState(false);

  async function scanAll() {
    setScanningAll(true);
    for (const lib of libraries) {
      if (lib.id) {
        try { await scanLib.mutateAsync({ id: lib.id }); } catch { /* toast handles */ }
      }
    }
    setScanningAll(false);
    toast.success("All library scans triggered");
  }

  return (
    <div>
      <h3 className="text-sm font-semibold mb-3" style={{ color: "var(--text)" }}>
        Quick Actions
      </h3>
      <div className="flex gap-3 flex-wrap">
        <Button
          variant="secondary"
          loading={scanningAll}
          onClick={scanAll}
          disabled={libraries.length === 0}
        >
          <svg className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" d="M21 21l-5.197-5.197m0 0A7.5 7.5 0 105.196 5.196a7.5 7.5 0 0010.607 10.607z" />
          </svg>
          Scan All Libraries
        </Button>
        <Link to="/admin/libraries">
          <Button variant="secondary">
            <svg className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" d="M3.75 9.776c.112-.017.227-.026.344-.026h15.812c.117 0 .232.009.344.026m-16.5 0a2.25 2.25 0 00-1.883 2.542l.857 6a2.25 2.25 0 002.227 1.932H19.05a2.25 2.25 0 002.227-1.932l.857-6a2.25 2.25 0 00-1.883-2.542m-16.5 0V6A2.25 2.25 0 016 3.75h3.879a1.5 1.5 0 011.06.44l2.122 2.12a1.5 1.5 0 001.06.44H18A2.25 2.25 0 0120.25 9v.776" />
            </svg>
            Manage Libraries
          </Button>
        </Link>
        <Link to="/admin/users">
          <Button variant="secondary">
            <svg className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" d="M15 19.128a9.38 9.38 0 002.625.372 9.337 9.337 0 004.121-.952 4.125 4.125 0 00-7.533-2.493M15 19.128v-.003c0-1.113-.285-2.16-.786-3.07M15 19.128v.106A12.318 12.318 0 018.624 21c-2.331 0-4.512-.645-6.374-1.766l-.001-.109a6.375 6.375 0 0111.964-3.07M12 6.375a3.375 3.375 0 11-6.75 0 3.375 3.375 0 016.75 0zm8.25 2.25a2.625 2.625 0 11-5.25 0 2.625 2.625 0 015.25 0z" />
            </svg>
            Manage Users
          </Button>
        </Link>
      </div>
    </div>
  );
}

function RecentActivity() {
  const { data, isLoading } = useAdminServiceGetAuditLog({ limit: 8, offset: 0 });
  const entries = data?.entries ?? [];

  if (isLoading) {
    return (
      <div className="flex justify-center py-8">
        <Spinner size="md" />
      </div>
    );
  }

  if (entries.length === 0) return null;

  return (
    <div>
      <div className="flex items-center justify-between mb-3">
        <h3 className="text-sm font-semibold" style={{ color: "var(--text)" }}>
          Recent Activity
        </h3>
        <Link
          to="/admin/audit"
          className="text-xs font-medium hover:underline"
          style={{ color: "var(--accent)" }}
        >
          View all
        </Link>
      </div>
      <div
        className="overflow-x-auto rounded-[var(--radius-lg)]"
        style={{ border: "1px solid var(--border)" }}
      >
        <table className="w-full text-sm">
          <thead>
            <tr style={{ backgroundColor: "var(--surface)", borderBottom: "1px solid var(--border)" }}>
              {["Time", "User", "Action", "Resource", "Details"].map((h) => (
                <th key={h} className="px-3 py-2 text-left font-medium" style={{ color: "var(--text-muted)" }}>
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {entries.map((entry) => (
              <tr
                key={entry.id}
                style={{ borderBottom: "1px solid var(--border)" }}
                className="hover:bg-[var(--surface-raised)] transition-colors"
              >
                <td className="px-3 py-2 whitespace-nowrap text-xs" style={{ color: "var(--text-muted)" }}>
                  {entry.createdAt ? new Date(entry.createdAt).toLocaleString() : "--"}
                </td>
                <td className="px-3 py-2 whitespace-nowrap" style={{ color: "var(--text)" }}>
                  {entry.username ?? "--"}
                </td>
                <td className="px-3 py-2 whitespace-nowrap">
                  <Badge variant="info">{entry.action ?? "--"}</Badge>
                </td>
                <td className="px-3 py-2 whitespace-nowrap text-xs" style={{ color: "var(--text-muted)" }}>
                  {entry.resourceType ?? "--"}
                </td>
                <td className="px-3 py-2 max-w-xs truncate text-xs" style={{ color: "var(--text-muted)" }}>
                  {entry.details ?? "--"}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function PopularMedia() {
  const { data } = useActivityServiceGetServerStats({ popularLimit: 5 });
  const popular = data?.popularMedia ?? [];
  if (popular.length === 0) return null;

  return (
    <div>
      <h3 className="text-sm font-semibold mb-3" style={{ color: "var(--text)" }}>
        Popular Media
      </h3>
      <div className="flex flex-col gap-1">
        {popular.map((m, i) => (
          <Link
            key={m.mediaItemId ?? i}
            to="/media/$mediaId"
            params={{ mediaId: m.mediaItemId ?? "" }}
            className="flex items-center gap-3 px-3 py-2 rounded-[var(--radius-md)] hover:bg-[var(--surface-raised)] transition-colors"
          >
            <span className="text-sm font-mono w-6 text-center" style={{ color: "var(--accent)" }}>
              {i + 1}
            </span>
            <span className="text-sm flex-1 truncate" style={{ color: "var(--text)" }}>
              {m.title ?? "Unknown"}
            </span>
            <span className="text-xs" style={{ color: "var(--text-muted)" }}>
              {m.playCount ?? 0} plays
            </span>
            {m.uniqueViewers != null && m.uniqueViewers > 0 && (
              <span className="text-xs" style={{ color: "var(--text-muted)" }}>
                {m.uniqueViewers} viewers
              </span>
            )}
          </Link>
        ))}
      </div>
    </div>
  );
}

interface DashboardStatsProps {
  info: V1DashboardInfo;
}

export function DashboardStats({ info }: DashboardStatsProps) {
  const { data: moviesData } = useLibraryServiceListMediaItems({
    type: "MEDIA_ITEM_TYPE_MOVIE" as never,
    "pagination.page": 1,
    "pagination.pageSize": 1,
  });
  const { data: seriesData } = useLibraryServiceListMediaItems({
    type: "MEDIA_ITEM_TYPE_SERIES" as never,
    "pagination.page": 1,
    "pagination.pageSize": 1,
  });

  const movieCount = moviesData?.pagination?.totalItems ?? 0;
  const seriesCount = seriesData?.pagination?.totalItems ?? 0;

  return (
    <div className="flex flex-col gap-8">
      {/* Stat cards grid */}
      <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-4">
        <StatCard icon={<MovieIcon />} label="Movies" value={movieCount} />
        <StatCard icon={<SeriesIcon />} label="Series" value={seriesCount} />
        <StatCard icon={<UsersIcon />} label="Users" value={info.totalUsers ?? 0} />
        <StatCard icon={<StreamIcon />} label="Active Streams" value={info.activeStreams ?? 0} />
        <StatCard icon={<CpuIcon />} label="CPU" value={`${(info.cpuPercent ?? 0).toFixed(1)}%`} valueClassName={getUsageColor(info.cpuPercent ?? 0)} />
        <StatCard icon={<MemoryIcon />} label="Memory" value={`${(info.memoryPercent ?? 0).toFixed(1)}%`} valueClassName={getUsageColor(info.memoryPercent ?? 0)} />
        <StatCard
          icon={<StreamIcon />}
          label="Transcodes"
          value={info.activeTranscodes ?? 0}
        />
        <StatCard
          icon={<StorageIcon />}
          label="Total Media"
          value={info.totalMediaItems ?? 0}
        />
      </div>

      {/* Storage bars */}
      {info.storage && info.storage.length > 0 && (
        <div>
          <h3 className="text-sm font-semibold mb-3" style={{ color: "var(--text)" }}>
            Storage
          </h3>
          <div className="flex flex-col gap-2">
            {info.storage.map((s, i) => {
              const used = parseInt(s.usedBytes ?? "0");
              const total = parseInt(s.totalBytes ?? "1");
              const pct = total > 0 ? Math.round((used / total) * 100) : 0;
              return (
                <div
                  key={i}
                  className="p-3 rounded-[var(--radius-md)]"
                  style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)" }}
                >
                  <div className="flex justify-between text-sm mb-2">
                    <span style={{ color: "var(--text)" }} className="truncate">{s.path ?? "--"}</span>
                    <span className={getUsageColor(pct)}>
                      {formatBytes(s.usedBytes)} / {formatBytes(s.totalBytes)} ({pct}%)
                    </span>
                  </div>
                  <div className="h-2 rounded-full" style={{ backgroundColor: "var(--border)" }}>
                    <div
                      className="h-2 rounded-full transition-all"
                      style={{
                        width: `${pct}%`,
                        backgroundColor: pct > 85 ? "#ef4444" : pct > 70 ? "#f97316" : "var(--accent)",
                      }}
                    />
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}

      {/* Quick Actions */}
      <QuickActions />

      {/* Two-column layout: recent activity + popular media */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <RecentActivity />
        <PopularMedia />
      </div>
    </div>
  );
}
