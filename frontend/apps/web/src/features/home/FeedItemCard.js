import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
export function FeedItemCard({ item, showProgress }) {
    const posterUrl = item.mediaItemId
        ? `/api/v1/media/${item.mediaItemId}/artwork/poster`
        : null;
    const initial = item.title ? item.title[0].toUpperCase() : "?";
    return (_jsxs("a", { href: `/media/${item.mediaItemId}`, className: "group block flex-shrink-0 w-40", children: [_jsxs("div", { className: "relative w-full overflow-hidden", style: { aspectRatio: "2/3", borderRadius: "var(--radius-md)" }, children: [posterUrl ? (_jsx("img", { src: posterUrl, alt: item.title ?? "", className: "w-full h-full object-cover", onError: (e) => {
                            const target = e.currentTarget;
                            target.style.display = "none";
                            const fallback = target.nextElementSibling;
                            if (fallback)
                                fallback.style.display = "flex";
                        } })) : null, _jsx("div", { className: "absolute inset-0 items-center justify-center text-2xl font-bold", style: {
                            backgroundColor: "var(--surface-raised)",
                            color: "var(--text-muted)",
                            display: posterUrl ? "none" : "flex",
                        }, children: initial }), _jsx("div", { className: "absolute inset-0 flex items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity", style: { backgroundColor: "rgba(0,0,0,0.5)" }, children: _jsx("div", { className: "w-12 h-12 rounded-full flex items-center justify-center", style: { backgroundColor: "var(--accent)" }, children: _jsx("svg", { className: "w-5 h-5 text-white ml-1", fill: "currentColor", viewBox: "0 0 20 20", children: _jsx("path", { d: "M6.3 2.8L17 10 6.3 17.2V2.8z" }) }) }) }), showProgress && item.positionMs && (_jsx("div", { className: "absolute bottom-0 left-0 right-0 h-1", style: { backgroundColor: "var(--border)" }, children: _jsx("div", { className: "h-full", style: { backgroundColor: "var(--accent)", width: "40%" } }) }))] }), _jsxs("div", { className: "mt-2 px-0.5", children: [_jsx("p", { className: "text-sm font-medium truncate", style: { color: "var(--text)" }, children: item.title }), item.type && (_jsx("span", { className: "text-xs px-1.5 py-0.5 rounded", style: {
                            backgroundColor: "var(--surface-raised)",
                            color: "var(--text-muted)",
                            borderRadius: "var(--radius-sm)",
                        }, children: item.type }))] }), _jsx("style", { children: `
        .group:hover > div:first-child {
          transform: scale(1.05);
          transition: transform 0.2s ease;
        }
        .group > div:first-child {
          transition: transform 0.2s ease;
        }
      ` })] }));
}
