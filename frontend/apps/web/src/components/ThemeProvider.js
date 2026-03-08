import { Fragment as _Fragment, jsx as _jsx } from "react/jsx-runtime";
import { useEffect } from "react";
import { useThemeStore } from "@evey/store";
import { applyThemeToDOM } from "@evey/theme";
export function ThemeProvider({ children }) {
    const { theme, customVars } = useThemeStore();
    useEffect(() => {
        applyThemeToDOM(theme, customVars ?? undefined);
    }, [theme, customVars]);
    return _jsx(_Fragment, { children: children });
}
