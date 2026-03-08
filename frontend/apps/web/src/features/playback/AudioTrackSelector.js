import { jsxs as _jsxs, jsx as _jsx } from "react/jsx-runtime";
import { useState } from "react";
export function AudioTrackSelector({ streams, currentIndex, onChange }) {
    const [open, setOpen] = useState(false);
    if (streams.length === 0)
        return null;
    const current = streams.find((s) => s.index === currentIndex) ?? streams[0];
    const label = current?.title ?? current?.language ?? `Track ${current?.index}`;
    return (_jsxs("div", { className: "relative", children: [_jsxs("button", { onClick: () => setOpen((o) => !o), className: "px-3 py-1 text-sm rounded-[var(--radius-sm)] bg-[var(--surface)] text-[var(--text)] hover:bg-[var(--surface-raised)] transition-colors", "aria-label": "Audio track", children: ["Audio: ", label] }), open && (_jsx("div", { className: "absolute bottom-full mb-1 right-0 min-w-max rounded-[var(--radius-md)] bg-[var(--surface)] border border-[var(--border)] shadow-lg overflow-hidden z-10", children: streams.map((s) => {
                    const trackLabel = s.title ?? s.language ?? `Track ${s.index}`;
                    return (_jsx("button", { onClick: () => {
                            onChange(s.index);
                            setOpen(false);
                        }, className: `block w-full text-left px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors ${s.index === currentIndex ? "text-[var(--accent)]" : "text-[var(--text)]"}`, children: trackLabel }, s.index));
                }) }))] }));
}
