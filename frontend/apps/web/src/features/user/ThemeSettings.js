import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { ThemeSelector } from "@/components/ThemeSelector";
export function ThemeSettings() {
    return (_jsxs("div", { className: "flex flex-col gap-2", children: [_jsx("h3", { className: "text-sm font-semibold text-[var(--text)]", children: "Appearance" }), _jsx("p", { className: "text-xs text-[var(--text-muted)]", children: "Choose a color theme for the interface." }), _jsx(ThemeSelector, {})] }));
}
