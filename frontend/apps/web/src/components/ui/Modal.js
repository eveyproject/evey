import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { useEffect, useRef } from "react";
export function Modal({ open, onClose, title, children }) {
    const backdropRef = useRef(null);
    useEffect(() => {
        if (!open)
            return;
        const handler = (e) => {
            if (e.key === "Escape")
                onClose();
        };
        window.addEventListener("keydown", handler);
        return () => window.removeEventListener("keydown", handler);
    }, [open, onClose]);
    if (!open)
        return null;
    return (_jsx("div", { ref: backdropRef, className: "fixed inset-0 z-50 flex items-center justify-center bg-black/60", onClick: (e) => {
            if (e.target === backdropRef.current)
                onClose();
        }, children: _jsxs("div", { role: "dialog", "aria-modal": "true", "aria-labelledby": title ? "modal-title" : undefined, className: "bg-[var(--surface)] rounded-[var(--radius-lg)] border border-[var(--border)] p-6 w-full max-w-md shadow-xl", children: [title && (_jsx("h2", { id: "modal-title", className: "text-lg font-semibold text-[var(--text)] mb-4", children: title })), children] }) }));
}
