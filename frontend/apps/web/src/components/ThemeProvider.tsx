import { useEffect, type ReactNode } from "react";
import { useThemeStore } from "@evey/store";
import { applyThemeToDOM } from "@evey/theme";
import { loadPluginTheme, unloadAllPluginThemes } from "@/features/plugins/theme-loader";
import { getPluginIdForTheme } from "./ThemeSelector";
import { isPluginTheme, pluginNameFromTheme } from "./theme-utils";

export { isPluginTheme, pluginNameFromTheme, pluginThemeName } from "./theme-utils";

export function ThemeProvider({ children }: { children: ReactNode }) {
  const { theme, customVars } = useThemeStore();

  useEffect(() => {
    unloadAllPluginThemes();

    if (isPluginTheme(theme)) {
      const name = pluginNameFromTheme(theme);
      document.documentElement.setAttribute("data-theme", name);
      const pluginId = getPluginIdForTheme(name);
      if (pluginId) {
        loadPluginTheme(pluginId);
      }
      const link = document.getElementById("theme-link") as HTMLLinkElement | null;
      if (link) link.href = "";
    } else {
      document.documentElement.removeAttribute("data-theme");
      applyThemeToDOM(theme, customVars ?? undefined);
    }
  }, [theme, customVars]);

  return <>{children}</>;
}
