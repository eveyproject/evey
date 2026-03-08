import { jsx as _jsx, jsxs as _jsxs, Fragment as _Fragment } from "react/jsx-runtime";
import { useState, useEffect } from "react";
import { createFileRoute } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { libraryServiceSearchMedia } from "@evey/api";
import { Input } from "@/components/ui/Input";
import { Spinner } from "@/components/ui/Spinner";
import { MediaGrid } from "@/features/library/MediaGrid";
import { MediaCard } from "@/features/library/MediaCard";
export const Route = createFileRoute("/_auth/search")({
    component: SearchPage,
});
function SearchPage() {
    const [inputValue, setInputValue] = useState("");
    const [query, setQuery] = useState("");
    useEffect(() => {
        const timer = setTimeout(() => setQuery(inputValue.trim()), 300);
        return () => clearTimeout(timer);
    }, [inputValue]);
    const { data, isLoading } = useQuery({
        queryKey: ["/api/v1/media/search", query],
        queryFn: ({ signal }) => libraryServiceSearchMedia({ query }, signal),
        enabled: query.length > 1,
    });
    const items = data?.items ?? [];
    return (_jsxs("div", { children: [_jsx("h1", { className: "text-2xl font-bold mb-6", style: { color: "var(--text)" }, children: "Search" }), _jsx("div", { className: "mb-6 max-w-lg", children: _jsx(Input, { type: "search", placeholder: "Search movies, series, music...", value: inputValue, onChange: (e) => setInputValue(e.target.value), autoFocus: true }) }), query.length > 1 && isLoading && (_jsx("div", { className: "flex justify-center py-16", children: _jsx(Spinner, { size: "lg" }) })), query.length > 1 && !isLoading && items.length === 0 && (_jsxs("div", { className: "text-center py-16", style: { color: "var(--text-muted)" }, children: ["No results for \u201C", query, "\u201D."] })), query.length <= 1 && !isLoading && (_jsx("div", { className: "text-center py-16", style: { color: "var(--text-muted)" }, children: "Type at least 2 characters to search." })), items.length > 0 && (_jsxs(_Fragment, { children: [_jsxs("p", { className: "text-sm mb-4", style: { color: "var(--text-muted)" }, children: [data?.totalHits ?? items.length, " result", (data?.totalHits ?? items.length) !== 1 ? "s" : "", " for \u201C", query, "\u201D"] }), _jsx(MediaGrid, { children: items.map((item) => (_jsx(MediaCard, { item: item }, item.id))) })] }))] }));
}
