import { useEffect, type ReactNode } from "react";
import { useThemeStore } from "@evey/store";
import { applyThemeToDOM } from "@evey/theme";
import { eveyFetch } from "@evey/api";
import { loadPluginTheme, unloadAllPluginThemes } from "@/features/plugins/theme-loader";
import { isPluginTheme, pluginNameFromTheme } from "./theme-utils";

export { isPluginTheme, pluginNameFromTheme, pluginThemeName } from "./theme-utils";

interface PluginInfo {
  id: string;
}

interface ListPluginsResponse {
  plugins?: PluginInfo[];
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const { theme, pluginId, customVars, setTheme } = useThemeStore();

  // Reset theme if the active plugin was uninstalled
  useEffect(() => {
    if (!pluginId) return;
    let ignore = false;
    const checkPlugin = async () => {
      try {
        const res = await eveyFetch<ListPluginsResponse>({ url: "/api/v1/plugins", method: "GET" });
        const installed = res.plugins ?? [];
        if (!ignore && !installed.some((p) => p.id === pluginId)) {
          setTheme("default");
        }
      } catch {
        // non-admin or network error
      }
    };
    void checkPlugin();
    return () => { ignore = true; };
  }, [pluginId, setTheme]);

  useEffect(() => {
    unloadAllPluginThemes();

    if (isPluginTheme(theme)) {
      const name = pluginNameFromTheme(theme);
      document.documentElement.setAttribute("data-theme", name);
      if (pluginId) {
        loadPluginTheme(pluginId);
      }
      const link = document.getElementById("theme-link") as HTMLLinkElement | null;
      if (link) link.href = "";
    } else {
      document.documentElement.removeAttribute("data-theme");
      applyThemeToDOM(theme, customVars ?? undefined);
    }
  }, [theme, pluginId, customVars]);

  return <>{children}</>;
}
