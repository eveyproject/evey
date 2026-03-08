import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { useThemeStore } from "@evey/store";
import { themes } from "@evey/theme";
export function ThemeSelector() {
    const { theme, setTheme } = useThemeStore();
    return (_jsxs("div", { className: "flex flex-col gap-2", children: [_jsx("label", { className: "text-sm font-medium text-[var(--text-muted)]", children: "Theme" }), _jsx("div", { className: "flex gap-2", children: Object.keys(themes).map((name) => (_jsx("button", { onClick: () => setTheme(name), className: `px-3 py-1.5 rounded-[var(--radius-md)] text-sm capitalize transition-colors ${theme === name
                        ? "bg-[var(--accent)] text-white"
                        : "bg-[var(--surface-raised)] text-[var(--text-muted)] hover:text-[var(--text)]"}`, children: name }, name))) })] }));
}
