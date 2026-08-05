import { Bell, Plus, Play, Info, TriangleAlert } from "lucide-react";
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
import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { Modal } from "@/components/ui/Modal";
import { translateNotificationTitle, useRelativeTime } from "@/features/user/notificationUtils";

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


const TYPE_ICON_COLORS: Record<string, string> = {
  library_new: "var(--accent)",
  playback: "var(--badge-info-text, #93c5fd)",
  system: "var(--badge-warning-text, #fde047)",
};

function NotifIcon({ type }: { type?: string }) {
  const color = TYPE_ICON_COLORS[type ?? ""] ?? TYPE_ICON_COLORS.system;
  if (type === "library_new") return <Plus size={20} className="shrink-0" style={{ color }} />;
  if (type === "playback") return <Play size={20} className="shrink-0" style={{ color }} />;
  return <Info size={20} className="shrink-0" style={{ color }} />;
}

function NotificationRow({ notif }: { notif: Notification }) {
  const { t } = useTranslation();
  const relativeTime = useRelativeTime();
  const qc = useQueryClient();
  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: getNotificationServiceListNotificationsQueryKey() });
    void qc.invalidateQueries({ queryKey: getNotificationServiceGetUnreadCountQueryKey() });
  };

  const markRead = useNotificationServiceMarkRead({ mutation: { onSuccess: invalidate } });
  const isRead = !!notif.readAt;

  return (
    <div
      className={`flex items-start gap-3 py-3 px-3 border-b border-[var(--border)] last:border-0 transition-colors-fast ${
        isRead ? "opacity-50" : "border-l-2 border-l-[var(--accent)]"
      }`}
    >
      <div className="mt-0.5">
        <NotifIcon type={notif.type} />
      </div>
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 mb-0.5">
          <p className="text-sm font-medium text-[var(--text)] truncate">{translateNotificationTitle(t, notif)}</p>
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
          {t("common.notifications.markRead")}
        </button>
      )}
    </div>
  );
}

function NotificationsPage() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [showDeleteModal, setShowDeleteModal] = useState(false);

  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: getNotificationServiceListNotificationsQueryKey() });
    void qc.invalidateQueries({ queryKey: getNotificationServiceGetUnreadCountQueryKey() });
  };

  const { data, isLoading, isError } = useNotificationServiceListNotifications();
  const markAllRead = useNotificationServiceMarkAllRead({ mutation: { onSuccess: invalidate } });
  const deleteAll = useNotificationServiceDeleteAllNotifications({
    mutation: {
      onSuccess: () => {
        invalidate();
        setShowDeleteModal(false);
      },
      onError: (err: Error) => toast.error(err.message || t("common.error")),
    },
  });

  const notifications = data?.notifications ?? [];
  const hasUnread = notifications.some((n) => !n.readAt);

  return (
    <div className="px-6 py-8 max-w-2xl flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-[var(--text)]">{t("common.notifications.title")}</h1>
        {notifications.length > 0 && (
          <div className="flex gap-2">
            {hasUnread && (
              <Button
                variant="secondary"
                loading={markAllRead.isPending}
                onClick={() => markAllRead.mutate({ data: {} })}
              >
                {t("common.notifications.markAllRead")}
              </Button>
            )}
            <Button variant="danger" onClick={() => setShowDeleteModal(true)}>
              {t("common.notifications.deleteAll")}
            </Button>
          </div>
        )}
      </div>

      {isLoading && <p className="text-sm text-[var(--text-muted)]">{t("common.loading")}</p>}

      {!isLoading && isError && (
        <EmptyState icon={<TriangleAlert size={48} strokeWidth={1.5} />} title={t("common.errorBoundary.title")} description={t("common.errorBoundary.description")} />
      )}

      {!isLoading && !isError && notifications.length === 0 && (
        <EmptyState icon={<Bell size={48} strokeWidth={1.5} />} title={t("common.notifications.noNotifications")} description={t("common.notifications.allCaughtUp")} />
      )}

      {!isLoading && !isError && notifications.length > 0 && (
        <div className="bg-[var(--surface-raised)] rounded-[var(--radius-md)] overflow-hidden">
          {notifications.map((n) => (
            <NotificationRow key={n.id} notif={n} />
          ))}
        </div>
      )}

      <Modal open={showDeleteModal} onClose={() => setShowDeleteModal(false)} title={t("common.notifications.deleteAllTitle")}>
        <p className="text-sm text-[var(--text-muted)] mb-6">
          {t("common.notifications.deleteAllConfirm")}
        </p>
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={() => setShowDeleteModal(false)}>
            {t("common.cancel")}
          </Button>
          <Button variant="danger" loading={deleteAll.isPending} onClick={() => deleteAll.mutate({})}>
            {t("common.notifications.deleteAll")}
          </Button>
        </div>
      </Modal>
    </div>
  );
}
