import { createFileRoute } from "@tanstack/react-router";
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
import { Badge } from "@/components/ui/Badge";

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

export const Route = createFileRoute("/_auth/profile/notifications")({
  component: NotificationsPage,
});

function formatDate(iso: string | undefined): string {
  if (!iso) return "";
  return new Date(iso).toLocaleString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
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
      className={`flex items-start gap-3 py-3 border-b border-[var(--border)] last:border-0 ${
        isRead ? "opacity-60" : ""
      }`}
    >
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 mb-0.5">
          <p className="text-sm font-medium text-[var(--text)] truncate">{notif.title}</p>
          {!isRead && <Badge variant="info">New</Badge>}
        </div>
        <p className="text-sm text-[var(--text-muted)]">{notif.message}</p>
        <p className="text-xs text-[var(--text-muted)] mt-1">{formatDate(notif.createdAt)}</p>
      </div>
      {!isRead && notif.id && (
        <button
          onClick={() => markRead.mutate({ id: notif.id! })}
          disabled={markRead.isPending}
          className="shrink-0 text-xs text-[var(--accent)] hover:underline disabled:opacity-50"
        >
          Mark read
        </button>
      )}
    </div>
  );
}

function NotificationsPage() {
  const qc = useQueryClient();
  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: getNotificationServiceListNotificationsQueryKey() });
    void qc.invalidateQueries({ queryKey: getNotificationServiceGetUnreadCountQueryKey() });
  };

  const { data, isLoading } = useNotificationServiceListNotifications();
  const markAllRead = useNotificationServiceMarkAllRead({ mutation: { onSuccess: invalidate } });
  const deleteAll = useNotificationServiceDeleteAllNotifications({ mutation: { onSuccess: invalidate } });

  const notifications = data?.notifications ?? [];

  return (
    <div className="px-6 py-8 max-w-2xl flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-[var(--text)]">Notifications</h1>
        <div className="flex gap-2">
          <Button
            variant="secondary"
            loading={markAllRead.isPending}
            onClick={() => markAllRead.mutate({ data: {} })}
          >
            Mark all read
          </Button>
          <Button
            variant="danger"
            loading={deleteAll.isPending}
            onClick={() => deleteAll.mutate({})}
          >
            Clear all
          </Button>
        </div>
      </div>

      {isLoading && <p className="text-sm text-[var(--text-muted)]">Loading...</p>}

      {!isLoading && notifications.length === 0 && (
        <p className="text-sm text-[var(--text-muted)]">No notifications.</p>
      )}

      {!isLoading && notifications.length > 0 && (
        <div className="bg-[var(--surface-raised)] rounded-[var(--radius-md)] px-4">
          {notifications.map((n) => (
            <NotificationRow key={n.id} notif={n} />
          ))}
        </div>
      )}
    </div>
  );
}
