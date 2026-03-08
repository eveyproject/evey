import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { createFileRoute } from "@tanstack/react-router";
import { useNotificationServiceListNotifications, useNotificationServiceMarkRead, useNotificationServiceMarkAllRead, useNotificationServiceDeleteAllNotifications, getNotificationServiceListNotificationsQueryKey, getNotificationServiceGetUnreadCountQueryKey, } from "@evey/api";
import { useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
export const Route = createFileRoute("/_auth/profile/notifications")({
    component: NotificationsPage,
});
function formatDate(iso) {
    if (!iso)
        return "";
    return new Date(iso).toLocaleString(undefined, {
        year: "numeric",
        month: "short",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
    });
}
function NotificationRow({ notif }) {
    const qc = useQueryClient();
    const invalidate = () => {
        void qc.invalidateQueries({ queryKey: getNotificationServiceListNotificationsQueryKey() });
        void qc.invalidateQueries({ queryKey: getNotificationServiceGetUnreadCountQueryKey() });
    };
    const markRead = useNotificationServiceMarkRead({ mutation: { onSuccess: invalidate } });
    const isRead = !!notif.readAt;
    return (_jsxs("div", { className: `flex items-start gap-3 py-3 border-b border-[var(--border)] last:border-0 ${isRead ? "opacity-60" : ""}`, children: [_jsxs("div", { className: "flex-1 min-w-0", children: [_jsxs("div", { className: "flex items-center gap-2 mb-0.5", children: [_jsx("p", { className: "text-sm font-medium text-[var(--text)] truncate", children: notif.title }), !isRead && _jsx(Badge, { variant: "info", children: "New" })] }), _jsx("p", { className: "text-sm text-[var(--text-muted)]", children: notif.message }), _jsx("p", { className: "text-xs text-[var(--text-muted)] mt-1", children: formatDate(notif.createdAt) })] }), !isRead && notif.id && (_jsx("button", { onClick: () => markRead.mutate({ id: notif.id }), disabled: markRead.isPending, className: "shrink-0 text-xs text-[var(--accent)] hover:underline disabled:opacity-50", children: "Mark read" }))] }));
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
    return (_jsxs("div", { className: "px-6 py-8 max-w-2xl flex flex-col gap-6", children: [_jsxs("div", { className: "flex items-center justify-between", children: [_jsx("h1", { className: "text-2xl font-bold text-[var(--text)]", children: "Notifications" }), _jsxs("div", { className: "flex gap-2", children: [_jsx(Button, { variant: "secondary", loading: markAllRead.isPending, onClick: () => markAllRead.mutate({ data: {} }), children: "Mark all read" }), _jsx(Button, { variant: "danger", loading: deleteAll.isPending, onClick: () => deleteAll.mutate({}), children: "Clear all" })] })] }), isLoading && _jsx("p", { className: "text-sm text-[var(--text-muted)]", children: "Loading..." }), !isLoading && notifications.length === 0 && (_jsx("p", { className: "text-sm text-[var(--text-muted)]", children: "No notifications." })), !isLoading && notifications.length > 0 && (_jsx("div", { className: "bg-[var(--surface-raised)] rounded-[var(--radius-md)] px-4", children: notifications.map((n) => (_jsx(NotificationRow, { notif: n }, n.id))) }))] }));
}
