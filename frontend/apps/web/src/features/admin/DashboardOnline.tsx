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

// A short, human label for a raw user-agent (browser + OS best-effort).
function deviceLabel(ua: string): string {
  if (!ua) return "";
  const s = ua.toLowerCase();
  let browser = "";
  if (s.includes("edg/")) browser = "Edge";
  else if (s.includes("chrome/") && !s.includes("chromium")) browser = "Chrome";
  else if (s.includes("firefox/")) browser = "Firefox";
  else if (s.includes("safari/") && !s.includes("chrome")) browser = "Safari";
  else if (s.startsWith("curl")) browser = "curl";
  let os = "";
  if (s.includes("windows")) os = "Windows";
  else if (s.includes("android")) os = "Android";
  else if (s.includes("iphone") || s.includes("ipad")) os = "iOS";
  else if (s.includes("mac os")) os = "macOS";
  else if (s.includes("linux")) os = "Linux";
  return [browser, os].filter(Boolean).join(" · ") || ua.slice(0, 24);
}

// OnlineNow is a compact dashboard card of who is connected right now (live
// heartbeat presence), sized to sit beside the Now Playing panel.
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
    <div className="rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)] overflow-hidden flex flex-col">
      <div className="flex items-center justify-between px-4 py-3 border-b border-[var(--border)]">
        <h3 className="text-sm font-semibold text-[var(--text)] flex items-center gap-2">
          <Radio size={15} className="text-[var(--accent)]" />
          {t("admin.online.title")}
        </h3>
        <span className="flex items-center gap-1.5 text-xs font-medium text-[var(--text-muted)]">
          <span className="inline-flex h-2 w-2 rounded-full bg-[var(--success)] shadow-[0_0_6px] shadow-[var(--success)]/70 animate-pulse" />
          {t(total === 1 ? "admin.online.countOne" : "admin.online.countOther", { count: total })}
        </span>
      </div>

      {users.length === 0 ? (
        <p className="text-sm text-[var(--text-muted)] px-4 py-6 text-center">{t("admin.online.empty")}</p>
      ) : (
        <ul className="flex flex-col max-h-[22rem] overflow-y-auto divide-y divide-[var(--border)]">
          {users.map((u) => {
            const Icon = deviceIcon(u.userAgent ?? "");
            const dev = deviceLabel(u.userAgent ?? "");
            return (
              <li
                key={u.userId}
                className="flex items-center gap-3 px-4 py-2.5 hover:bg-[var(--surface-raised)] transition-colors"
              >
                <div className="flex items-center justify-center w-8 h-8 rounded-full shrink-0 bg-[var(--accent)] text-white text-xs font-bold">
                  {(u.username ?? "?").charAt(0).toUpperCase()}
                </div>
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-1.5">
                    <span className="font-medium text-sm text-[var(--text)] truncate">{u.username || u.userId}</span>
                    {u.isAdmin && (
                      <Shield size={11} className="shrink-0 text-[var(--accent)]" aria-label={t("admin.online.adminBadge")} />
                    )}
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-[var(--text-muted)] min-w-0">
                    <Icon size={12} className="shrink-0" />
                    <span className="truncate">
                      {dev}
                      {u.clientIp ? ` · ${u.clientIp}` : ""}
                    </span>
                  </div>
                </div>
                <span className="shrink-0 text-xs text-[var(--text-muted)] tabular-nums">{lastSeen(u)}</span>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
