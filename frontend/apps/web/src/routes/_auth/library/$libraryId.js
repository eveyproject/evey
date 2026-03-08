import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { useState, useEffect } from "react";
import { createFileRoute } from "@tanstack/react-router";
import { useLibraryServiceListLibraries, useLibraryServiceListMediaItems, } from "@evey/api";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { MediaGrid } from "@/features/library/MediaGrid";
import { MediaCard } from "@/features/library/MediaCard";
export const Route = createFileRoute("/_auth/library/$libraryId")({
    component: LibraryPage,
});
const MOVIE_TYPE_OPTIONS = [
    { label: "All", value: "MEDIA_ITEM_TYPE_MOVIE" },
];
const SHOW_TYPE_OPTIONS = [
    { label: "Series", value: "MEDIA_ITEM_TYPE_SERIES" },
    { label: "Seasons", value: "MEDIA_ITEM_TYPE_SEASON" },
    { label: "Episodes", value: "MEDIA_ITEM_TYPE_EPISODE" },
];
const SORT_OPTIONS = [
    { label: "Title", value: "MEDIA_SORT_FIELD_TITLE" },
    { label: "Year", value: "MEDIA_SORT_FIELD_YEAR" },
    { label: "Rating", value: "MEDIA_SORT_FIELD_RATING" },
    { label: "Date Added", value: "MEDIA_SORT_FIELD_DATE_ADDED" },
];
const PAGE_SIZE = 48;
function LibraryPage() {
    const { libraryId } = Route.useParams();
    const [typeFilter, setTypeFilter] = useState("");
    const [sortField, setSortField] = useState("MEDIA_SORT_FIELD_TITLE");
    const [page, setPage] = useState(1);
    const { data: librariesData } = useLibraryServiceListLibraries();
    const library = librariesData?.libraries?.find((l) => l.id === libraryId);
    const isShowLibrary = library?.type === "LIBRARY_TYPE_SHOWS";
    // Set default type filter based on library type once library data loads
    useEffect(() => {
        if (!library)
            return;
        if (library.type === "LIBRARY_TYPE_SHOWS") {
            setTypeFilter("MEDIA_ITEM_TYPE_SERIES");
        }
        else if (library.type === "LIBRARY_TYPE_MOVIES") {
            setTypeFilter("MEDIA_ITEM_TYPE_MOVIE");
        }
    }, [library?.type]);
    const typeOptions = isShowLibrary ? SHOW_TYPE_OPTIONS : MOVIE_TYPE_OPTIONS;
    const params = {
        libraryId,
        ...(typeFilter ? { type: typeFilter } : {}),
        sortField,
        sortOrder: "SORT_ORDER_ASC",
        "pagination.page": 1,
        "pagination.pageSize": page * PAGE_SIZE,
    };
    const { data, isLoading } = useLibraryServiceListMediaItems(params);
    const items = data?.items ?? [];
    const totalItems = data?.pagination?.totalItems ?? 0;
    const hasMore = items.length < totalItems;
    const selectStyle = {
        backgroundColor: "var(--surface-raised)",
        color: "var(--text)",
        border: "1px solid var(--border)",
        borderRadius: "var(--radius-sm)",
        padding: "0.375rem 0.75rem",
        fontSize: "0.875rem",
    };
    return (_jsxs("div", { children: [_jsxs("div", { className: "flex items-center justify-between gap-4 mb-6 flex-wrap", children: [_jsx("h1", { className: "text-2xl font-bold", style: { color: "var(--text)" }, children: library?.name ?? "Library" }), _jsxs("div", { className: "flex items-center gap-3", children: [typeOptions.length > 1 && (_jsx("select", { value: typeFilter, onChange: (e) => {
                                    setTypeFilter(e.target.value);
                                    setPage(1);
                                }, style: selectStyle, children: typeOptions.map((o) => (_jsx("option", { value: o.value, children: o.label }, o.value))) })), _jsx("select", { value: sortField, onChange: (e) => {
                                    setSortField(e.target.value);
                                    setPage(1);
                                }, style: selectStyle, children: SORT_OPTIONS.map((o) => (_jsx("option", { value: o.value, children: o.label }, o.value))) })] })] }), _jsx(MediaGrid, { isLoading: isLoading, children: items.map((item) => (_jsx(MediaCard, { item: item }, item.id))) }), !isLoading && items.length === 0 && (_jsxs("div", { className: "text-center py-16 rounded-[var(--radius-lg)]", style: {
                    backgroundColor: "var(--surface)",
                    border: "1px solid var(--border)",
                }, children: [_jsx("p", { className: "text-lg mb-2", style: { color: "var(--text)" }, children: "No items found." }), _jsx("p", { className: "mb-4", style: { color: "var(--text-muted)" }, children: typeFilter
                            ? "Try a different filter, or scan the library to add content."
                            : "This library is empty. Trigger a scan to add content." }), _jsx("a", { href: "/admin/libraries", className: "inline-block px-4 py-2 rounded-[var(--radius-md)] text-sm font-medium", style: { backgroundColor: "var(--accent)", color: "#fff" }, children: "Go to Admin \u2192 Libraries" })] })), hasMore && !isLoading && (_jsx("div", { className: "flex justify-center mt-8", children: _jsx(Button, { variant: "secondary", onClick: () => setPage((p) => p + 1), children: "Load more" }) })), isLoading && items.length > 0 && (_jsx("div", { className: "flex justify-center mt-8", children: _jsx(Spinner, {}) }))] }));
}
