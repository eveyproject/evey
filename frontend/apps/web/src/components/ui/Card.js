import { jsx as _jsx } from "react/jsx-runtime";
export function Card({ className = "", children, ...props }) {
    return (_jsx("div", { ...props, className: `bg-[var(--surface)] rounded-[var(--radius-lg)] border border-[var(--border)] p-4 ${className}`, children: children }));
}
