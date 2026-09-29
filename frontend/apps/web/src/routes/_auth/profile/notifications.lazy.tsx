import { Bell, Plus, Play, Info } from "lucide-react";
import { createLazyFileRoute, useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import {
  notificationServiceListNotifications,
  useNotificationServiceMarkRead,
  useNotificationServiceMarkAllRead,
  useNotificationServiceDeleteAllNotifications,
  getNotificationServiceListNotificationsQueryKey,
  getNotificationServiceGetUnreadCountQueryKey,
  apiErrorMessage,
} from "@evey/api";
import { useInfiniteQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { FetchErrorState } from "@/components/ui/FetchErrorState";
import { Modal } from "@/components/ui/Modal";
import { translateNotificationTitle, useRelativeTime, notificationUrl } from "@/features/user/notificationUtils";
import { useInfiniteSentinel } from "@/hooks/useInfiniteSentinel";
import { Spinner } from "@/components/ui/Spinner";
import { useDemoLock } from "@/hooks/useDemoMode";

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

// A row is the notification itself: clicking it goes where the bell's
// dropdown goes (the title, the request queue) and marks it read on the way.
// The page used to be display-only - pretty, and useless.
function NotificationRow({ notif }: { notif: Notification }) {
  const { t } = useTranslation();
  const { lock } = useDemoLock();
  const navigate = useNavigate();
  const relativeTime = useRelativeTime();
  const qc = useQueryClient();
  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: getNotificationServiceListNotificationsQueryKey() });
    void qc.invalidateQueries({ queryKey: getNotificationServiceGetUnreadCountQueryKey() });
  };

  const markRead = useNotificationServiceMarkRead({ mutation: { onSuccess: invalidate } });
  const isRead = !!notif.readAt;
  const url = notificationUrl(notif);

  function open() {
    if (!isRead && notif.id) markRead.mutate({ id: notif.id });
    if (url) void navigate({ to: url });
  }

  return (
    <div
      role={url ? "link" : undefined}
      tabIndex={url ? 0 : undefined}
      onClick={url ? open : undefined}
      onKeyDown={url ? (e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); open(); } } : undefined}
      data-testid={`notification-row-${notif.id ?? ""}`}
      className={`flex items-start gap-3 py-3 px-3 border-b border-[var(--border)] last:border-0 transition-colors-fast ${
        url ? "cursor-pointer hover:bg-[var(--surface)] focus:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]" : ""
      } ${isRead ? "opacity-50" : "border-l-2 border-l-[var(--accent)]"}`}
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
          {...lock}
          onClick={(e) => { e.stopPropagation(); markRead.mutate({ id: notif.id! }); }}
          disabled={markRead.isPending}
          className="shrink-0 text-xs text-[var(--accent)] hover:underline disabled:opacity-50 mt-1"
        >
          {t("common.notifications.markRead")}
        </button>
      )}
    </div>
  );
}

const NOTIFICATIONS_PAGE_SIZE = 50;

function NotificationsPage() {
  const { t } = useTranslation();
  const { lock } = useDemoLock();
  const qc = useQueryClient();
  const [showDeleteModal, setShowDeleteModal] = useState(false);

  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: getNotificationServiceListNotificationsQueryKey() });
    void qc.invalidateQueries({ queryKey: getNotificationServiceGetUnreadCountQueryKey() });
  };

  // Pages of PAGE_SIZE behind a scroll sentinel: this list grows for the
  // life of the profile, so neither a hard cap (200 used to be silently
  // dropped past) nor loading everything at once is right for it. The key
  // keeps the generated prefix so every existing invalidation still hits it.
  const { data, isLoading, isError, error, refetch, fetchNextPage, hasNextPage, isFetchingNextPage } = useInfiniteQuery({
    queryKey: [...getNotificationServiceListNotificationsQueryKey(), "pages"],
    initialPageParam: 0,
    queryFn: ({ pageParam, signal }) => notificationServiceListNotifications({ limit: NOTIFICATIONS_PAGE_SIZE, offset: pageParam }, signal),
    getNextPageParam: (last, _pages, lastOffset) =>
      (last.notifications?.length ?? 0) < NOTIFICATIONS_PAGE_SIZE ? undefined : lastOffset + NOTIFICATIONS_PAGE_SIZE,
  });
  const sentinelRef = useInfiniteSentinel(hasNextPage, isFetchingNextPage, fetchNextPage);
  const markAllRead = useNotificationServiceMarkAllRead({ mutation: { onSuccess: invalidate } });
  const deleteAll = useNotificationServiceDeleteAllNotifications({
    mutation: {
      onSuccess: () => {
        invalidate();
        setShowDeleteModal(false);
      },
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("common.error"))),
    },
  });

  const notifications = data?.pages.flatMap((p) => p.notifications ?? []) ?? [];
  const hasUnread = notifications.some((n) => !n.readAt);

  return (
    <div className="px-6 py-8 max-w-2xl flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-[var(--text)]">{t("common.notifications.title")}</h1>
        {notifications.length > 0 && (
          <div className="flex gap-2">
            {hasUnread && (
              <Button
                {...lock}
                variant="secondary"
                loading={markAllRead.isPending}
                onClick={() => markAllRead.mutate({ data: {} })}
              >
                {t("common.notifications.markAllRead")}
              </Button>
            )}
            <Button {...lock} variant="danger" onClick={() => setShowDeleteModal(true)}>
              {t("common.notifications.deleteAll")}
            </Button>
          </div>
        )}
      </div>

      {isLoading && <p className="text-sm text-[var(--text-muted)]">{t("common.loading")}</p>}

      {!isLoading && isError && (
        <FetchErrorState error={error} onRetry={() => void refetch()} />
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
      {!isLoading && !isError && notifications.length > 0 && (
        <div ref={sentinelRef} className="flex justify-center py-2" aria-hidden={!isFetchingNextPage}>
          {isFetchingNextPage && <Spinner size="sm" />}
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
          <Button {...lock} variant="danger" loading={deleteAll.isPending} onClick={() => deleteAll.mutate({})}>
            {t("common.notifications.deleteAll")}
          </Button>
        </div>
      </Modal>
    </div>
  );
}
