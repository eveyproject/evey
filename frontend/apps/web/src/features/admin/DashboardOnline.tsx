import { Avatar } from "@/components/ui/Avatar";
import { Radio, Shield } from "lucide-react";
import { useTranslation } from "react-i18next";
import { usePresenceServiceListOnline } from "@evey/api";
import type { V1OnlineUser } from "@evey/api";
import { ICON } from "@/constants/iconSizes";
import { deviceIcon, deviceLabel } from "./onlineDevice";
import { FetchErrorState } from "@/components/ui/FetchErrorState";

const REFRESH_INTERVAL = 5_000;

// OnlineNow is a compact dashboard card of who is connected right now (live
// heartbeat presence), sized to sit beside the Now Playing panel.
export function OnlineNow() {
  const { t } = useTranslation();
  const { data, isError, error, refetch } = usePresenceServiceListOnline({
    query: { refetchInterval: REFRESH_INTERVAL, refetchIntervalInBackground: false },
  });

  // "Nobody connected" and "I could not ask" are the same picture on a card an
  // admin reads to spot a session that should not be there. Only claim the
  // failure when there is nothing to show: a poll that fails while a previous
  // answer is still on screen must keep showing that answer, stale but true.
  const failed = isError && !data;
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
          <Radio size={ICON.md} className="text-[var(--accent)]" />
          {t("admin.online.title")}
        </h3>
        <span className="flex items-center gap-1.5 text-xs font-medium text-[var(--text-muted)]">
          <span className="inline-flex h-2 w-2 rounded-full bg-[var(--success)] shadow-[0_0_6px] shadow-[var(--success)]/70 animate-pulse" />
          {failed
            ? "-"
            : t(total === 1 ? "admin.online.countOne" : "admin.online.countOther", { count: total })}
        </span>
      </div>

      {failed ? (
        <FetchErrorState error={error} onRetry={() => void refetch()} />
      ) : users.length === 0 ? (
        <p className="text-sm text-[var(--text-muted)] px-4 py-6 text-center">{t("admin.online.empty")}</p>
      ) : (
        <ul className="flex flex-col max-h-[22rem] overflow-y-auto divide-y divide-[var(--border)]">
          {users.map((u) => {
            const Icon = deviceIcon(u.userAgent ?? "");
            const dev = deviceLabel(u.userAgent ?? "");
            return (
              <li
                // One row per DEVICE since the tracker keys on (user, user
                // agent): the same account on web + mobile is two rows.
                key={`${u.userId}|${u.userAgent ?? ""}`}
                className="flex items-center gap-3 px-4 py-2.5 hover:bg-[var(--surface-raised)] transition-colors"
              >
                <Avatar initial={(u.username ?? "?").charAt(0).toUpperCase()} size={32} />
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-1.5">
                    <span className="font-medium text-sm text-[var(--text)] truncate">{u.username || u.userId}</span>
                    {u.isAdmin && (
                      <Shield size={ICON.xs} className="shrink-0 text-[var(--accent)]" aria-label={t("admin.online.adminBadge")} />
                    )}
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-[var(--text-muted)] min-w-0">
                    <Icon size={ICON.xs} className="shrink-0" />
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
