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
import { toast } from "@evey/store";
import { translateNotificationTitle, useRelativeTime, notificationUrl } from "./notificationUtils";

interface Notification {
  id?: string;
  type?: string;
  title?: string;
  message?: string;
  mediaItemId?: string;
  createdAt?: string;
  readAt?: string;
}

// Notification types hidden from the user-facing bell. The average user
// doesn't care about plugin failures - admins still see them in the
// dedicated plugin security dashboard.
const HIDDEN_TYPES = new Set(["plugin_error"]);

// The dropdown shows this many and scrolls inside a fixed box - it never
// grows with the count. The fetch pulls a few extra so the hidden types
// filtered below cannot leave it short.
const DROPDOWN_LIMIT = 15;
const FETCH_LIMIT = 25;

export function NotificationBell() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const relativeTime = useRelativeTime();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  const { data: countData, isError: countError } = useNotificationServiceGetUnreadCount();
  const count = countData?.count ?? 0;

  // Pull a few extra so filtering doesn't leave the dropdown empty when
  // the head of the list happens to be plugin_error noise. Only fetched once
  // the dropdown opens: the bell is mounted on every page, and the unread
  // badge above needs just the count query.
  const { data: listData, isError: listError } = useNotificationServiceListNotifications(
    { limit: FETCH_LIMIT },
    { query: { enabled: open } },
  );
  const notifications = (listData?.notifications ?? [])
    .filter((n: Notification) => !n.type || !HIDDEN_TYPES.has(n.type))
    .slice(0, DROPDOWN_LIMIT);
  const queryClient = useQueryClient();
  const markAllRead = useNotificationServiceMarkAllRead({
    mutation: {
      onSuccess: () => {
        queryClient.invalidateQueries({ queryKey: ["/api/v1/notifications"] });
        queryClient.invalidateQueries({ queryKey: ["/api/v1/notifications/unread-count"] });
      },
      onError: (err: Error) => toast.error(err?.message || t("common.error")),
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

  useEffect(() => {
    if (!open) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
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
        className="relative text-[var(--text-muted)] hover:text-[var(--text)] transition-colors-fast min-h-11 min-w-11 inline-flex items-center justify-center"
        aria-label={t("common.notifications.title")}
        aria-haspopup="true"
        aria-expanded={open}
        data-testid="notification-bell"
      >
        <Bell size={20} strokeWidth={1.5}  aria-hidden="true" />
        {countError && !open ? (
          <span className="absolute -top-1 -right-1 w-2 h-2 bg-[var(--text-muted)] rounded-full" aria-label={t("common.unableToLoadNotifications")} />
        ) : count > 0 ? (
          <span className="absolute top-0 -right-0.5 w-4 h-4 text-xs rounded-full flex items-center justify-center font-medium bg-[var(--color-danger)] text-[var(--filter-active-text)]">
            {count > 9 ? "9+" : count}
          </span>
        ) : null}
      </button>

      {open && (
        <div data-testid="notification-dropdown" className="absolute right-0 top-full mt-2 w-80 bg-[var(--surface)] border border-[var(--border)] rounded-[var(--radius-lg)] shadow-xl z-overlay overflow-hidden">
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
            <div className="max-h-96 overflow-y-auto overscroll-contain" data-testid="notification-list">
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
                    <p className="text-sm font-medium text-[var(--text)] truncate">{translateNotificationTitle(t, n)}</p>
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
            data-testid="notification-view-all"
            className="block px-4 py-2.5 text-center text-xs font-medium text-[var(--accent)] hover:bg-[var(--surface-raised)] border-t border-[var(--border)] transition-colors-fast"
          >
            {t("common.viewAllNotifications")}
          </Link>
        </div>
      )}
    </div>
  );
}
