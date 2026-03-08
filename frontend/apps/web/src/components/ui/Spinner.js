import { jsx as _jsx } from "react/jsx-runtime";
const sizeClasses = { sm: "w-4 h-4", md: "w-8 h-8", lg: "w-12 h-12" };
export function Spinner({ size = "md", className = "" }) {
    return (_jsx("div", { className: `border-2 border-[var(--accent)] border-t-transparent rounded-full animate-spin ${sizeClasses[size]} ${className}`, "aria-label": "Loading" }));
}
