import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
function SkeletonCard() {
    return (_jsxs("div", { className: "flex-shrink-0 w-40 animate-pulse", style: { aspectRatio: "2/3" }, children: [_jsx("div", { className: "w-full h-full", style: {
                    backgroundColor: "var(--surface-raised)",
                    borderRadius: "var(--radius-md)",
                } }), _jsxs("div", { className: "mt-2 space-y-1 px-0.5", children: [_jsx("div", { className: "h-3 w-4/5 rounded", style: { backgroundColor: "var(--surface-raised)" } }), _jsx("div", { className: "h-3 w-2/5 rounded", style: { backgroundColor: "var(--surface-raised)" } })] })] }));
}
export function FeedSection({ title, children, isLoading }) {
    return (_jsxs("section", { className: "mb-8", children: [_jsx("h2", { className: "text-xl font-semibold mb-4", style: { color: "var(--text)" }, children: title }), _jsx("div", { className: "overflow-x-auto pb-2", style: { scrollbarWidth: "thin" }, children: _jsx("div", { className: "flex gap-4", style: { width: "max-content" }, children: isLoading
                        ? Array.from({ length: 5 }).map((_, i) => (_jsx(SkeletonCard, {}, i)))
                        : children }) })] }));
}
