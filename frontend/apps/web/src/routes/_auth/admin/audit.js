import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import { useAdminServiceGetAuditLog } from "@evey/api";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { AuditLogTable } from "@/features/admin/AuditLogTable";
export const Route = createFileRoute("/_auth/admin/audit")({
    component: AuditPage,
});
const PAGE_SIZE = 50;
function AuditPage() {
    const [page, setPage] = useState(0);
    const [actionFilter, setActionFilter] = useState("");
    const [resourceFilter, setResourceFilter] = useState("");
    const [activeFilters, setActiveFilters] = useState({});
    const { data, isLoading } = useAdminServiceGetAuditLog({
        limit: PAGE_SIZE,
        offset: page * PAGE_SIZE,
        ...(activeFilters.action ? { action: activeFilters.action } : {}),
        ...(activeFilters.resourceType ? { resourceType: activeFilters.resourceType } : {}),
    });
    const entries = data?.entries ?? [];
    const total = data?.total ?? 0;
    function applyFilters() {
        setPage(0);
        setActiveFilters({
            action: actionFilter.trim() || undefined,
            resourceType: resourceFilter.trim() || undefined,
        });
    }
    function clearFilters() {
        setActionFilter("");
        setResourceFilter("");
        setActiveFilters({});
        setPage(0);
    }
    return (_jsxs("div", { className: "flex flex-col gap-6", children: [_jsx("h1", { className: "text-2xl font-bold", style: { color: "var(--text)" }, children: "Audit Log" }), _jsxs("div", { className: "flex flex-wrap items-end gap-3 p-4 rounded-[var(--radius-lg)]", style: { backgroundColor: "var(--surface)", border: "1px solid var(--border)" }, children: [_jsx(Input, { label: "Action", value: actionFilter, onChange: (e) => setActionFilter(e.target.value), placeholder: "e.g. login", className: "w-48" }), _jsx(Input, { label: "Resource Type", value: resourceFilter, onChange: (e) => setResourceFilter(e.target.value), placeholder: "e.g. library", className: "w-48" }), _jsxs("div", { className: "flex gap-2", children: [_jsx(Button, { onClick: applyFilters, children: "Apply" }), _jsx(Button, { variant: "secondary", onClick: clearFilters, children: "Clear" })] })] }), isLoading ? (_jsx("div", { className: "flex justify-center py-16", children: _jsx(Spinner, { size: "lg" }) })) : (_jsx(AuditLogTable, { entries: entries, total: total, page: page, pageSize: PAGE_SIZE, onNextPage: () => setPage((p) => p + 1), onPrevPage: () => setPage((p) => Math.max(0, p - 1)) }))] }));
}
