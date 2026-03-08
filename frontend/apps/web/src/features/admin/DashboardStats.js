import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { Card } from "@/components/ui/Card";
function formatBytes(bytes) {
    const n = parseInt(bytes ?? "0");
    if (n > 1e9)
        return `${(n / 1e9).toFixed(1)} GB`;
    if (n > 1e6)
        return `${(n / 1e6).toFixed(1)} MB`;
    return `${n} B`;
}
function StatCard({ label, value }) {
    return (_jsxs(Card, { className: "flex flex-col gap-1", children: [_jsx("span", { className: "text-xs font-medium uppercase tracking-wide", style: { color: "var(--text-muted)" }, children: label }), _jsx("span", { className: "text-2xl font-bold", style: { color: "var(--text)" }, children: value })] }));
}
export function DashboardStats({ info }) {
    return (_jsxs("div", { className: "flex flex-col gap-6", children: [_jsxs("div", { className: "grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-4", children: [_jsx(StatCard, { label: "Active Streams", value: info.activeStreams ?? 0 }), _jsx(StatCard, { label: "Active Transcodes", value: info.activeTranscodes ?? 0 }), _jsx(StatCard, { label: "CPU", value: `${(info.cpuPercent ?? 0).toFixed(1)}%` }), _jsx(StatCard, { label: "Memory", value: `${(info.memoryPercent ?? 0).toFixed(1)}%` }), _jsx(StatCard, { label: "Total Media", value: info.totalMediaItems ?? 0 }), _jsx(StatCard, { label: "Total Users", value: info.totalUsers ?? 0 })] }), info.storage && info.storage.length > 0 && (_jsxs("div", { children: [_jsx("h3", { className: "text-sm font-semibold mb-3", style: { color: "var(--text-muted)" }, children: "Storage" }), _jsx("div", { className: "flex flex-col gap-2", children: info.storage.map((s, i) => {
                            const used = parseInt(s.usedBytes ?? "0");
                            const total = parseInt(s.totalBytes ?? "1");
                            const pct = total > 0 ? Math.round((used / total) * 100) : 0;
                            return (_jsxs("div", { className: "p-3 rounded-[var(--radius-md)]", style: { backgroundColor: "var(--surface)", border: "1px solid var(--border)" }, children: [_jsxs("div", { className: "flex justify-between text-sm mb-2", children: [_jsx("span", { style: { color: "var(--text)" }, className: "truncate", children: s.path ?? "—" }), _jsxs("span", { style: { color: "var(--text-muted)" }, children: [formatBytes(s.usedBytes), " / ", formatBytes(s.totalBytes)] })] }), _jsx("div", { className: "h-1.5 rounded-full", style: { backgroundColor: "var(--border)" }, children: _jsx("div", { className: "h-1.5 rounded-full", style: { width: `${pct}%`, backgroundColor: "var(--accent)" } }) })] }, i));
                        }) })] }))] }));
}
