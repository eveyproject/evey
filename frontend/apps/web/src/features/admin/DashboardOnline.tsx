import { Radio, Shield, Monitor, Smartphone, Tv } from "lucide-react";
import { useTranslation } from "react-i18next";
import { usePresenceServiceListOnline } from "@evey/api";
import type { V1OnlineUser } from "@evey/api";

const REFRESH_INTERVAL = 5_000;

function deviceIcon(ua: string) {
  const s = ua.toLowerCase();
  if (/(tv|tizen|webos|crkey|appletv)/.test(s)) return Tv;
  if (/(mobile|android|iphone|ipad)/.test(s)) return Smartphone;
  return Monitor;
}

// OnlineNow is a compact dashboard card of who is connected right now (live
// heartbeat presence), sized to sit half-width next to the active-users summary.
export function OnlineNow() {
  const { t } = useTranslation();
  const { data } = usePresenceServiceListOnline({
    query: { refetchInterval: REFRESH_INTERVAL, refetchIntervalInBackground: false },
  });

  const users = data?.users ?? [];
  const total = data?.total ?? users.length;

  function lastSeen(u: V1OnlineUser): string {
    const idle = u.idleSeconds ?? 0;
    if (idle < 5) return t("admin.online.justNow");
    if (idle < 60) return t("admin.online.secondsAgo", { count: idle });
    return t("admin.online.minutesAgo", { count: Math.floor(idle / 60) });
  }

  return (
    <div className="p-4 rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]">
      <div className="flex items-center justify-between mb-3">
        <h3 className="text-sm font-semibold text-[var(--text)] flex items-center gap-2">
          <Radio size={15} className="text-[var(--accent)]" />
          {t("admin.online.title")}
        </h3>
        <span className="flex items-center gap-1.5 text-xs text-[var(--text-muted)]">
          <span className="inline-flex h-2 w-2 rounded-full bg-emerald-500 animate-pulse" />
          {t(total === 1 ? "admin.online.countOne" : "admin.online.countOther", { count: total })}
        </span>
      </div>

      {users.length === 0 ? (
        <p className="text-sm text-[var(--text-muted)] py-2">{t("admin.online.empty")}</p>
      ) : (
        <div className="flex flex-col gap-2 max-h-64 overflow-y-auto">
          {users.map((u) => {
            const Icon = deviceIcon(u.userAgent ?? "");
            return (
              <div key={u.userId} className="flex items-center gap-3 text-sm">
                <div className="flex items-center justify-center w-7 h-7 rounded-full shrink-0 bg-[var(--accent)] text-white text-xs font-bold">
                  {(u.username ?? "?").charAt(0).toUpperCase()}
                </div>
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-1.5">
                    <span className="font-medium text-[var(--text)] truncate">{u.username || u.userId}</span>
                    {u.isAdmin && (
                      <Shield size={11} className="shrink-0 text-[var(--accent)]" aria-label={t("admin.online.adminBadge")} />
                    )}
                  </div>
                  <div className="text-xs text-[var(--text-muted)] truncate">{u.clientIp || "-"}</div>
                </div>
                <div className="flex items-center gap-1.5 shrink-0 text-xs text-[var(--text-muted)]">
                  <Icon size={13} />
                  {lastSeen(u)}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
