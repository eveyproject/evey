import { useEffect, useRef } from "react";
import { useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
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
  /** Granted permissions the host enforces itself (media.trash). */
  hostPermissions?: string[];
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
  /** Granted permissions the host enforces itself: media.trash offers evey.media.trash. */
  hostPermissions: string[];
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

/** How often the admin list is asked again while a plugin is still starting. */
export const PLUGIN_SETTLING_POLL_MS = 2000;

interface PluginState {
  id?: string;
  state?: string;
}

/**
 * The admin list's refetch interval: a plugin still starting is asked about
 * again until it runs or fails. The list was only read right after the admin's
 * action, so a plugin slower than that kept "Starting" until a reload.
 */
export function pluginSettlingInterval(plugins: ReadonlyArray<PluginState> | undefined): number | false {
  return (plugins ?? []).some((p) => p.state === "PLUGIN_STATE_INITIALIZING") ? PLUGIN_SETTLING_POLL_MS : false;
}

function pluginStatesSignature(plugins: ReadonlyArray<PluginState>): string {
  return plugins.map((p) => `${p.id ?? ""}:${p.state ?? ""}`).sort().join("|");
}

/**
 * Reloads the viewer list when a plugin of the admin list changes state. Only
 * running plugins are in the viewer list, so the slots of one that has just
 * finished starting (its own config panel among them) appear without a reload.
 * Pass the list as fetched: undefined while it loads.
 */
export function useViewerListFollowsPluginStates(plugins: ReadonlyArray<PluginState> | undefined): void {
  const queryClient = useQueryClient();
  const signature = plugins === undefined ? null : pluginStatesSignature(plugins);
  const seen = useRef<string | null>(null);
  useEffect(() => {
    if (signature === null) return;
    if (seen.current !== null && seen.current !== signature) {
      void queryClient.invalidateQueries({ queryKey: PLUGIN_LIST_QUERY_KEY });
    }
    seen.current = signature;
  }, [signature, queryClient]);
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
      entries.push({ pluginId: p.id, name: p.name || p.id, entryPath: entryPathFor(mod.src, webRoot), externalFetch, hostPermissions: p.hostPermissions ?? [] });
    }
  }
  return entries;
}

/** The slot's entries as a hook - what a menu of admin.nav plugins reads. */
export function usePluginSlotEntries(slot: string): SlotEntry[] {
  const { data } = usePluginList();
  return resolveSlotEntries(data?.plugins, slot);
}
