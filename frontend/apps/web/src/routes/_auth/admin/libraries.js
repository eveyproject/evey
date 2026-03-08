import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import { useQueryClient, useQuery } from "@tanstack/react-query";
import { useLibraryServiceListLibraries, useLibraryServiceDeleteLibrary, useLibraryServiceScanLibrary, libraryServiceListMediaItems, } from "@evey/api";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { Modal } from "@/components/ui/Modal";
import { LibraryForm } from "@/features/admin/LibraryForm";
export const Route = createFileRoute("/_auth/admin/libraries")({
    component: LibrariesPage,
});
function libraryTypeLabel(type) {
    switch (type) {
        case "LIBRARY_TYPE_MOVIES": return "Movies";
        case "LIBRARY_TYPE_SHOWS": return "Shows";
        case "LIBRARY_TYPE_MUSIC": return "Music";
        default: return "Library";
    }
}
function formatDate(iso) {
    if (!iso)
        return "—";
    try {
        return new Date(iso).toLocaleString();
    }
    catch {
        return iso;
    }
}
function LibraryRow({ library }) {
    const queryClient = useQueryClient();
    const [scanDone, setScanDone] = useState(false);
    const deleteLib = useLibraryServiceDeleteLibrary({
        mutation: {
            onSuccess: () => queryClient.invalidateQueries({ queryKey: ["/api/v1/libraries"] }),
        },
    });
    const scanLib = useLibraryServiceScanLibrary({
        mutation: {
            onSuccess: () => {
                setScanDone(true);
                queryClient.invalidateQueries({ queryKey: ["/api/v1/libraries"] });
                queryClient.invalidateQueries({ queryKey: ["/api/v1/media"] });
            },
        },
    });
    const { data: mediaData } = useQuery({
        queryKey: ["/api/v1/media", { libraryId: library.id, pageSize: 1 }],
        queryFn: ({ signal }) => libraryServiceListMediaItems({ libraryId: library.id, "pagination.pageSize": 1 }, signal),
        enabled: !!library.id,
    });
    const totalItems = mediaData?.pagination?.totalItems;
    return (_jsxs("tr", { style: { borderBottom: "1px solid var(--border)" }, children: [_jsx("td", { className: "px-4 py-3", style: { color: "var(--text)" }, children: library.name }), _jsx("td", { className: "px-4 py-3", children: _jsx(Badge, { variant: "info", children: libraryTypeLabel(library.type) }) }), _jsx("td", { className: "px-4 py-3 text-sm", style: { color: "var(--text-muted)" }, children: library.paths?.join(", ") ?? "—" }), _jsx("td", { className: "px-4 py-3 text-sm", style: { color: "var(--text-muted)" }, children: totalItems != null ? `${totalItems} items` : _jsx("span", { className: "opacity-50", children: "\u2014" }) }), _jsx("td", { className: "px-4 py-3 text-sm", style: { color: "var(--text-muted)" }, children: formatDate(library.lastScannedAt) }), _jsx("td", { className: "px-4 py-3 whitespace-nowrap", children: _jsxs("div", { className: "flex items-center gap-2", children: [scanLib.isPending ? (_jsxs("div", { className: "flex items-center gap-2 text-sm", style: { color: "var(--text-muted)" }, children: [_jsx(Spinner, { size: "sm" }), _jsx("span", { children: "Scanning..." })] })) : (_jsx(Button, { variant: "secondary", onClick: () => {
                                setScanDone(false);
                                scanLib.mutate({ id: library.id });
                            }, children: scanDone ? "Scan again" : "Scan" })), _jsx(Button, { variant: "danger", loading: deleteLib.isPending, onClick: () => {
                                if (confirm(`Delete library "${library.name}"?`)) {
                                    deleteLib.mutate({ id: library.id });
                                }
                            }, children: "Delete" })] }) })] }));
}
function LibrariesPage() {
    const [showModal, setShowModal] = useState(false);
    const queryClient = useQueryClient();
    const { data, isLoading } = useLibraryServiceListLibraries();
    const libraries = data?.libraries ?? [];
    function handleCreateSuccess() {
        queryClient.invalidateQueries({ queryKey: ["/api/v1/libraries"] });
        setShowModal(false);
    }
    return (_jsxs("div", { className: "flex flex-col gap-6", children: [_jsxs("div", { className: "flex items-center justify-between", children: [_jsx("h1", { className: "text-2xl font-bold", style: { color: "var(--text)" }, children: "Libraries" }), _jsx(Button, { onClick: () => setShowModal(true), children: "Add Library" })] }), isLoading ? (_jsx("div", { className: "flex justify-center py-16", children: _jsx(Spinner, { size: "lg" }) })) : (_jsx("div", { className: "overflow-x-auto rounded-[var(--radius-lg)]", style: { border: "1px solid var(--border)" }, children: _jsxs("table", { className: "w-full text-sm", children: [_jsx("thead", { children: _jsx("tr", { style: { backgroundColor: "var(--surface)", borderBottom: "1px solid var(--border)" }, children: ["Name", "Type", "Paths", "Items", "Last Scanned", "Actions"].map((h) => (_jsx("th", { className: "px-4 py-2 text-left font-medium", style: { color: "var(--text-muted)" }, children: h }, h))) }) }), _jsx("tbody", { children: libraries.length === 0 ? (_jsx("tr", { children: _jsx("td", { colSpan: 6, className: "px-4 py-8 text-center", style: { color: "var(--text-muted)" }, children: "No libraries. Add one to get started." }) })) : (libraries.map((lib) => _jsx(LibraryRow, { library: lib }, lib.id))) })] }) })), _jsx(Modal, { open: showModal, onClose: () => setShowModal(false), title: "Add Library", children: _jsx(LibraryForm, { onSuccess: handleCreateSuccess }) })] }));
}
