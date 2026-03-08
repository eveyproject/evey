import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
export function Input({ label, error, className = "", id, ...props }) {
    const inputId = id ?? label?.toLowerCase().replace(/\s+/g, "-");
    return (_jsxs("div", { className: "flex flex-col gap-1", children: [label && (_jsx("label", { htmlFor: inputId, className: "text-sm font-medium text-[var(--text-muted)]", children: label })), _jsx("input", { id: inputId, ...props, className: `px-3 py-2 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)] placeholder:text-[var(--text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent ${error ? "border-red-500" : ""} ${className}` }), error && _jsx("span", { className: "text-xs text-red-400", children: error })] }));
}
