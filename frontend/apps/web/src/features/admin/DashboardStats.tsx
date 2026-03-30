import { useState } from "react";
import { Film, Monitor, Users, Zap, Cpu, MemoryStick, HardDrive, Search, Folder } from "lucide-react";
import { Link } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useAdminServiceGetAuditLog,
  useLibraryServiceListLibraries,
  useLibraryServiceScanLibrary,
  useLibraryServiceListMediaItems,
  useActivityServiceGetServerStats,
} from "@evey/api";
import { toast } from "@evey/store";
import type { V1DashboardInfo } from "@evey/api";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { Spinner } from "@/components/ui/Spinner";

function getUsageColor(pct: number): string {
  if (pct > 85) return "var(--color-danger)";
  if (pct > 70) return "var(--color-warning)";
  return "var(--color-success)";
}

function formatBytes(bytes: string | undefined) {
  const n = parseInt(bytes ?? "0");
  if (n > 1e12) return `${(n / 1e12).toFixed(2)} TB`;
  if (n > 1e9) return `${(n / 1e9).toFixed(1)} GB`;
  if (n > 1e6) return `${(n / 1e6).toFixed(1)} MB`;
  return `${n} B`;
}

const MovieIcon = () => <Film size={20} strokeWidth={1.5} />;
const SeriesIcon = () => <Monitor size={20} strokeWidth={1.5} />;
const UsersIcon = () => <Users size={20} strokeWidth={1.5} />;
const StreamIcon = () => <Zap size={20} strokeWidth={1.5} />;
const CpuIcon = () => <Cpu size={20} strokeWidth={1.5} />;
const MemoryIcon = () => <MemoryStick size={20} strokeWidth={1.5} />;
const StorageIcon = () => <HardDrive size={20} strokeWidth={1.5} />;

interface StatCardProps {
  icon: React.ReactNode;
  label: string;
  value: string | number;
  valueColor?: string;
}

function StatCard({ icon, label, value, valueColor }: StatCardProps) {
  return (
    <div
      className="flex items-center gap-3 p-4 rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]"
    >
      <div
        className="flex items-center justify-center w-10 h-10 rounded-[var(--radius-md)] bg-[var(--surface-raised)] text-[var(--accent)]"
      >
        {icon}
      </div>
      <div className="flex flex-col min-w-0">
        <span className="text-2xl font-bold leading-tight" style={{ color: valueColor ?? "var(--text)" }}>
          {value}
        </span>
        <span className="text-xs font-medium uppercase tracking-wide text-[var(--text-muted)]">
          {label}
        </span>
      </div>
    </div>
  );
}

