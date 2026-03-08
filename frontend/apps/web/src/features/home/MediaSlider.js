import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { useRef, useState } from "react";
function SkeletonCard() {
    return (_jsxs("div", { className: "flex-shrink-0 w-40", style: { aspectRatio: "2/3" }, children: [_jsx("div", { className: "w-full h-full animate-pulse", style: {
                    backgroundColor: "var(--surface-raised)",
                    borderRadius: "var(--radius-md)",
                } }), _jsx("div", { className: "mt-2 space-y-1 px-0.5", children: _jsx("div", { className: "h-3 w-4/5 animate-pulse rounded", style: { backgroundColor: "var(--surface-raised)" } }) })] }));
}
export function MediaSlider({ title, seeAllHref, children, isLoading, skeletonCount = 6, }) {
    const scrollRef = useRef(null);
    const [showLeft, setShowLeft] = useState(false);
    const [showRight, setShowRight] = useState(true);
    const scroll = (dir) => {
        const el = scrollRef.current;
        if (!el)
            return;
        const amount = el.clientWidth * 0.8;
        el.scrollBy({ left: dir === "left" ? -amount : amount, behavior: "smooth" });
    };
    const handleScroll = () => {
        const el = scrollRef.current;
        if (!el)
            return;
        setShowLeft(el.scrollLeft > 0);
        setShowRight(el.scrollLeft + el.clientWidth < el.scrollWidth - 4);
    };
    return (_jsxs("section", { className: "mb-8 group/slider", children: [_jsxs("div", { className: "flex items-center justify-between mb-3 px-1", children: [_jsx("h2", { className: "text-lg font-semibold", style: { color: "var(--text)" }, children: title }), seeAllHref && (_jsx("a", { href: seeAllHref, className: "text-sm transition-colors", style: { color: "var(--accent)" }, children: "See all \u2192" }))] }), _jsxs("div", { className: "relative", children: [showLeft && (_jsx("button", { onClick: () => scroll("left"), className: "absolute left-0 top-0 bottom-0 z-10 flex items-center justify-center w-10 opacity-0 group-hover/slider:opacity-100 transition-opacity", style: { backgroundColor: "rgba(0,0,0,0.5)" }, "aria-label": "Scroll left", children: _jsx("svg", { className: "w-5 h-5 text-white", fill: "none", stroke: "currentColor", viewBox: "0 0 24 24", children: _jsx("path", { strokeLinecap: "round", strokeLinejoin: "round", strokeWidth: 2, d: "M15 19l-7-7 7-7" }) }) })), _jsx("div", { ref: scrollRef, onScroll: handleScroll, className: "flex gap-3 overflow-x-auto pb-2", style: { scrollbarWidth: "none", scrollSnapType: "x mandatory" }, children: isLoading
                            ? Array.from({ length: skeletonCount }).map((_, i) => _jsx(SkeletonCard, {}, i))
                            : children }), showRight && (_jsx("button", { onClick: () => scroll("right"), className: "absolute right-0 top-0 bottom-0 z-10 flex items-center justify-center w-10 opacity-0 group-hover/slider:opacity-100 transition-opacity", style: { backgroundColor: "rgba(0,0,0,0.5)" }, "aria-label": "Scroll right", children: _jsx("svg", { className: "w-5 h-5 text-white", fill: "none", stroke: "currentColor", viewBox: "0 0 24 24", children: _jsx("path", { strokeLinecap: "round", strokeLinejoin: "round", strokeWidth: 2, d: "M9 5l7 7-7 7" }) }) }))] })] }));
}
