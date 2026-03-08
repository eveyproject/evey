import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { useState, useEffect, useCallback } from "react";
import { createFileRoute } from "@tanstack/react-router";
import { useQueryClient, useQuery } from "@tanstack/react-query";
import { useLibraryServiceListMediaItems, libraryServiceGetMediaItem, eveyFetch, } from "@evey/api";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { Input } from "@/components/ui/Input";
export const Route = createFileRoute("/_auth/admin/metadata")({
    component: MetadataPage,
});
function mediaTypeForTmdb(item) {
    if (item.type === "MEDIA_ITEM_TYPE_SERIES")
        return "tv";
    return "movie";
}
function posterSrc(id) {
    return `/api/v1/media/${id}/artwork/poster`;
}
function TmdbSearchPanel({ item, onApplied, }) {
    const [results, setResults] = useState(null);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);
    const [applying, setApplying] = useState(null);
    const queryClient = useQueryClient();
    const search = useCallback(async () => {
        if (!item.id)
            return;
        setLoading(true);
        setError(null);
        try {
            const data = await eveyFetch({
                url: `/api/v1/admin/media/${item.id}/search-metadata`,
                method: "GET",
                params: { query: item.title ?? "", type: mediaTypeForTmdb(item) },
            });
            setResults(data ?? []);
        }
        catch (e) {
            setError(e.message ?? "Search failed");
            setResults([]);
        }
        finally {
            setLoading(false);
        }
    }, [item]);
    async function apply(result) {
        if (!item.id)
            return;
        setApplying(result.tmdbId);
        try {
            await eveyFetch({
                url: `/api/v1/admin/media/${item.id}/refresh-metadata`,
                method: "POST",
                headers: { "Content-Type": "application/json" },
                data: { tmdb_id: result.tmdbId, media_type: mediaTypeForTmdb(item) },
            });
            await queryClient.invalidateQueries({ queryKey: [`/api/v1/media/${item.id}`] });
            onApplied();
        }
        catch (e) {
            setError(e.message ?? "Apply failed");
        }
        finally {
            setApplying(null);
        }
    }
    return (_jsxs("div", { className: "flex flex-col gap-3 mt-4", children: [_jsx(Button, { variant: "secondary", onClick: search, loading: loading, children: "Search TMDB" }), error && _jsx("p", { className: "text-sm text-red-400", children: error }), results !== null && results.length === 0 && !loading && (_jsx("p", { className: "text-sm", style: { color: "var(--text-muted)" }, children: "No TMDB results found." })), results && results.length > 0 && (_jsx("div", { className: "flex flex-col gap-2 max-h-64 overflow-y-auto", children: results.map((r) => (_jsxs("div", { className: "flex items-start gap-3 p-3 rounded-[var(--radius-md)]", style: { backgroundColor: "var(--surface-raised)", border: "1px solid var(--border)" }, children: [r.posterUrl && (_jsx("img", { src: r.posterUrl, alt: r.title, className: "w-12 h-16 object-cover rounded flex-shrink-0", onError: (e) => { e.target.style.display = "none"; } })), _jsxs("div", { className: "flex-1 min-w-0", children: [_jsxs("p", { className: "font-medium text-sm", style: { color: "var(--text)" }, children: [r.title, " ", r.year ? `(${r.year})` : ""] }), _jsx("p", { className: "text-xs mt-1 line-clamp-2", style: { color: "var(--text-muted)" }, children: r.overview })] }), _jsx(Button, { variant: "primary", loading: applying === r.tmdbId, onClick: () => apply(r), className: "flex-shrink-0", children: "Apply" })] }, r.tmdbId))) }))] }));
}
function GenrePills({ genres, onChange, }) {
    const [input, setInput] = useState("");
    function add() {
        const trimmed = input.trim();
        if (trimmed && !genres.includes(trimmed)) {
            onChange([...genres, trimmed]);
        }
        setInput("");
    }
    function remove(g) {
        onChange(genres.filter((x) => x !== g));
    }
    return (_jsxs("div", { className: "flex flex-col gap-2", children: [_jsx("label", { className: "text-sm font-medium", style: { color: "var(--text-muted)" }, children: "Genres" }), _jsx("div", { className: "flex flex-wrap gap-1 min-h-8", children: genres.map((g) => (_jsxs("span", { className: "inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs", style: { backgroundColor: "var(--surface-raised)", color: "var(--text)" }, children: [g, _jsx("button", { onClick: () => remove(g), className: "hover:text-red-400 transition-colors", "aria-label": `Remove ${g}`, children: "\u00D7" })] }, g))) }), _jsxs("div", { className: "flex gap-2", children: [_jsx("input", { className: "flex-1 px-3 py-1.5 rounded-[var(--radius-md)] text-sm bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)] placeholder:text-[var(--text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]", placeholder: "Add genre...", value: input, onChange: (e) => setInput(e.target.value), onKeyDown: (e) => { if (e.key === "Enter") {
                            e.preventDefault();
                            add();
                        } } }), _jsx(Button, { variant: "secondary", onClick: add, disabled: !input.trim(), children: "Add" })] })] }));
}
function MetadataEditorModal({ itemId, open, onClose, }) {
    const { data: item, isLoading } = useQuery({
        queryKey: [`/api/v1/media/${itemId}`],
        queryFn: ({ signal }) => libraryServiceGetMediaItem(itemId, signal),
        enabled: open && !!itemId,
    });
    const [title, setTitle] = useState("");
    const [overview, setOverview] = useState("");
    const [year, setYear] = useState("");
    const [genres, setGenres] = useState([]);
    const [showTmdb, setShowTmdb] = useState(false);
    useEffect(() => {
        if (item) {
            setTitle(item.title ?? "");
            setOverview(item.overview ?? "");
            setYear(item.year != null ? String(item.year) : "");
            setGenres(item.genres ?? []);
            setShowTmdb(false);
        }
    }, [item]);
    if (!open)
        return null;
    const poster = `/api/v1/media/${itemId}/artwork/poster`;
    const backdrop = `/api/v1/media/${itemId}/artwork/backdrop`;
    return (_jsx(Modal, { open: open, onClose: onClose, title: item?.title ?? "Edit Metadata", children: isLoading ? (_jsx("div", { className: "flex justify-center py-8", children: _jsx(Spinner, {}) })) : item ? (_jsxs("div", { className: "flex flex-col gap-4 max-h-[80vh] overflow-y-auto pr-1", children: [_jsxs("div", { className: "flex gap-3", children: [_jsx("img", { src: poster, alt: "Poster", className: "w-20 h-28 object-cover rounded-[var(--radius-md)] flex-shrink-0", onError: (e) => { e.target.style.display = "none"; } }), _jsx("img", { src: backdrop, alt: "Backdrop", className: "flex-1 h-28 object-cover rounded-[var(--radius-md)]", onError: (e) => { e.target.style.display = "none"; } })] }), _jsx(Input, { label: "Title", value: title, onChange: (e) => setTitle(e.target.value) }), _jsxs("div", { className: "flex flex-col gap-1", children: [_jsx("label", { className: "text-sm font-medium", style: { color: "var(--text-muted)" }, children: "Overview" }), _jsx("textarea", { className: "px-3 py-2 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)] placeholder:text-[var(--text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)] resize-y min-h-20", value: overview, onChange: (e) => setOverview(e.target.value), placeholder: "Overview..." })] }), _jsx(Input, { label: "Year", type: "number", value: year, onChange: (e) => setYear(e.target.value) }), _jsx(GenrePills, { genres: genres, onChange: setGenres }), _jsxs("div", { children: [_jsx(Button, { variant: "secondary", onClick: () => setShowTmdb((v) => !v), children: showTmdb ? "Hide TMDB Search" : "Search TMDB" }), showTmdb && (_jsx(TmdbSearchPanel, { item: { ...item, title }, onApplied: () => {
                                setShowTmdb(false);
                                onClose();
                            } }))] })] })) : (_jsx("p", { style: { color: "var(--text-muted)" }, children: "Failed to load item." })) }));
}
function MediaThumb({ item, onClick }) {
    return (_jsxs("button", { onClick: onClick, className: "flex flex-col gap-2 text-left group", children: [_jsx("div", { className: "w-full aspect-[2/3] rounded-[var(--radius-md)] overflow-hidden", style: { backgroundColor: "var(--surface-raised)" }, children: item.id && (_jsx("img", { src: posterSrc(item.id), alt: item.title, className: "w-full h-full object-cover group-hover:scale-105 transition-transform duration-200", onError: (e) => { e.target.style.display = "none"; } })) }), _jsxs("div", { children: [_jsx("p", { className: "text-sm font-medium truncate", style: { color: "var(--text)" }, children: item.title ?? "Untitled" }), _jsxs("p", { className: "text-xs", style: { color: "var(--text-muted)" }, children: [item.type?.replace("MEDIA_ITEM_TYPE_", "") ?? "", item.year ? ` · ${item.year}` : ""] })] })] }));
}
function MetadataPage() {
    const [query, setQuery] = useState("");
    const [debouncedQuery, setDebouncedQuery] = useState("");
    const [selectedId, setSelectedId] = useState(null);
    useEffect(() => {
        const t = setTimeout(() => setDebouncedQuery(query), 400);
        return () => clearTimeout(t);
    }, [query]);
    const { data, isLoading } = useLibraryServiceListMediaItems({
        "pagination.pageSize": 96,
    });
    const items = data?.items ?? [];
    const filtered = debouncedQuery
        ? items.filter((i) => i.title?.toLowerCase().includes(debouncedQuery.toLowerCase()))
        : items;
    return (_jsxs("div", { className: "flex flex-col gap-6", children: [_jsx("h1", { className: "text-2xl font-bold", style: { color: "var(--text)" }, children: "Metadata" }), _jsx("input", { className: "w-full px-4 py-2.5 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)] placeholder:text-[var(--text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]", placeholder: "Search media items...", value: query, onChange: (e) => setQuery(e.target.value) }), isLoading ? (_jsx("div", { className: "flex justify-center py-16", children: _jsx(Spinner, { size: "lg" }) })) : filtered.length === 0 ? (_jsx("p", { className: "text-center py-16", style: { color: "var(--text-muted)" }, children: debouncedQuery ? "No results." : "No media items found." })) : (_jsx("div", { className: "grid grid-cols-2 sm:grid-cols-4 md:grid-cols-6 lg:grid-cols-8 gap-4", children: filtered.map((item) => (_jsx(MediaThumb, { item: item, onClick: () => setSelectedId(item.id ?? null) }, item.id))) })), selectedId && (_jsx(MetadataEditorModal, { itemId: selectedId, open: true, onClose: () => setSelectedId(null) }))] }));
}
