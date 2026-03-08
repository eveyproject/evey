import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { usePlaybackServiceGetWatchHistory, usePlaybackServiceMarkAsUnwatched, getPlaybackServiceGetWatchHistoryQueryKey, } from "@evey/api";
import { useQueryClient } from "@tanstack/react-query";
import { Badge } from "@/components/ui/Badge";
function formatDate(iso) {
    if (!iso)
        return "";
    return new Date(iso).toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" });
}
function ProgressBar({ percent }) {
    const clamped = Math.min(100, Math.max(0, percent));
    return (_jsx("div", { className: "h-1 bg-[var(--border)] rounded-full overflow-hidden", children: _jsx("div", { className: "h-full bg-[var(--accent)] transition-all", style: { width: `${clamped}%` } }) }));
}
function HistoryItem({ item }) {
    const qc = useQueryClient();
    const unwatch = usePlaybackServiceMarkAsUnwatched({
        mutation: {
            onSuccess: () => {
                void qc.invalidateQueries({ queryKey: getPlaybackServiceGetWatchHistoryQueryKey() });
            },
        },
    });
    const pct = item.percentComplete ?? 0;
    return (_jsxs("div", { className: "flex items-center justify-between gap-4 py-3 border-b border-[var(--border)] last:border-0", children: [_jsxs("div", { className: "flex-1 min-w-0", children: [_jsx("a", { href: `/library/${item.mediaItemId}`, className: "text-sm font-medium text-[var(--text)] hover:text-[var(--accent)] transition-colors truncate block", children: item.mediaItemId }), _jsx("p", { className: "text-xs text-[var(--text-muted)] mt-0.5", children: formatDate(item.lastPlayedAt) }), _jsx("div", { className: "mt-1.5 w-32", children: _jsx(ProgressBar, { percent: pct }) }), _jsxs("p", { className: "text-xs text-[var(--text-muted)] mt-0.5", children: [Math.round(pct), "%"] })] }), _jsxs("div", { className: "flex items-center gap-2 shrink-0", children: [item.played && _jsx(Badge, { variant: "success", children: "Watched" }), _jsx("button", { onClick: () => {
                            if (item.mediaItemId) {
                                unwatch.mutate({ mediaItemId: item.mediaItemId });
                            }
                        }, disabled: unwatch.isPending, className: "text-xs text-[var(--text-muted)] hover:text-[var(--text)] transition-colors disabled:opacity-50", children: "Mark unwatched" })] })] }));
}
export function WatchHistory({ limit }) {
    const { data, isLoading } = usePlaybackServiceGetWatchHistory(limit ? { limit } : undefined);
    const items = data?.items ?? [];
    if (isLoading) {
        return _jsx("p", { className: "text-sm text-[var(--text-muted)]", children: "Loading..." });
    }
    if (items.length === 0) {
        return _jsx("p", { className: "text-sm text-[var(--text-muted)]", children: "No watch history yet." });
    }
    return (_jsx("div", { className: "bg-[var(--surface-raised)] rounded-[var(--radius-md)] px-4", children: items.map((item) => (_jsx(HistoryItem, { item: item }, item.id ?? item.mediaItemId))) }));
}
