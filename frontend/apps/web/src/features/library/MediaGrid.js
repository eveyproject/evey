import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
function SkeletonCard() {
    return (_jsxs("div", { className: "w-36", style: { aspectRatio: "2/3" }, children: [_jsx("div", { className: "w-full h-full animate-pulse", style: {
                    backgroundColor: "var(--surface-raised)",
                    borderRadius: "var(--radius-md)",
                } }), _jsx("div", { className: "mt-2 h-4 w-3/4 animate-pulse", style: { backgroundColor: "var(--surface-raised)", borderRadius: "var(--radius-sm)" } })] }));
}
export function MediaGrid({ children, isLoading }) {
    if (isLoading) {
        return (_jsx("div", { className: "grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-4", children: Array.from({ length: 12 }).map((_, i) => (_jsx(SkeletonCard, {}, i))) }));
    }
    return (_jsx("div", { className: "grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-4", children: children }));
}
