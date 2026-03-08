import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { usePlaybackServiceMarkAsWatched, usePlaybackServiceMarkAsUnwatched, useFeedServiceSimilarItems, useLibraryServiceListMediaItems, useLibraryServiceGetMediaItem, } from "@evey/api";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { StreamList } from "./StreamList";
import { MediaCard } from "./MediaCard";
function formatDuration(ms) {
    if (!ms)
        return "";
    const totalMs = parseInt(ms, 10);
    if (isNaN(totalMs))
        return "";
    const totalSec = Math.floor(totalMs / 1000);
    const h = Math.floor(totalSec / 3600);
    const m = Math.floor((totalSec % 3600) / 60);
    if (h > 0)
        return `${h}h ${m}m`;
    return `${m}m`;
}
function formatFileSize(bytes) {
    if (!bytes)
        return "";
    const n = parseInt(bytes, 10);
    if (isNaN(n))
        return "";
    if (n >= 1_073_741_824)
        return `${(n / 1_073_741_824).toFixed(1)} GB`;
    if (n >= 1_048_576)
        return `${(n / 1_048_576).toFixed(0)} MB`;
    return `${(n / 1024).toFixed(0)} KB`;
}
function Stars({ rating }) {
    const full = Math.round(rating / 2);
    return (_jsx("span", { className: "flex items-center gap-0.5", children: Array.from({ length: 5 }, (_, i) => (_jsx("svg", { className: "w-3.5 h-3.5", fill: i < full ? "var(--accent)" : "none", stroke: "var(--accent)", strokeWidth: 1.5, viewBox: "0 0 24 24", children: _jsx("path", { d: "M12 2l3.09 6.26L22 9.27l-5 4.87L18.18 21 12 17.77 5.82 21 7 14.14 2 9.27l6.91-1.01L12 2z" }) }, i))) }));
}
function CastCard({ person }) {
    const initial = person.name ? person.name[0].toUpperCase() : "?";
    return (_jsxs(Link, { to: "/person/$personId", params: { personId: person.id ?? "" }, className: "flex-shrink-0 w-28 text-center group", children: [_jsxs("div", { className: "w-20 h-20 rounded-full mx-auto overflow-hidden flex items-center justify-center mb-2", style: { backgroundColor: "var(--surface-raised)" }, children: [person.thumbUrl ? (_jsx("img", { src: person.thumbUrl, alt: person.name ?? "", className: "w-full h-full object-cover", onError: (e) => {
                            const t = e.currentTarget;
                            t.style.display = "none";
                            const fb = t.nextElementSibling;
                            if (fb)
                                fb.style.display = "flex";
                        } })) : null, _jsx("span", { className: "text-xl font-bold", style: {
                            color: "var(--text-muted)",
                            display: person.thumbUrl ? "none" : "flex",
                        }, children: initial })] }), _jsx("p", { className: "text-xs font-medium truncate", style: { color: "var(--text)" }, children: person.name }), person.role && (_jsx("p", { className: "text-xs truncate", style: { color: "var(--text-muted)" }, children: person.role }))] }));
}
function SeasonRow({ seasonId }) {
    const { data: season } = useLibraryServiceGetMediaItem(seasonId);
    const { data: episodesData } = useLibraryServiceListMediaItems({
        parentId: seasonId,
        sortField: "MEDIA_SORT_FIELD_TITLE",
        sortOrder: "SORT_ORDER_ASC",
    });
    const [open, setOpen] = useState(false);
    const episodes = episodesData?.items ?? [];
    return (_jsxs("div", { className: "mb-2", style: { border: "1px solid var(--border)", borderRadius: "var(--radius-md)" }, children: [_jsxs("button", { onClick: () => setOpen((v) => !v), className: "w-full flex items-center justify-between px-4 py-3 text-left", style: { backgroundColor: "var(--surface)", borderRadius: "var(--radius-md)" }, children: [_jsx("span", { className: "font-medium", style: { color: "var(--text)" }, children: season?.title ?? "Season" }), _jsxs("span", { className: "text-sm", style: { color: "var(--text-muted)" }, children: [episodes.length, " ep ", open ? "▲" : "▼"] })] }), open && (_jsx("div", { className: "divide-y", style: { borderColor: "var(--border)" }, children: episodes.map((ep) => (_jsxs(Link, { to: "/player/$mediaId", params: { mediaId: ep.id ?? "" }, className: "flex items-center gap-3 px-4 py-2 hover:opacity-80 transition-opacity", style: { color: "var(--text)" }, children: [_jsx("span", { className: "text-sm w-8 text-center flex-shrink-0", style: { color: "var(--text-muted)" }, children: ep.indexNumber ?? "—" }), _jsx("span", { className: "text-sm flex-1 truncate", children: ep.title }), ep.durationMs && (_jsx("span", { className: "text-xs flex-shrink-0", style: { color: "var(--text-muted)" }, children: formatDuration(ep.durationMs) }))] }, ep.id))) }))] }));
}
function SeriesSection({ item }) {
    const { data: seasonsData } = useLibraryServiceListMediaItems({
        parentId: item.id ?? "",
        sortField: "MEDIA_SORT_FIELD_TITLE",
        sortOrder: "SORT_ORDER_ASC",
    });
    const seasons = seasonsData?.items ?? [];
    if (!seasons.length)
        return null;
    return (_jsxs("div", { className: "mt-10", children: [_jsx("h2", { className: "text-lg font-semibold mb-4", style: { color: "var(--text)" }, children: "Seasons" }), seasons.map((s) => s.id ? _jsx(SeasonRow, { seasonId: s.id }, s.id) : null)] }));
}
function SeasonEpisodesSection({ item }) {
    const { data: episodesData } = useLibraryServiceListMediaItems({
        parentId: item.id ?? "",
        sortField: "MEDIA_SORT_FIELD_TITLE",
        sortOrder: "SORT_ORDER_ASC",
    });
    const episodes = episodesData?.items ?? [];
    if (!episodes.length)
        return null;
    return (_jsxs("div", { className: "mt-10", children: [_jsx("h2", { className: "text-lg font-semibold mb-4", style: { color: "var(--text)" }, children: "Episodes" }), _jsx("div", { className: "divide-y", style: { border: "1px solid var(--border)", borderRadius: "var(--radius-md)", borderColor: "var(--border)" }, children: episodes.map((ep) => (_jsxs(Link, { to: "/player/$mediaId", params: { mediaId: ep.id ?? "" }, className: "flex items-center gap-3 px-4 py-2 hover:opacity-80 transition-opacity", style: { color: "var(--text)" }, children: [_jsx("span", { className: "text-sm w-8 text-center flex-shrink-0", style: { color: "var(--text-muted)" }, children: ep.indexNumber ?? "—" }), _jsx("span", { className: "text-sm flex-1 truncate", children: ep.title }), ep.durationMs && (_jsx("span", { className: "text-xs flex-shrink-0", style: { color: "var(--text-muted)" }, children: formatDuration(ep.durationMs) }))] }, ep.id))) })] }));
}
function SimilarSection({ item }) {
    const { data: similarData } = useFeedServiceSimilarItems(item.id ?? "");
    const firstGenre = item.genres?.[0];
    const hasSimilar = (similarData?.items?.length ?? 0) > 0;
    const { data: byGenreData } = useLibraryServiceListMediaItems({ genre: firstGenre, type: item.type }, 
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    { query: { enabled: !hasSimilar && !!firstGenre } });
    // Feed items have mediaItemId; derive simple card items from them
    const fromFeed = (similarData?.items ?? [])
        .filter((fi) => !!fi.mediaItemId)
        .map((fi) => ({ id: fi.mediaItemId, title: fi.title }));
    // Fallback: media items from same genre
    const fromGenre = (byGenreData?.items ?? []).filter((m) => m.id && m.id !== item.id).map((m) => ({
        id: m.id,
        title: m.title,
        year: m.year,
        type: m.type,
    }));
    const cards = fromFeed.length ? fromFeed : fromGenre;
    if (!cards.length)
        return null;
    return (_jsxs("div", { className: "mt-10", children: [_jsx("h2", { className: "text-lg font-semibold mb-4", style: { color: "var(--text)" }, children: "Similar Titles" }), _jsx("div", { className: "flex gap-4 overflow-x-auto pb-2", style: { scrollbarWidth: "thin" }, children: cards.map((c) => (_jsx(MediaCard, { item: c }, c.id))) })] }));
}
export function MediaDetail({ item }) {
    const [watched, setWatched] = useState(false);
    const [fileInfoOpen, setFileInfoOpen] = useState(false);
    const markWatched = usePlaybackServiceMarkAsWatched({
        mutation: { onSuccess: () => setWatched(true) },
    });
    const markUnwatched = usePlaybackServiceMarkAsUnwatched({
        mutation: { onSuccess: () => setWatched(false) },
    });
    const backdropUrl = item.artwork?.find((a) => a.type === "ARTWORK_TYPE_BACKDROP")?.url
        ?? (item.id ? `/api/v1/media/${item.id}/artwork/backdrop` : undefined);
    const posterUrl = item.artwork?.find((a) => a.type === "ARTWORK_TYPE_PRIMARY")?.url
        ?? (item.id ? `/api/v1/media/${item.id}/artwork/poster` : undefined);
    const duration = formatDuration(item.durationMs);
    const actors = item.people?.filter((p) => p.type === "PERSON_TYPE_ACTOR") ?? [];
    const directors = item.people?.filter((p) => p.type === "PERSON_TYPE_DIRECTOR") ?? [];
    return (_jsxs("div", { children: [_jsxs("div", { className: "relative w-full overflow-hidden -mx-6 -mt-6 mb-0", style: { height: "420px" }, children: [backdropUrl ? (_jsx("img", { src: backdropUrl, alt: "", className: "w-full h-full object-cover" })) : (_jsx("div", { className: "w-full h-full", style: { backgroundColor: "var(--surface)" } })), _jsx("div", { className: "absolute inset-0", style: { background: "linear-gradient(to top, var(--bg) 0%, rgba(0,0,0,0.3) 60%, transparent 100%)" } }), _jsxs("div", { className: "absolute bottom-0 left-0 right-0 px-6 pb-6", children: [_jsxs("div", { className: "flex items-center gap-2 mb-2 flex-wrap", children: [item.officialRating && (_jsx("span", { className: "text-xs font-medium px-2 py-0.5 border", style: {
                                            color: "rgba(255,255,255,0.9)",
                                            borderColor: "rgba(255,255,255,0.5)",
                                            borderRadius: "var(--radius-sm)",
                                        }, children: item.officialRating })), duration && (_jsx("span", { className: "text-sm", style: { color: "rgba(255,255,255,0.8)" }, children: duration })), item.communityRating !== undefined && item.communityRating > 0 && (_jsxs("span", { className: "flex items-center gap-1.5", children: [_jsx(Stars, { rating: item.communityRating }), _jsx("span", { className: "text-sm", style: { color: "rgba(255,255,255,0.8)" }, children: item.communityRating.toFixed(1) })] }))] }), _jsxs("h1", { className: "text-4xl font-bold text-white drop-shadow-lg mb-2", children: [item.title, item.year && (_jsxs("span", { className: "text-2xl font-normal ml-3", style: { color: "rgba(255,255,255,0.7)" }, children: ["(", item.year, ")"] }))] }), item.genres && item.genres.length > 0 && (_jsx("div", { className: "flex flex-wrap gap-2", children: item.genres.map((g) => (_jsx("span", { className: "text-xs px-2 py-0.5", style: {
                                        backgroundColor: "rgba(255,255,255,0.15)",
                                        color: "rgba(255,255,255,0.9)",
                                        borderRadius: "var(--radius-sm)",
                                    }, children: g }, g))) }))] })] }), _jsxs("div", { className: "flex gap-6 mt-6", children: [_jsx("div", { className: "flex-shrink-0 w-48 hidden md:block", children: _jsx("div", { className: "w-48 overflow-hidden", style: { borderRadius: "var(--radius-md)", aspectRatio: "2/3", backgroundColor: "var(--surface-raised)" }, children: posterUrl ? (_jsx("img", { src: posterUrl, alt: item.title ?? "", className: "w-full h-full object-cover" })) : (_jsx("div", { className: "w-full h-full flex items-center justify-center text-3xl font-bold", style: { color: "var(--text-muted)" }, children: item.title?.[0]?.toUpperCase() ?? "?" })) }) }), _jsxs("div", { className: "flex-1 space-y-4 min-w-0", children: [item.tagline && (_jsx("p", { className: "text-base italic", style: { color: "var(--text-muted)" }, children: item.tagline })), item.overview && (_jsx("p", { className: "text-sm leading-relaxed", style: { color: "var(--text-muted)" }, children: item.overview })), directors.length > 0 && (_jsxs("p", { className: "text-sm", style: { color: "var(--text-muted)" }, children: [_jsx("span", { className: "font-medium", style: { color: "var(--text)" }, children: "Director: " }), directors.map((d) => d.name).join(", ")] })), item.studios && item.studios.length > 0 && (_jsxs("p", { className: "text-sm", style: { color: "var(--text-muted)" }, children: [_jsx("span", { className: "font-medium", style: { color: "var(--text)" }, children: "Studio: " }), item.studios.join(", ")] })), _jsxs("div", { className: "flex gap-3 flex-wrap pt-2", children: [item.id && item.type !== "MEDIA_ITEM_TYPE_SERIES" && item.type !== "MEDIA_ITEM_TYPE_SEASON" && (_jsx(Link, { to: "/player/$mediaId", params: { mediaId: item.id }, children: _jsxs(Button, { variant: "primary", children: [_jsx("svg", { className: "w-4 h-4", fill: "currentColor", viewBox: "0 0 20 20", children: _jsx("path", { d: "M6.3 2.8L17 10 6.3 17.2V2.8z" }) }), "Play"] }) })), item.id && (watched ? (_jsx(Button, { variant: "secondary", loading: markUnwatched.isPending, onClick: () => markUnwatched.mutate({ mediaItemId: item.id }), children: "Unmark Watched" })) : (_jsx(Button, { variant: "secondary", loading: markWatched.isPending, onClick: () => markWatched.mutate({ mediaItemId: item.id }), children: "\u2713 Mark Watched" })))] })] })] }), actors.length > 0 && (_jsxs("div", { className: "mt-10", children: [_jsx("h2", { className: "text-lg font-semibold mb-4", style: { color: "var(--text)" }, children: "Cast" }), _jsx("div", { className: "flex gap-4 overflow-x-auto pb-2", style: { scrollbarWidth: "thin" }, children: actors.map((person) => (_jsx(CastCard, { person: person }, person.id ?? person.name))) })] })), item.type === "MEDIA_ITEM_TYPE_SERIES" && _jsx(SeriesSection, { item: item }), item.type === "MEDIA_ITEM_TYPE_SEASON" && _jsx(SeasonEpisodesSection, { item: item }), item.id && item.type !== "MEDIA_ITEM_TYPE_SEASON" && _jsx(SimilarSection, { item: item }), (item.filePath || item.fileSize || item.containerFormat || (item.streams && item.streams.length > 0)) && (_jsxs("div", { className: "mt-10", children: [_jsxs("button", { className: "flex items-center gap-2 text-sm font-semibold mb-3", style: { color: "var(--text-muted)" }, onClick: () => setFileInfoOpen((v) => !v), children: [_jsx("svg", { className: "w-4 h-4 transition-transform", style: { transform: fileInfoOpen ? "rotate(90deg)" : "rotate(0deg)" }, fill: "none", stroke: "currentColor", strokeWidth: 2, viewBox: "0 0 24 24", children: _jsx("path", { d: "M9 18l6-6-6-6" }) }), "File Info"] }), fileInfoOpen && (_jsxs("div", { className: "space-y-2", children: [item.filePath && (_jsx("div", { className: "text-xs font-mono truncate", style: { color: "var(--text-muted)" }, children: item.filePath })), _jsxs("div", { className: "flex gap-4 flex-wrap", children: [item.fileSize && (_jsx(Badge, { variant: "default", children: formatFileSize(item.fileSize) })), item.containerFormat && (_jsx(Badge, { variant: "default", children: item.containerFormat.toUpperCase() })), item.bitrate && (_jsxs(Badge, { variant: "default", children: [Math.round(parseInt(item.bitrate) / 1000), " kbps"] })), duration && _jsx(Badge, { variant: "default", children: duration })] }), item.streams && item.streams.length > 0 && (_jsx("div", { className: "mt-4", children: _jsx(StreamList, { streams: item.streams }) }))] }))] }))] }));
}