function QuickActions() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const { data: libData } = useLibraryServiceListLibraries();
  const libraries = libData?.libraries ?? [];
  const scanLib = useLibraryServiceScanLibrary({
    mutation: {
      onSuccess: () => {
        queryClient.invalidateQueries({ queryKey: ["/api/v1/media"] });
        toast.success(t("admin.dashboardStats.scanStarted"));
      },
      onError: (err: Error) => toast.error(err.message || t("admin.dashboardStats.scanFailed")),
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
    toast.success(t("admin.dashboardStats.allScansTriggered"));
  }

  return (
    <div>
      <h3 className="text-sm font-semibold mb-3 text-[var(--text)]">
        {t("admin.dashboardStats.quickActions")}
      </h3>
      <div className="flex gap-3 flex-wrap">
        <Button
          variant="secondary"
          loading={scanningAll}
          onClick={scanAll}
          disabled={libraries.length === 0}
        >
          <Search size={16} />
          {t("admin.dashboardStats.scanAllLibraries")}
        </Button>
        <Link to="/admin/libraries">
          <Button variant="secondary">
            <Folder size={16} />
            {t("admin.dashboardStats.manageLibraries")}
          </Button>
        </Link>
        <Link to="/admin/users">
          <Button variant="secondary">
            <Users size={16} />
            {t("admin.dashboardStats.manageUsers")}
          </Button>
        </Link>
      </div>
    </div>
  );
}

function RecentActivity() {
  const { t } = useTranslation();
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
        <h3 className="text-sm font-semibold text-[var(--text)]">
          {t("admin.dashboardStats.recentActivity")}
        </h3>
        <Link
          to="/admin/audit"
          className="text-xs font-medium hover:underline text-[var(--accent)]"
        >
          {t("admin.dashboardStats.viewAll")}
        </Link>
      </div>
      <div
        className="overflow-x-auto rounded-[var(--radius-lg)] border border-[var(--border)]"
      >
        <table className="w-full text-sm">
          <thead>
            <tr className="bg-[var(--surface)] border-b border-[var(--border)]">
              {[t("admin.audit.tableHeaders.time"), t("admin.audit.tableHeaders.user"), t("admin.audit.tableHeaders.action"), t("admin.audit.tableHeaders.resource"), t("admin.audit.tableHeaders.details")].map((h) => (
                <th key={h} className="px-3 py-2 text-left font-medium text-[var(--text-muted)]">
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {entries.map((entry) => (
              <tr
                key={entry.id}
                className="border-b border-[var(--border)] hover:bg-[var(--surface-raised)] transition-colors-fast"
              >
                <td className="px-3 py-2 whitespace-nowrap text-xs text-[var(--text-muted)]">
                  {entry.createdAt ? new Date(entry.createdAt).toLocaleString() : "--"}
                </td>
                <td className="px-3 py-2 whitespace-nowrap text-[var(--text)]">
                  {entry.username ?? "--"}
                </td>
                <td className="px-3 py-2 whitespace-nowrap">
                  <Badge variant="info">{entry.action ?? "--"}</Badge>
                </td>
                <td className="px-3 py-2 whitespace-nowrap text-xs text-[var(--text-muted)]">
                  {entry.resourceType ?? "--"}
                </td>
                <td className="px-3 py-2 max-w-xs truncate text-xs text-[var(--text-muted)]">
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
  const { t } = useTranslation();
  const { data } = useActivityServiceGetServerStats({ popularLimit: 5 });
  const popular = data?.popularMedia ?? [];
  if (popular.length === 0) return null;

  return (
    <div>
      <h3 className="text-sm font-semibold mb-3 text-[var(--text)]">
        {t("admin.dashboardStats.popularMedia")}
      </h3>
      <div className="flex flex-col gap-1">
        {popular.map((m, i) => (
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
              {t("admin.activity.plays", { count: m.playCount ?? 0 })}
            </span>
            {m.uniqueViewers != null && m.uniqueViewers > 0 && (
              <span className="text-xs text-[var(--text-muted)]">
                {t("admin.dashboardStats.viewers", { count: m.uniqueViewers })}
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
  const { t } = useTranslation();
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
      <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-4">
        <StatCard icon={<MovieIcon />} label={t("admin.dashboardStats.movies")} value={movieCount} />
        <StatCard icon={<SeriesIcon />} label={t("admin.dashboardStats.series")} value={seriesCount} />
        <StatCard icon={<UsersIcon />} label={t("admin.dashboardStats.users")} value={info.totalUsers ?? 0} />
        <StatCard icon={<StreamIcon />} label={t("admin.dashboardStats.activeStreams")} value={info.activeStreams ?? 0} />
        <StatCard icon={<CpuIcon />} label={t("admin.dashboardStats.cpu")} value={`${(info.cpuPercent ?? 0).toFixed(1)}%`} valueColor={getUsageColor(info.cpuPercent ?? 0)} />
        <StatCard icon={<MemoryIcon />} label={t("admin.dashboardStats.memory")} value={`${(info.memoryPercent ?? 0).toFixed(1)}%`} valueColor={getUsageColor(info.memoryPercent ?? 0)} />
        <StatCard
          icon={<StreamIcon />}
          label={t("admin.dashboardStats.transcodes")}
          value={info.activeTranscodes ?? 0}
        />
        <StatCard
          icon={<StorageIcon />}
          label={t("admin.dashboardStats.totalMedia")}
          value={info.totalMediaItems ?? 0}
        />
      </div>

      {info.storage && info.storage.length > 0 && (
        <div>
          <h3 className="text-sm font-semibold mb-3 text-[var(--text)]">
            {t("admin.dashboardStats.storage")}
          </h3>
          <div className="flex flex-col gap-2">
            {info.storage.map((s, i) => {
              const used = parseInt(s.usedBytes ?? "0");
              const total = parseInt(s.totalBytes ?? "1");
              const pct = total > 0 ? Math.round((used / total) * 100) : 0;
              return (
                <div
                  key={i}
                  className="p-3 rounded-[var(--radius-md)] bg-[var(--surface)] border border-[var(--border)]"
                >
                  <div className="flex justify-between text-sm mb-2">
                    <span className="truncate text-[var(--text)]">{s.path ?? "--"}</span>
                    <span style={{ color: getUsageColor(pct) }}>
                      {formatBytes(s.usedBytes)} / {formatBytes(s.totalBytes)} ({pct}%)
                    </span>
                  </div>
                  <div className="h-2 rounded-full bg-[var(--border)]">
                    <div
                      className="h-2 rounded-full transition-all"
                      style={{
                        width: `${pct}%`,
                        backgroundColor: pct > 85 ? "var(--color-danger)" : pct > 70 ? "var(--color-warning)" : "var(--accent)",
                      }}
                    />
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}

      <QuickActions />

      <RecentActivity />
      <PopularMedia />
    </div>
  );
}
