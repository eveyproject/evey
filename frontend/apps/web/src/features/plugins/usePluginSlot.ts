import { useQuery } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";
import { useAuthStore } from "@evey/store";

interface FrontModule {
  slot: string;
  src: string;
}

interface PluginInfo {
  id: string;
  name?: string;
  enabled?: boolean;
  frontend?: FrontModule[];
  // grpc-gateway protojson emits camelCase by default; accept both forms.
  web_root?: string;
  webRoot?: string;
  external_fetch?: string[];
  externalFetch?: string[];
}

interface ListPluginsResponse {
  plugins?: PluginInfo[];
}

export interface SlotEntry {
  pluginId: string;
  /** Display name (manifest name), falling back to the id. */
  name: string;
  /** Asset path relative to the plugin's web_root, ready for the iframe src. */
  entryPath: string;
  externalFetch: string[];
}

/**
 * Lists the enabled+running plugins that mount a frontend module in `slot`,
 * resolving each module's entry path (web_root-stripped) and display name.
 * Shares the `["/api/v1/plugins"]` query key with PluginSlot + the admin
 * Plugins page so install/enable/uninstall mutations invalidate it too.
 */
export function usePluginSlotEntries(slot: string): SlotEntry[] {
  const isAuthenticated = useAuthStore((s) => !!s.accessToken);

  const { data } = useQuery<ListPluginsResponse>({
    queryKey: ["/api/v1/plugins"],
    queryFn: () =>
      eveyFetch<ListPluginsResponse>({ url: "/api/v1/plugins", method: "GET" }),
    staleTime: 60_000,
    enabled: isAuthenticated,
  });

  const entries: SlotEntry[] = [];
  for (const p of data?.plugins ?? []) {
    if (p.enabled === false || !p.id) continue;
    const webRoot = p.webRoot ?? p.web_root ?? "";
    const externalFetch = p.externalFetch ?? p.external_fetch ?? [];
    for (const mod of p.frontend ?? []) {
      if (mod.slot !== slot) continue;
      let entry = mod.src;
      if (webRoot && webRoot !== ".") {
        const prefix = webRoot.endsWith("/") ? webRoot : webRoot + "/";
        if (entry.startsWith(prefix)) entry = entry.slice(prefix.length);
        else if (entry === webRoot) entry = "";
      }
      entries.push({ pluginId: p.id, name: p.name || p.id, entryPath: entry, externalFetch });
    }
  }
  return entries;
}
