import { createLazyFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import {
  useNotificationServiceListNotifications,
  useNotificationServiceMarkRead,
  useNotificationServiceMarkAllRead,
  useNotificationServiceDeleteAllNotifications,
  getNotificationServiceListNotificationsQueryKey,
  getNotificationServiceGetUnreadCountQueryKey,
} from "@evey/api";
import { useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { Modal } from "@/components/ui/Modal";

type Notification = {
  id?: string;
  profileId?: string;
  type?: string;
  title?: string;
  message?: string;
  mediaItemId?: string;
  level?: string;
  createdAt?: string;
  expiresAt?: string;
  readAt?: string;
};

export const Route = createLazyFileRoute("/_auth/profile/notifications")({
  component: NotificationsPage,
});

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

const TYPE_ICONS: Record<string, { path: string; color: string }> = {
  library_new: {
    path: "M12 6v6m0 0v6m0-6h6m-6 0H6",
    color: "var(--accent)",
  },
  playback: {
    path: "M5 3l14 9-14 9V3z",
    color: "var(--badge-info-text, #93c5fd)",
  },
  system: {
    path: "M13 16h-1v-4h-1m1-4h.01M12 2a10 10 0 100 20 10 10 0 000-20z",
    color: "var(--badge-warning-text, #fde047)",
  },
};

function NotifIcon({ type }: { type?: string }) {
  const cfg = TYPE_ICONS[type ?? ""] ?? TYPE_ICONS.system;
  return (
    <svg
      className="w-5 h-5 shrink-0"
      fill="none"
      stroke={cfg.color}
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      viewBox="0 0 24 24"
    >
      <path d={cfg.path} />
    </svg>
  );
}

function NotificationRow({ notif }: { notif: Notification }) {
  const qc = useQueryClient();
  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: getNotificationServiceListNotificationsQueryKey() });
    void qc.invalidateQueries({ queryKey: getNotificationServiceGetUnreadCountQueryKey() });
  };

  const markRead = useNotificationServiceMarkRead({ mutation: { onSuccess: invalidate } });
  const isRead = !!notif.readAt;

  return (
    <div
      className={`flex items-start gap-3 py-3 px-3 border-b border-[var(--border)] last:border-0 transition-colors ${
        isRead ? "opacity-50" : "border-l-2 border-l-[var(--accent)]"
      }`}
    >
      <div className="mt-0.5">
        <NotifIcon type={notif.type} />
      </div>
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 mb-0.5">
          <p className="text-sm font-medium text-[var(--text)] truncate">{notif.title}</p>
          {!isRead && (
            <span className="w-2 h-2 rounded-full bg-[var(--accent)] shrink-0" />
          )}
        </div>
        <p className="text-sm text-[var(--text-muted)] line-clamp-2">{notif.message}</p>
        <p className="text-xs text-[var(--text-muted)] mt-1">{relativeTime(notif.createdAt)}</p>
      </div>
      {!isRead && notif.id && (
        <button
          onClick={() => markRead.mutate({ id: notif.id! })}
          disabled={markRead.isPending}
          className="shrink-0 text-xs text-[var(--accent)] hover:underline disabled:opacity-50 mt-1"
        >
          Mark read
        </button>
      )}
    </div>
  );
}

function NotificationsPage() {
  const qc = useQueryClient();
  const [showDeleteModal, setShowDeleteModal] = useState(false);

  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: getNotificationServiceListNotificationsQueryKey() });
    void qc.invalidateQueries({ queryKey: getNotificationServiceGetUnreadCountQueryKey() });
  };

  const { data, isLoading } = useNotificationServiceListNotifications();
  const markAllRead = useNotificationServiceMarkAllRead({ mutation: { onSuccess: invalidate } });
  const deleteAll = useNotificationServiceDeleteAllNotifications({
    mutation: {
      onSuccess: () => {
        invalidate();
        setShowDeleteModal(false);
      },
    },
  });

  const notifications = data?.notifications ?? [];
  const hasUnread = notifications.some((n) => !n.readAt);

  return (
    <div className="px-6 py-8 max-w-2xl flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-[var(--text)]">Notifications</h1>
        {notifications.length > 0 && (
          <div className="flex gap-2">
            {hasUnread && (
              <Button
                variant="secondary"
                loading={markAllRead.isPending}
                onClick={() => markAllRead.mutate({ data: {} })}
              >
                Mark all read
              </Button>
            )}
            <Button variant="danger" onClick={() => setShowDeleteModal(true)}>
              Delete all
            </Button>
          </div>
        )}
      </div>

      {isLoading && <p className="text-sm text-[var(--text-muted)]">Loading...</p>}

      {!isLoading && notifications.length === 0 && (
        <EmptyState icon="notification" title="No notifications" description="You're all caught up." />
      )}

      {!isLoading && notifications.length > 0 && (
        <div className="bg-[var(--surface-raised)] rounded-[var(--radius-md)] overflow-hidden">
          {notifications.map((n) => (
            <NotificationRow key={n.id} notif={n} />
          ))}
        </div>
      )}

      <Modal open={showDeleteModal} onClose={() => setShowDeleteModal(false)} title="Delete all notifications">
        <p className="text-sm text-[var(--text-muted)] mb-6">
          This will permanently delete all your notifications. This action cannot be undone.
        </p>
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={() => setShowDeleteModal(false)}>
            Cancel
          </Button>
          <Button variant="danger" loading={deleteAll.isPending} onClick={() => deleteAll.mutate({})}>
            Delete all
          </Button>
        </div>
      </Modal>
    </div>
  );
}
