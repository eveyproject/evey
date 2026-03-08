import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { useEffect, useState } from "react";
import { eveyFetch, useLibraryServiceCreateLibrary } from "@evey/api";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
export function LibraryForm({ onSuccess }) {
    const [name, setName] = useState("");
    const [type, setType] = useState("LIBRARY_TYPE_MOVIES");
    const [paths, setPaths] = useState([""]);
    const [mediaDirs, setMediaDirs] = useState([]);
    useEffect(() => {
        eveyFetch({
            url: "/api/v1/admin/fs/browse",
            method: "GET",
            params: { path: "/media" },
        })
            .then((dirs) => setMediaDirs(dirs.map((d) => d.path)))
            .catch(() => setMediaDirs([]));
    }, []);
    const createLib = useLibraryServiceCreateLibrary({
        mutation: { onSuccess },
    });
    function addPath() {
        setPaths((prev) => [...prev, ""]);
    }
    function removePath(i) {
        setPaths((prev) => prev.filter((_, idx) => idx !== i));
    }
    function updatePath(i, val) {
        setPaths((prev) => prev.map((p, idx) => (idx === i ? val : p)));
    }
    function handleSubmit(e) {
        e.preventDefault();
        createLib.mutate({
            data: {
                name,
                type,
                paths: paths.filter((p) => p.trim() !== "" && p !== "__custom__"),
            },
        });
    }
    const inputStyle = {
        backgroundColor: "var(--surface-raised)",
        border: "1px solid var(--border)",
        color: "var(--text)",
        borderRadius: "var(--radius-md)",
        padding: "0.5rem 0.75rem",
        fontSize: "0.875rem",
        flex: 1,
        outline: "none",
    };
    return (_jsxs("form", { onSubmit: handleSubmit, className: "flex flex-col gap-4", children: [_jsx(Input, { label: "Name", value: name, onChange: (e) => setName(e.target.value), required: true }), _jsxs("div", { className: "flex flex-col gap-1", children: [_jsx("label", { className: "text-sm font-medium", style: { color: "var(--text-muted)" }, children: "Type" }), _jsxs("select", { value: type, onChange: (e) => setType(e.target.value), style: { ...inputStyle, flex: undefined, width: "100%" }, children: [_jsx("option", { value: "LIBRARY_TYPE_MOVIES", children: "Movies" }), _jsx("option", { value: "LIBRARY_TYPE_SHOWS", children: "TV Shows" }), _jsx("option", { value: "LIBRARY_TYPE_MUSIC", children: "Music" })] })] }), _jsxs("div", { className: "flex flex-col gap-2", children: [_jsxs("span", { className: "text-sm font-medium", style: { color: "var(--text-muted)" }, children: ["Paths", mediaDirs.length === 0 && (_jsx("span", { className: "ml-2 text-xs font-normal", children: "(enter the full path inside the container, e.g. /media/movies)" }))] }), paths.map((p, i) => (_jsxs("div", { className: "flex gap-2 items-center", children: [mediaDirs.length > 0 && p !== "__custom__" ? (_jsxs("select", { value: p, onChange: (e) => updatePath(i, e.target.value), style: inputStyle, children: [_jsx("option", { value: "", children: "\u2014 select a directory \u2014" }), mediaDirs.map((d) => (_jsx("option", { value: d, children: d }, d))), _jsx("option", { value: "__custom__", children: "Enter custom path..." })] })) : (_jsx("input", { style: inputStyle, placeholder: "/media/movies", value: p === "__custom__" ? "" : p, onChange: (e) => updatePath(i, e.target.value), autoFocus: p === "__custom__" })), paths.length > 1 && (_jsx(Button, { type: "button", variant: "danger", onClick: () => removePath(i), children: "Remove" }))] }, i))), _jsx(Button, { type: "button", variant: "secondary", onClick: addPath, className: "self-start", children: "+ Add Path" })] }), _jsx("div", { className: "flex justify-end gap-2 pt-2", children: _jsx(Button, { type: "submit", loading: createLib.isPending, disabled: !name.trim() || paths.every((p) => !p.trim() || p === "__custom__"), children: "Create Library" }) })] }));
}
