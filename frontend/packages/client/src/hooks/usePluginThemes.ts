import { useQuery } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";
import type { PluginThemeInfo } from "../lib/pluginTheme";

/**
 * The themes the installed plugins ship, with their sheets. Off by default
 * where nothing needs them (a built-in theme in use), so a screen that only
 * paints never asks the server.
 */
export function usePluginThemes(enabled = true) {
  return useQuery<PluginThemeInfo[]>({
    queryKey: ["plugin-themes"],
    queryFn: async () => {
      const data = await eveyFetch<{ themes: PluginThemeInfo[] }>({
        url: "/api/v1/plugins/themes",
        method: "GET",
      });
      return data.themes ?? [];
    },
    enabled,
    staleTime: 60_000,
  });
}
