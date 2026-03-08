import { jsxs as _jsxs, jsx as _jsx } from "react/jsx-runtime";
import { useState } from "react";
export function SubtitleSelector({ streams, videoRef }) {
    const [open, setOpen] = useState(false);
    const [selectedIndex, setSelectedIndex] = useState(null);
    if (streams.length === 0)
        return null;
    function applyTrack(index) {
        const video = videoRef.current;
        if (!video)
            return;
        const tracks = video.textTracks;
        for (let i = 0; i < tracks.length; i++) {
            tracks[i].mode = tracks[i].id === String(index) ? "showing" : "hidden";
        }
        setSelectedIndex(index);
        setOpen(false);
    }
    const current = selectedIndex !== null
        ? streams.find((s) => s.index === selectedIndex)
        : null;
    const label = current
        ? (current.title ?? current.language ?? `Track ${current.index}`)
        : "Off";
    return (_jsxs("div", { className: "relative", children: [_jsxs("button", { onClick: () => setOpen((o) => !o), className: "px-3 py-1 text-sm rounded-[var(--radius-sm)] bg-[var(--surface)] text-[var(--text)] hover:bg-[var(--surface-raised)] transition-colors", "aria-label": "Subtitles", children: ["Subtitles: ", label] }), open && (_jsxs("div", { className: "absolute bottom-full mb-1 right-0 min-w-max rounded-[var(--radius-md)] bg-[var(--surface)] border border-[var(--border)] shadow-lg overflow-hidden z-10", children: [_jsx("button", { onClick: () => applyTrack(null), className: `block w-full text-left px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors ${selectedIndex === null ? "text-[var(--accent)]" : "text-[var(--text)]"}`, children: "Off" }), streams.map((s) => {
                        const trackLabel = s.title ?? s.language ?? `Track ${s.index}`;
                        return (_jsx("button", { onClick: () => applyTrack(s.index), className: `block w-full text-left px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors ${s.index === selectedIndex ? "text-[var(--accent)]" : "text-[var(--text)]"}`, children: trackLabel }, s.index));
                    })] }))] }));
}
