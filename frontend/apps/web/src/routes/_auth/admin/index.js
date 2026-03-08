import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { createFileRoute, Link, useRouterState } from "@tanstack/react-router";
import { useAdminServiceGetDashboard, useAdminServiceGetSystemInfo } from "@evey/api";
import { Spinner } from "@/components/ui/Spinner";
import { Card } from "@/components/ui/Card";
import { DashboardStats } from "@/features/admin/DashboardStats";
export const Route = createFileRoute("/_auth/admin/")({
    component: AdminDashboard,
});
const adminNavLinks = [
    { href: "/admin/libraries", label: "Libraries" },
    { href: "/admin/users", label: "Users" },
    { href: "/admin/plugins", label: "Plugins" },
    { href: "/admin/webhooks", label: "Webhooks" },
    { href: "/admin/audit", label: "Audit Log" },
    { href: "/admin/metadata", label: "Metadata" },
];
function AdminNav() {
    const { location } = useRouterState();
    const pathname = location.pathname;
    return (_jsx("div", { className: "flex flex-wrap gap-2", children: adminNavLinks.map(({ href, label }) => {
            const isActive = pathname === href || (href !== "/admin" && pathname.startsWith(href));
            return (_jsx(Link, { to: href, className: "px-4 py-2 text-sm font-medium rounded-[var(--radius-md)] transition-colors", style: {
                    backgroundColor: isActive ? "var(--accent)" : "var(--surface)",
                    border: `1px solid ${isActive ? "var(--accent)" : "var(--border)"}`,
                    color: isActive ? "white" : "var(--text)",
                }, children: label }, href));
        }) }));
}
function AdminDashboard() {
    const { data: dashData, isLoading: dashLoading } = useAdminServiceGetDashboard();
    const { data: sysData, isLoading: sysLoading } = useAdminServiceGetSystemInfo();
    return (_jsxs("div", { className: "flex flex-col gap-8", children: [_jsxs("div", { children: [_jsx("h1", { className: "text-2xl font-bold mb-2", style: { color: "var(--text)" }, children: "Admin" }), _jsx(AdminNav, {})] }), _jsxs("section", { children: [_jsx("h2", { className: "text-lg font-semibold mb-4", style: { color: "var(--text)" }, children: "Dashboard" }), dashLoading ? (_jsx("div", { className: "flex justify-center py-8", children: _jsx(Spinner, { size: "lg" }) })) : dashData ? (_jsx(DashboardStats, { info: dashData })) : null] }), _jsxs("section", { children: [_jsx("h2", { className: "text-lg font-semibold mb-4", style: { color: "var(--text)" }, children: "System Info" }), sysLoading ? (_jsx("div", { className: "flex justify-center py-8", children: _jsx(Spinner, { size: "md" }) })) : sysData ? (_jsx(Card, { children: _jsx("dl", { className: "grid grid-cols-2 sm:grid-cols-3 gap-x-8 gap-y-3 text-sm", children: [
                                ["Version", sysData.version],
                                ["OS", sysData.os],
                                ["Arch", sysData.arch],
                                ["Go Version", sysData.goVersion],
                                ["FFmpeg", sysData.ffmpegVersion],
                                ["Uptime", sysData.uptimeSeconds ? `${Math.floor(parseInt(sysData.uptimeSeconds) / 60)}m` : undefined],
                            ].map(([k, v]) => v ? (_jsxs("div", { children: [_jsx("dt", { className: "font-medium", style: { color: "var(--text-muted)" }, children: k }), _jsx("dd", { style: { color: "var(--text)" }, children: v })] }, k)) : null) }) })) : null] })] }));
}
