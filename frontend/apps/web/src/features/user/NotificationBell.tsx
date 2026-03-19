import { useState, useRef, useEffect } from "react";
import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  useNotificationServiceGetUnreadCount,
  useNotificationServiceListNotifications,
  useNotificationServiceMarkAllRead,
} from "@evey/api";

function relativeTime(iso: string | undefined): string {
  if (!iso) return "";
  const diff = Date.now() - new Date(iso).getTime();
  const mins = Math.floor(diff / 60000);
  if (mins < 1) return "Just now";
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 7) return `${days}d ago`;
  return new Date(iso).toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

export function NotificationBell() {
  const { t } = useTranslation("common");
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  const { data: countData, isError: countError } = useNotificationServiceGetUnreadCount();
  const count = countData?.count ?? 0;

  const { data: listData, isError: listError } = useNotificationServiceListNotifications({ limit: 5 });
  const notifications = listData?.notifications ?? [];
  const queryClient = useQueryClient();
  const markAllRead = useNotificationServiceMarkAllRead({
    mutation: {
      onSuccess: () => {
        queryClient.invalidateQueries({ queryKey: ["/api/v1/notifications"] });
        queryClient.invalidateQueries({ queryKey: ["/api/v1/notifications/unread-count"] });
      },
    },
  });

  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [open]);

  return (
    <div ref={ref} className="relative">
      <button
        onClick={() => setOpen((v) => !v)}
        className="relative text-[var(--text-muted)] hover:text-[var(--text)] transition-colors"
        aria-label={t("notifications")}
      >
        <svg className="w-5 h-5" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9" />
        </svg>
        {countError && !open ? (
          <span className="absolute -top-1 -right-1 w-2 h-2 bg-[var(--text-muted)] rounded-full" aria-label={t("unableToLoadNotifications")} />
        ) : count > 0 ? (
          <span className="absolute -top-1.5 -right-1.5 w-4 h-4 text-[10px] rounded-full flex items-center justify-center font-medium" style={{ backgroundColor: "var(--color-danger)", color: "var(--filter-active-text)" }}>
            {count > 9 ? "9+" : count}
          </span>
        ) : null}
      </button>

      {open && (
        <div className="absolute right-0 top-full mt-2 w-80 bg-[var(--surface)] border border-[var(--border)] rounded-[var(--radius-lg)] shadow-xl z-50 overflow-hidden">
          <div className="px-4 py-3 border-b border-[var(--border)] flex items-center justify-between">
            <span className="text-sm font-semibold text-[var(--text)]">{t("notifications")}</span>
            {count > 0 && (
              <button
                onClick={() => markAllRead.mutate({ data: {} })}
                className="text-xs text-[var(--accent)] font-medium hover:underline"
              >
                {t("markAllRead")}
              </button>
            )}
          </div>

          {countError || listError ? (
            <div className="px-4 py-8 text-center text-sm text-[var(--text-muted)]">{t("unableToLoadNotifications")}</div>
          ) : notifications.length === 0 ? (
            <div className="px-4 py-8 text-center text-sm text-[var(--text-muted)]">
              {t("noNotifications")}
            </div>
          ) : (
            <div className="max-h-80 overflow-y-auto">
              {notifications.map((n) => (
                <div
                  key={n.id}
                  className={`px-4 py-3 border-b border-[var(--border)] last:border-0 ${
                    n.readAt ? "opacity-50" : ""
                  }`}
                >
                  <p className="text-sm font-medium text-[var(--text)] truncate">{n.title}</p>
                  <p className="text-xs text-[var(--text-muted)] line-clamp-1 mt-0.5">{n.message}</p>
                  <p className="text-xs text-[var(--text-muted)] mt-1">{relativeTime(n.createdAt)}</p>
                </div>
              ))}
            </div>
          )}

          <Link
            to="/profile/notifications"
            onClick={() => setOpen(false)}
            className="block px-4 py-2.5 text-center text-xs font-medium text-[var(--accent)] hover:bg-[var(--surface-raised)] border-t border-[var(--border)] transition-colors"
          >
            {t("viewAllNotifications")}
          </Link>
        </div>
      )}
    </div>
  );
}
