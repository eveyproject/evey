import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { createFileRoute } from "@tanstack/react-router";
import { useLibraryServiceListLibraries } from "@evey/api";
import { Spinner } from "@/components/ui/Spinner";
import { Badge } from "@/components/ui/Badge";
export const Route = createFileRoute("/_auth/library/")({
    component: LibraryIndex,
});
function libraryTypeLabel(type) {
    switch (type) {
        case "LIBRARY_TYPE_MOVIES": return "Movies";
        case "LIBRARY_TYPE_SHOWS": return "Shows";
        case "LIBRARY_TYPE_MUSIC": return "Music";
        case "LIBRARY_TYPE_PHOTOS": return "Photos";
        case "LIBRARY_TYPE_BOOKS": return "Books";
        default: return "Library";
    }
}
function LibraryCard({ library }) {
    const label = libraryTypeLabel(library.type);
    const pathCount = library.paths?.length ?? 0;
    return (_jsxs("a", { href: `/library/${library.id}`, className: "block p-5 transition-colors", style: {
            backgroundColor: "var(--surface)",
            border: "1px solid var(--border)",
            borderRadius: "var(--radius-lg)",
        }, onMouseEnter: (e) => {
            e.currentTarget.style.backgroundColor = "var(--surface-raised)";
        }, onMouseLeave: (e) => {
            e.currentTarget.style.backgroundColor = "var(--surface)";
        }, children: [_jsxs("div", { className: "flex items-start justify-between gap-2 mb-3", children: [_jsx("h2", { className: "text-lg font-semibold truncate", style: { color: "var(--text)" }, children: library.name }), _jsx(Badge, { variant: "info", children: label })] }), _jsxs("p", { className: "text-sm", style: { color: "var(--text-muted)" }, children: [pathCount, " ", pathCount === 1 ? "path" : "paths"] })] }));
}
function LibraryIndex() {
    const { data, isLoading } = useLibraryServiceListLibraries();
    const libraries = data?.libraries ?? [];
    if (isLoading) {
        return (_jsx("div", { className: "flex justify-center py-16", children: _jsx(Spinner, { size: "lg" }) }));
    }
    return (_jsxs("div", { children: [_jsx("h1", { className: "text-2xl font-bold mb-6", style: { color: "var(--text)" }, children: "Libraries" }), libraries.length === 0 ? (_jsxs("div", { className: "text-center py-16 rounded-[var(--radius-lg)]", style: { backgroundColor: "var(--surface)", border: "1px solid var(--border)" }, children: [_jsx("p", { className: "text-lg mb-2", style: { color: "var(--text)" }, children: "No libraries configured." }), _jsxs("p", { style: { color: "var(--text-muted)" }, children: ["Go to", " ", _jsx("a", { href: "/admin/libraries", style: { color: "var(--accent)" }, children: "Admin" }), " ", "to add one."] })] })) : (_jsx("div", { className: "grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4", children: libraries.map((lib) => (_jsx(LibraryCard, { library: lib }, lib.id))) }))] }));
}
