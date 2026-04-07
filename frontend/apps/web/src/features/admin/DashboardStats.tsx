import { useState, useRef, useEffect, useCallback } from "react";
import { Film, Monitor, Users, Zap, Cpu, MemoryStick, HardDrive, Search, Folder, Play, Pause, Square, MessageSquare, Send } from "lucide-react";
import { Link } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useAdminSessionServiceGetAuditLog,
  useAdminSessionServiceListActiveSessions,
  useAdminSessionServiceKillSession,
  useAdminSessionServiceSendSessionCommand,
  getAdminSessionServiceListActiveSessionsQueryKey,
  useLibraryServiceListLibraries,
  useLibraryServiceScanLibrary,
  useLibraryServiceListMediaItems,
  useActivityServiceGetServerStats,
  useActivityServiceGetAnalytics,
} from "@evey/api";
import type { V1ActiveSession } from "@evey/api";
import { toast } from "@evey/store";
import type { V1DashboardInfo } from "@evey/api";
import { BarChart, PieChart } from "react-gifted-charts";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { AdminTrickplayThumbnail } from "./AdminTrickplayThumbnail";
import { Spinner } from "@/components/ui/Spinner";
import { EmptyState } from "@/components/ui/EmptyState";

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

function formatMs(ms: number): string {
  if (!ms) return "0:00";
  const totalSec = Math.floor(ms / 1000);
  const h = Math.floor(totalSec / 3600);
  const m = Math.floor((totalSec % 3600) / 60);
  const s = totalSec % 60;
  if (h > 0) return `${h}:${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
  return `${m}:${String(s).padStart(2, "0")}`;
}

function usePlayMethodLabel() {
  const { t } = useTranslation();
  return (method: string): string => {
    switch (method) {
      case "direct_play": return t("admin.sessions.directPlay");
      case "direct_stream": return t("admin.sessions.directStream");
      case "transcode": return t("admin.sessions.transcode");
      default: return method;
    }
  };
}

function ProfileAvatar({ profileName, size = 28 }: { profileName?: string; size?: number }) {
  const initial = (profileName ?? "?").charAt(0).toUpperCase();
  return (
    <div
      className="rounded-full shrink-0 bg-[var(--accent)] text-white flex items-center justify-center overflow-hidden"
      style={{ width: size, height: size, fontSize: size * 0.4 }}
    >
      <span className="font-bold">{initial}</span>
    </div>
  );
}

function formatEpisodeLabel(session: V1ActiveSession): string | null {
  if (!session.seriesTitle) return null;
  const s = session.seasonNumber ?? 0;
  const e = session.episodeNumber ?? 0;
  if (s > 0 && e > 0) return `S${String(s).padStart(2, "0")}E${String(e).padStart(2, "0")}`;
  if (e > 0) return `E${String(e).padStart(2, "0")}`;
  return null;
}

function NowPlayingCard({ session }: { session: V1ActiveSession }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const killSession = useAdminSessionServiceKillSession({
    mutation: {
      onSuccess: () => {
        queryClient.invalidateQueries({ queryKey: getAdminSessionServiceListActiveSessionsQueryKey() });
        toast.success(t("admin.sessions.sessionStopped"));
      },
      onError: (err: Error) => toast.error(err.message || t("admin.sessions.failedStop")),
    },
  });
  const sendCommand = useAdminSessionServiceSendSessionCommand();
  const playMethodLabel = usePlayMethodLabel();
  const [showMsg, setShowMsg] = useState(false);
  const [msgText, setMsgText] = useState("");
  const progress = Math.min(100, Math.max(0, session.progress ?? 0));
  const positionMs = parseInt(session.positionMs ?? "0");
  const durationMs = parseInt(session.durationMs ?? "0");
  const remaining = durationMs > 0 ? formatMs(Math.max(0, durationMs - positionMs)) : null;
  const epLabel = formatEpisodeLabel(session);
  const bandwidthBps = parseInt(session.bandwidthBps ?? "0");

  return (
    <div className="rounded-[var(--radius-lg)] overflow-hidden bg-[var(--surface)] border border-[var(--border)]">
      <AdminTrickplayThumbnail
        mediaId={session.mediaItemId ?? ""}
        positionMs={positionMs}
        mediaType={session.mediaType}
        fullWidth
      />
      <div className="px-3 pt-1.5 pb-1">
        <div className="h-1.5 rounded-full bg-[var(--border)]">
          <div
            className="h-1.5 rounded-full transition-all duration-500 bg-[var(--accent)]"
            style={{ width: `${progress}%` }}
          />
        </div>
        <div className="flex justify-between text-xs mt-1 text-[var(--text-muted)]">
          <span>{formatMs(positionMs)}</span>
          <span>{remaining ? `-${remaining}` : "-"}</span>
        </div>
      </div>

      <div className="px-3 flex flex-col gap-1.5 py-1.5">
        <div className="flex items-center justify-center gap-3">
          <button
            className="p-2 rounded-full hover:bg-[var(--border)] transition-colors text-[var(--text)]"
            onClick={() => {
              const cmd = session.isPaused ? "play" : "pause";
              sendCommand.mutate({ sessionId: session.sessionId ?? "", data: { command: cmd } });
            }}
            title={session.isPaused ? t("common.play") : t("common.pause")}
          >
            {session.isPaused ? <Play size={18} fill="currentColor" /> : <Pause size={18} />}
          </button>
          <button
            className="p-2 rounded-full hover:bg-[var(--danger-bg,rgba(239,68,68,0.15))] transition-colors text-[var(--danger,#ef4444)]"
            onClick={() => {
              if (window.confirm(t("admin.sessions.stopSession") + " ?")) {
                killSession.mutate({ sessionId: session.sessionId ?? "" });
              }
            }}
            title={t("admin.sessions.stopSession")}
          >
            <Square size={16} fill="currentColor" />
          </button>
          <button
            className="p-2 rounded-full hover:bg-[var(--border)] transition-colors text-[var(--text-muted)] hover:text-[var(--text)]"
            onClick={() => setShowMsg(!showMsg)}
            title={t("admin.sessions.sendMessage")}
          >
            <MessageSquare size={16} />
          </button>
        </div>
        {showMsg && (
          <form
            className="flex items-center gap-1.5"
            onSubmit={(e) => {
              e.preventDefault();
              if (!msgText.trim()) return;
              sendCommand.mutate({
                sessionId: session.sessionId ?? "",
                data: { command: `message:${msgText.trim()}` },
              });
              setMsgText("");
              setShowMsg(false);
              toast.success(t("admin.sessions.messageSent"));
            }}
          >
            <input
              autoFocus
              className="flex-1 text-xs px-2 py-1 rounded bg-[var(--input-bg,var(--surface))] border border-[var(--border)] text-[var(--text)] placeholder:text-[var(--text-muted)]"
              placeholder={t("admin.sessions.messagePlaceholder")}
              value={msgText}
              onChange={(e) => setMsgText(e.target.value)}
            />
            <button
              type="submit"
              className="p-1.5 rounded-full hover:bg-[var(--border)] transition-colors text-[var(--accent)]"
              disabled={!msgText.trim()}
            >
              <Send size={14} />
            </button>
          </form>
        )}
      </div>

      <div className="px-3 pb-3 flex flex-col gap-2">
        <div>
          {session.seriesTitle && (
            <p className="text-xs font-medium text-[var(--accent)] truncate">{session.seriesTitle}</p>
          )}
          <p className="text-sm font-semibold truncate text-[var(--text)]">
            {epLabel && <span className="text-[var(--text-muted)] mr-1">{epLabel}</span>}
            {session.mediaTitle}
          </p>
        </div>

        <div className="flex items-center gap-2">
          <ProfileAvatar profileName={session.profileName} size={24} />
          <span className="text-xs truncate text-[var(--text-muted)]">
            {session.profileName}
            {session.deviceName ? ` - ${session.deviceName}` : ""}
          </span>
          <div className="ml-auto flex items-center gap-2 shrink-0">
            <Badge variant={session.playMethod === "transcode" ? "warning" : "success"}>
              {playMethodLabel(session.playMethod ?? "")}
            </Badge>
            {session.resolution && (
              <span className="text-xs text-[var(--text-muted)]">{session.resolution}</span>
            )}
          </div>
        </div>

        {bandwidthBps > 0 && (
          <div className="text-xs text-[var(--text-muted)]">
            {formatBandwidth(bandwidthBps)}
          </div>
        )}
      </div>
    </div>
  );
}

function formatBandwidth(bps: number): string {
  if (bps > 1e6) return `${(bps / 1e6).toFixed(1)} Mbps`;
  if (bps > 1e3) return `${(bps / 1e3).toFixed(0)} Kbps`;
  return `${bps} bps`;
}

function NowPlaying() {
  const { t } = useTranslation();
  const { data, isLoading } = useAdminSessionServiceListActiveSessions({
    query: { refetchInterval: 3_000, refetchIntervalInBackground: false },
  });
  const sessions = data?.sessions ?? [];

  return (
    <div>
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-2">
          <Play size={18} strokeWidth={2} className="text-[var(--accent)]" />
          <h3 className="text-sm font-semibold text-[var(--text)]">
            {t("admin.dashboardStats.nowPlaying")}
          </h3>
          {sessions.length > 0 && (
            <span className="text-xs font-medium px-2 py-0.5 rounded-full bg-[var(--accent)] text-white">
              {sessions.length}
            </span>
          )}
        </div>
        <Link
          to="/admin/sessions"
          className="text-xs font-medium hover:underline text-[var(--accent)]"
        >
          {t("admin.dashboardStats.viewAll")}
        </Link>
      </div>
      {isLoading ? (
        <div className="flex justify-center py-6">
          <Spinner size="md" />
        </div>
      ) : sessions.length === 0 ? (
        <EmptyState
          icon={<Film size={40} strokeWidth={1.5} />}
          title={t("admin.dashboardStats.noActivePlayback")}
        />
      ) : (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-3">
          {sessions.map((s) => (
            <NowPlayingCard key={s.sessionId} session={s} />
          ))}
        </div>
      )}
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
  const { data, isLoading } = useAdminSessionServiceGetAuditLog({ limit: 8, offset: 0 });
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
            {entries.map((entry) => {
              const isFailed = entry.details?.includes('"failed"') ?? false;
              return (
                <tr
                  key={entry.id}
                  className={`border-b border-[var(--border)] hover:bg-[var(--surface-raised)] transition-colors-fast ${isFailed ? "bg-[color-mix(in_srgb,var(--color-danger)_10%,transparent)]" : ""}`}
                >
                  <td className="px-3 py-2 whitespace-nowrap text-xs text-[var(--text-muted)]">
                    {entry.createdAt ? new Date(entry.createdAt).toLocaleString() : "-"}
                  </td>
                  <td className="px-3 py-2 whitespace-nowrap text-[var(--text)]">
                    {entry.username || (isFailed ? entry.details?.match(/"username":"([^"]+)"/)?.[1] : null) || "-"}
                  </td>
                  <td className="px-3 py-2 whitespace-nowrap">
                    <Badge variant={isFailed ? "error" : "info"}>{entry.action ?? "-"}</Badge>
                  </td>
                  <td className="px-3 py-2 whitespace-nowrap text-xs text-[var(--text-muted)]">
                    {entry.resourceType ?? "-"}
                  </td>
                  <td className="px-3 py-2 text-xs text-[var(--text-muted)]">
                    {isFailed ? t("admin.dashboardStats.loginFailed") : "-"}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function useCssVar(name: string, fallback = "#888") {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim() || fallback;
}

const PIE_COLORS = ["#6366f1", "#f59e0b", "#10b981", "#ef4444", "#8b5cf6", "#ec4899", "#14b8a6", "#f97316"];

function PlayActivityChart() {
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

function StorageDonut({ storage }: { storage: { path?: string; totalBytes?: string; usedBytes?: string; freeBytes?: string }[] }) {
  const { t } = useTranslation();

  if (!storage || storage.length === 0) return null;

  const usedColor = PIE_COLORS[0];
  const freeColor = PIE_COLORS[2];

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

function formatWatchTime(ms: string | undefined): string {
  const total = parseInt(ms ?? "0");
  const h = Math.floor(total / 3600000);
  const m = Math.floor((total % 3600000) / 60000);
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

function formatRelativeTime(dateStr: string | undefined): string {
  if (!dateStr) return "-";
  const diff = Date.now() - new Date(dateStr).getTime();
  const mins = Math.floor(diff / 60000);
  if (mins < 1) return "now";
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

function ActiveUsersSummary() {
  const { t } = useTranslation();
  const { data } = useActivityServiceGetServerStats({ activeUsersLimit: 10 });
  const users = data?.activeUsers ?? [];

  if (users.length === 0) return null;

  return (
    <div className="p-4 rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]">
      <h3 className="text-sm font-semibold mb-3 text-[var(--text)]">
        {t("admin.dashboardStats.activeUsers")}
      </h3>
      <div className="flex flex-col gap-2">
        {users.map((u) => (
          <div key={u.userId} className="flex items-center gap-3 text-sm">
            <div className="flex items-center justify-center w-7 h-7 rounded-full shrink-0 bg-[var(--accent)] text-white text-xs font-bold">
              {(u.username ?? "?").charAt(0).toUpperCase()}
            </div>
            <span className="flex-1 truncate text-[var(--text)]">{u.username}</span>
            <span className="text-xs text-[var(--text-muted)]">
              {t("admin.activity.plays", { count: u.playCount ?? 0 })}
            </span>
            <span className="text-xs text-[var(--text-muted)]">{formatWatchTime(u.totalWatchTimeMs)}</span>
            <span className="text-xs text-[var(--text-muted)]">{formatRelativeTime(u.lastActiveAt)}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

function PeakHours() {
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
      <NowPlaying />

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

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
        <PlayActivityChart />
        <StorageDonut storage={info.storage ?? []} />
        <PeakHours />
      </div>

      <ActiveUsersSummary />

      <QuickActions />

      <PopularMedia />
      <RecentActivity />
    </div>
  );
}
