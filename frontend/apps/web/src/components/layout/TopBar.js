import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { useNavigate } from "@tanstack/react-router";
import { useAuthStore } from "@evey/store";
import { useNotificationServiceGetUnreadCount, useUserServiceGetCurrentUser } from "@evey/api";
export function TopBar() {
    const navigate = useNavigate();
    const { logout } = useAuthStore();
    const { data: unreadData } = useNotificationServiceGetUnreadCount();
    const { data: currentUser } = useUserServiceGetCurrentUser();
    const unreadCount = unreadData?.count ?? 0;
    const userInitial = currentUser?.username?.charAt(0).toUpperCase() ?? "U";
    const handleLogout = () => {
        logout();
        void navigate({ to: "/login" });
    };
    return (_jsxs("header", { className: "h-14 flex items-center justify-between px-6 bg-[var(--surface)] border-b border-[var(--border)] shrink-0", children: [_jsx("div", { className: "flex-1", children: _jsx("input", { type: "search", placeholder: "Search...", readOnly: true, className: "w-64 px-3 py-1.5 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)] text-sm text-[var(--text)] placeholder:text-[var(--text-muted)] focus:outline-none focus:ring-1 focus:ring-[var(--accent)]" }) }), _jsxs("div", { className: "flex items-center gap-3", children: [_jsxs("a", { href: "/profile/notifications", className: "relative text-[var(--text-muted)] hover:text-[var(--text)] transition-colors text-lg", "aria-label": "Notifications", children: ["\uD83D\uDD14", unreadCount > 0 && (_jsx("span", { className: "absolute -top-1 -right-1 w-4 h-4 bg-red-500 text-white text-xs rounded-full flex items-center justify-center", children: unreadCount > 9 ? "9+" : unreadCount }))] }), _jsx("a", { href: "/profile", className: "w-8 h-8 rounded-full bg-[var(--accent)] flex items-center justify-center text-white text-sm font-medium", "aria-label": "Profile", children: userInitial }), _jsx("button", { onClick: handleLogout, className: "text-sm text-[var(--text-muted)] hover:text-[var(--text)] transition-colors", children: "Sign out" })] })] }));
}
