import { Avatar } from "@/components/ui/Avatar";
import { useTranslation } from "react-i18next";
import { useActivityServiceGetServerStats } from "@evey/api";

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

export function ActiveUsersSummary() {
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
            <Avatar initial={(u.username ?? "?").charAt(0).toUpperCase()} size={28} />
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
