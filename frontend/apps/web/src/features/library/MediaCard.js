import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
export function MediaCard({ item, href, posterSrc }) {
    const poster = posterSrc ?? (item.id ? `/api/v1/media/${item.id}/artwork/poster` : null);
    const initial = item.title ? item.title[0].toUpperCase() : "?";
    const link = href ?? (item.id ? `/media/${item.id}` : "#");
    return (_jsxs("a", { href: link, className: "group block flex-shrink-0 w-40", children: [_jsxs("div", { className: "relative w-full overflow-hidden", style: {
                    aspectRatio: "2/3",
                    borderRadius: "var(--radius-md)",
                    transition: "transform 0.2s ease",
                }, children: [poster ? (_jsx("img", { src: poster, alt: item.title ?? "", className: "w-full h-full object-cover", onError: (e) => {
                            const t = e.currentTarget;
                            t.style.display = "none";
                            const fb = t.nextElementSibling;
                            if (fb)
                                fb.style.display = "flex";
                        } })) : null, _jsx("div", { className: "absolute inset-0 items-center justify-center text-2xl font-bold", style: {
                            backgroundColor: "var(--surface-raised)",
                            color: "var(--text-muted)",
                            display: poster ? "none" : "flex",
                        }, children: initial }), item.year !== undefined && (_jsx("span", { className: "absolute top-2 right-2 text-xs font-medium px-1.5 py-0.5", style: {
                            backgroundColor: "rgba(0,0,0,0.7)",
                            color: "#fff",
                            borderRadius: "var(--radius-sm)",
                        }, children: item.year })), _jsx("div", { className: "absolute inset-0 flex items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity", style: { backgroundColor: "rgba(0,0,0,0.5)" }, children: _jsx("div", { className: "w-12 h-12 rounded-full flex items-center justify-center", style: { backgroundColor: "var(--accent)" }, children: _jsx("svg", { className: "w-5 h-5 text-white ml-1", fill: "currentColor", viewBox: "0 0 20 20", children: _jsx("path", { d: "M6.3 2.8L17 10 6.3 17.2V2.8z" }) }) }) })] }), _jsx("div", { className: "mt-2 px-0.5", children: _jsx("p", { className: "text-sm font-medium truncate", style: { color: "var(--text)" }, children: item.title }) }), _jsx("style", { children: `
        .group:hover > div:first-child { transform: scale(1.05); }
      ` })] }));
}
