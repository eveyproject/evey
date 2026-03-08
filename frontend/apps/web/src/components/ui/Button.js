import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
const variantClasses = {
    primary: "bg-[var(--accent)] hover:bg-[var(--accent-hover)] text-white",
    secondary: "bg-[var(--surface-raised)] hover:bg-[var(--border)] text-[var(--text)]",
    ghost: "bg-transparent hover:bg-[var(--surface-raised)] text-[var(--text-muted)]",
    danger: "bg-red-600 hover:bg-red-700 text-white",
};
export function Button({ variant = "primary", loading, disabled, className = "", children, ...props }) {
    return (_jsxs("button", { ...props, disabled: disabled || loading, className: `inline-flex items-center justify-center gap-2 px-4 py-2 rounded-[var(--radius-md)] text-sm font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed ${variantClasses[variant]} ${className}`, children: [loading && (_jsx("span", { className: "w-4 h-4 border-2 border-current border-t-transparent rounded-full animate-spin" })), children] }));
}
