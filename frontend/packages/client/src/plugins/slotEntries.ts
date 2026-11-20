import { useQuery, type QueryClient } from "@tanstack/react-query";
import { eveyFetch, getAdminPluginServiceListPluginsQueryKey } from "@evey/api";
import { useAuthStore } from "@evey/store";

export interface PluginFrontModule {
  slot: string;
  src: string;
}

export interface PluginInfo {
  id: string;
  name?: string;
  // enabled is absent from the public /api/v1/plugins response (only
  // enabled+running plugins are returned), so treat it as optional.
  // When present (legacy admin response), the filter below still applies.
  enabled?: boolean;
  frontend?: PluginFrontModule[];
  capabilities?: string[];
  // grpc-gateway protojson emits camelCase by default (webRoot, externalFetch)
  // - accept both snake_case and camelCase to stay robust against gateway
  // configuration quirks.
  web_root?: string;
  webRoot?: string;
  external_fetch?: string[];
  externalFetch?: string[];
}

export interface ListPluginsResponse {
  plugins?: PluginInfo[];
}

export interface SlotEntry {
  pluginId: string;
  /** Display name (manifest name), falling back to the id. */
  name: string;
  /** Asset path relative to the plugin's web_root, ready for the host's src. */
  entryPath: string;
  externalFetch: string[];
}

export interface SlotFilter {
  /** Only the plugin with this id (a home row placed by the customise order). */
  pluginId?: string;
  /** Skip these plugins (the home renders them elsewhere, at their own position). */
  excludeRegistered?: Set<string>;
}

/** The query every plugin surface shares (home rows, slots, the customise panel). */
export const PLUGIN_LIST_QUERY_KEY = ["/api/v1/plugins"] as const;

/**
 * After an admin installs, removes, enables, disables or updates a plugin:
 * the admin list and every viewer surface reload. The viewer list counts as
 * fresh for a minute, so refreshing the admin list alone left the home of the
 * very tab that installed a plugin without it until a reload.
 */
export function refreshPluginLists(queryClient: QueryClient): Promise<void> {
  return Promise.all([
    queryClient.invalidateQueries({ queryKey: getAdminPluginServiceListPluginsQueryKey() }),
    queryClient.invalidateQueries({ queryKey: PLUGIN_LIST_QUERY_KEY }),
  ]).then(() => undefined);
}

export function usePluginList() {
  const isAuthenticated = useAuthStore((s) => !!s.accessToken);
  return useQuery<ListPluginsResponse>({
    queryKey: PLUGIN_LIST_QUERY_KEY,
    queryFn: () => eveyFetch<ListPluginsResponse>({ url: "/api/v1/plugins", method: "GET" }),
    staleTime: 60_000,
    enabled: isAuthenticated,
  });
}

/**
 * Strips the manifest's web_root prefix from a module src so the path is
 * relative to the assets base: web_root "dist" + src "dist/index.js" is served
 * at /assets/index.js.
 */
export function entryPathFor(src: string, webRoot: string): string {
  let entry = src;
  if (webRoot && webRoot !== ".") {
    const prefix = webRoot.endsWith("/") ? webRoot : webRoot + "/";
    if (entry.startsWith(prefix)) entry = entry.slice(prefix.length);
    else if (entry === webRoot) entry = "";
  }
  return entry;
}

/**
 * The enabled+running plugins that mount a frontend module in `slot`, with
 * their display name and resolved entry path. Pure: the three hosts (iframe,
 * WebView on the phone, WebView on the TV) used to each carry this loop.
 */
export function resolveSlotEntries(plugins: PluginInfo[] | undefined, slot: string, filter: SlotFilter = {}): SlotEntry[] {
  const entries: SlotEntry[] = [];
  for (const p of plugins ?? []) {
    if (p.enabled === false || !p.id) continue;
    if (filter.pluginId && p.id !== filter.pluginId) continue;
    if (filter.excludeRegistered?.has(p.id)) continue;
    const webRoot = p.webRoot ?? p.web_root ?? "";
    const externalFetch = p.externalFetch ?? p.external_fetch ?? [];
    for (const mod of p.frontend ?? []) {
      if (mod.slot !== slot) continue;
      entries.push({ pluginId: p.id, name: p.name || p.id, entryPath: entryPathFor(mod.src, webRoot), externalFetch });
    }
  }
  return entries;
}

/** The slot's entries as a hook - what a menu of admin.nav plugins reads. */
export function usePluginSlotEntries(slot: string): SlotEntry[] {
  const { data } = usePluginList();
  return resolveSlotEntries(data?.plugins, slot);
}
