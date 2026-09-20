import { Component, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { eveyFetch } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { PluginIframe } from "./PluginIframe";

interface FrontModule {
  slot: string;
  src: string;
}

interface PluginInfo {
  id: string;
  // enabled is absent from the public /api/v1/plugins response (only
  // enabled+running plugins are returned), so treat it as optional.
  // When present (legacy admin response), the filter below still applies.
  enabled?: boolean;
  frontend?: FrontModule[];
  capabilities?: string[];
  // grpc-gateway protojson emits camelCase by default (webRoot, externalFetch)
  // - accept both snake_case and camelCase to stay robust against gateway
  // configuration quirks.
  web_root?: string;
  webRoot?: string;
  external_fetch?: string[];
  externalFetch?: string[];
}

interface ListPluginsResponse {
  plugins?: PluginInfo[];
}

interface ErrorBoundaryProps {
  children: ReactNode;
  pluginId: string;
}

interface ErrorBoundaryState {
  hasError: boolean;
}

class PluginErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  state: ErrorBoundaryState = { hasError: false };

  static getDerivedStateFromError(): ErrorBoundaryState {
    return { hasError: true };
  }

  render() {
    if (this.state.hasError) return null;
    return this.props.children;
  }
}

interface PluginSlotProps {
  name: string;
  context?: Record<string, unknown>;
  // pluginId: when set, only render the plugin with this id (matching the
  // slot). Used by the home page to insert a single plugin section at the
  // exact position dictated by the customise-home ordering, instead of
  // dumping all home.section plugins as one block above the feeds.
  pluginId?: string;
  // excludeRegistered: skip plugins whose id is in this set. Used by the
  // home page to render only the "newly-installed" plugins not yet in the
  // user's feed config - the rest are rendered inline at their configured
  // position by other PluginSlot calls with pluginId set.
  excludeRegistered?: Set<string>;
}

export function PluginSlot({ name, context, pluginId, excludeRegistered }: PluginSlotProps) {
  const isAuthenticated = useAuthStore((s) => !!s.accessToken);

  // Use the same queryKey as the admin Plugins page so install/enable/
  // uninstall mutations there invalidate this list too - otherwise the
  // home page keeps rendering iframes for plugin ids that no longer exist.
  const { data, isError } = useQuery<ListPluginsResponse>({
    queryKey: ["/api/v1/plugins"],
    queryFn: () =>
      eveyFetch<ListPluginsResponse>({ url: "/api/v1/plugins", method: "GET" }),
    staleTime: 60_000,
    enabled: isAuthenticated,
  });

  const plugins = data?.plugins ?? [];
  const matches: {
    pluginId: string;
    entryPath: string;
    externalFetch: string[];
  }[] = [];

  for (const plugin of plugins) {
    // enabled is absent in the public API response (route only returns
    // enabled+running plugins), so treat undefined as true.
    if (plugin.enabled === false || !plugin.id) continue;
    if (pluginId && plugin.id !== pluginId) continue;
    if (excludeRegistered && excludeRegistered.has(plugin.id)) continue;
    const webRoot = plugin.webRoot ?? plugin.web_root ?? "";
    const externalFetch = plugin.externalFetch ?? plugin.external_fetch ?? [];
    for (const mod of plugin.frontend ?? []) {
      if (mod.slot !== name) continue;
      // Strip web_root prefix from the manifest src so the URL the iframe
      // loads is relative to the assets base. e.g. web_root="dist" + src=
      // "dist/index.js" → entry "index.js" served at /assets/index.js.
      let entry = mod.src;
      if (webRoot && webRoot !== ".") {
        const prefix = webRoot.endsWith("/") ? webRoot : webRoot + "/";
        if (entry.startsWith(prefix)) entry = entry.slice(prefix.length);
        else if (entry === webRoot) entry = "";
      }
      matches.push({
        pluginId: plugin.id,
        entryPath: entry,
        externalFetch,
      });
    }
  }

  // A failed plugin LIST must not pose as "no plugins here": render a muted
  // line instead of silently dropping every slot (failure-shown-as-empty
  // campaign). Same key as the per-module load error - zero new i18n debt.
  if (isError) return <PluginSlotError />;
  if (matches.length === 0) return null;

  return (
    <>
      {matches.map((m) => (
        <PluginErrorBoundary key={m.pluginId} pluginId={m.pluginId}>
          <PluginIframe
            pluginId={m.pluginId}
            slot={name}
            src={m.entryPath}
            externalFetch={m.externalFetch}
            context={context}
          />
        </PluginErrorBoundary>
      ))}
    </>
  );
}

function PluginSlotError() {
  const { t } = useTranslation();
  return (
    <p className="text-xs text-[var(--text-muted)] px-3 py-2" data-testid="plugin-slot-error">
      {t("admin.plugin.frontLoadError")}
    </p>
  );
}
