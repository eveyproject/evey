import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { Badge } from "@/components/ui/Badge";
function StreamRow({ stream }) {
    return (_jsxs("div", { className: "flex items-center gap-3 py-2 px-3 rounded-[var(--radius-sm)]", style: { backgroundColor: "var(--surface-raised)" }, children: [stream.codec && (_jsx("span", { className: "text-xs font-mono uppercase", style: { color: "var(--text)" }, children: stream.codec })), stream.language && (_jsx("span", { className: "text-xs uppercase", style: { color: "var(--text-muted)" }, children: stream.language })), stream.title && (_jsx("span", { className: "text-xs flex-1 truncate", style: { color: "var(--text-muted)" }, children: stream.title })), stream.isDefault && _jsx(Badge, { variant: "info", children: "Default" }), stream.isForced && _jsx(Badge, { variant: "warning", children: "Forced" })] }));
}
export function StreamList({ streams }) {
    if (!streams || streams.length === 0)
        return null;
    const audio = streams.filter((s) => s.type === "MEDIA_STREAM_TYPE_AUDIO");
    const subtitles = streams.filter((s) => s.type === "MEDIA_STREAM_TYPE_SUBTITLE");
    return (_jsxs("div", { className: "space-y-4", children: [audio.length > 0 && (_jsxs("div", { children: [_jsx("h3", { className: "text-sm font-semibold mb-2", style: { color: "var(--text-muted)" }, children: "Audio" }), _jsx("div", { className: "space-y-1", children: audio.map((s) => (_jsx(StreamRow, { stream: s }, s.index))) })] })), subtitles.length > 0 && (_jsxs("div", { children: [_jsx("h3", { className: "text-sm font-semibold mb-2", style: { color: "var(--text-muted)" }, children: "Subtitles" }), _jsx("div", { className: "space-y-1", children: subtitles.map((s) => (_jsx(StreamRow, { stream: s }, s.index))) })] }))] }));
}
