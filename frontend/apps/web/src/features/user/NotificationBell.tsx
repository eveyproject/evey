import { useState, useRef, useEffect } from "react";
import { Bell } from "lucide-react";
import { Link, useNavigate } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  useNotificationServiceGetUnreadCount,
  useNotificationServiceListNotifications,
  useNotificationServiceMarkAllRead,
} from "@evey/api";
import { translateNotificationTitle, useRelativeTime } from "./notificationUtils";

interface Notification {
  id?: string;
  title?: string;
  message?: string;
  mediaItemId?: string;
  createdAt?: string;
  readAt?: string;
}

function notificationUrl(n: Notification): string | undefined {
  if (n.mediaItemId) return `/media/${n.mediaItemId}`;
  return undefined;
}

export function NotificationBell() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const relativeTime = useRelativeTime();
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

  function handleNotificationClick(n: Notification) {
    const url = notificationUrl(n);
    if (url) {
      setOpen(false);
      void navigate({ to: url });
    }
  }

  return (
    <div ref={ref} className="relative">
      <button
        onClick={() => setOpen((v) => !v)}
        className="relative text-[var(--text-muted)] hover:text-[var(--text)] transition-colors-fast"
        aria-label={t("common.notifications.title")}
      >
        <Bell size={20} strokeWidth={1.5} />
        {countError && !open ? (
          <span className="absolute -top-1 -right-1 w-2 h-2 bg-[var(--text-muted)] rounded-full" aria-label={t("common.unableToLoadNotifications")} />
        ) : count > 0 ? (
          <span className="absolute -top-1.5 -right-1.5 w-4 h-4 text-[10px] rounded-full flex items-center justify-center font-medium bg-[var(--color-danger)] text-[var(--filter-active-text)]">
            {count > 9 ? "9+" : count}
          </span>
        ) : null}
      </button>

      {open && (
        <div className="absolute right-0 top-full mt-2 w-80 bg-[var(--surface)] border border-[var(--border)] rounded-[var(--radius-lg)] shadow-xl z-overlay overflow-hidden">
          <div className="px-4 py-3 border-b border-[var(--border)] flex items-center justify-between">
            <span className="text-sm font-semibold text-[var(--text)]">{t("common.notifications.title")}</span>
            {count > 0 && (
              <button
                onClick={() => markAllRead.mutate({ data: {} })}
                className="text-xs text-[var(--accent)] font-medium hover:underline"
              >
                {t("common.markAllRead")}
              </button>
            )}
          </div>

          {countError || listError ? (
            <div className="px-4 py-8 text-center text-sm text-[var(--text-muted)]">{t("common.unableToLoadNotifications")}</div>
          ) : notifications.length === 0 ? (
            <div className="px-4 py-8 text-center text-sm text-[var(--text-muted)]">
              {t("common.noNotifications")}
            </div>
          ) : (
            <div className="max-h-80 overflow-y-auto">
              {notifications.map((n) => {
                const url = notificationUrl(n);
                return (
                  <button
                    key={n.id}
                    type="button"
                    onClick={() => handleNotificationClick(n)}
                    className={`w-full text-left px-4 py-3 border-b border-[var(--border)] last:border-0 transition-colors-fast ${
                      url ? "hover:bg-[var(--surface-raised)] cursor-pointer" : "cursor-default"
                    } ${n.readAt ? "opacity-50" : ""}`}
                  >
                    <p className="text-sm font-medium text-[var(--text)] truncate">{translateNotificationTitle(t, n.title)}</p>
                    <p className="text-xs text-[var(--text-muted)] line-clamp-1 mt-0.5">{n.message}</p>
                    <p className="text-xs text-[var(--text-muted)] mt-1">{relativeTime(n.createdAt)}</p>
                  </button>
                );
              })}
            </div>
          )}

          <Link
            to="/profile/notifications"
            onClick={() => setOpen(false)}
            className="block px-4 py-2.5 text-center text-xs font-medium text-[var(--accent)] hover:bg-[var(--surface-raised)] border-t border-[var(--border)] transition-colors-fast"
          >
            {t("common.viewAllNotifications")}
          </Link>
        </div>
      )}
    </div>
  );
}
