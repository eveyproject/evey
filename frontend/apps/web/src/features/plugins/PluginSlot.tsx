import { Component, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { PluginIframe } from "./PluginIframe";

interface FrontModule {
  slot: string;
  src: string;
}

interface PluginInfo {
  id: string;
  enabled: boolean;
  frontend?: FrontModule[];
  capabilities?: string[];
  // grpc-gateway protojson emits camelCase by default (webRoot, externalFetch)
  // — accept both snake_case and camelCase to stay robust against gateway
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
}

export function PluginSlot({ name, context }: PluginSlotProps) {
  const isAuthenticated = useAuthStore((s) => !!s.accessToken);

  // Use the same queryKey as the admin Plugins page so install/enable/
  // uninstall mutations there invalidate this list too — otherwise the
  // home page keeps rendering iframes for plugin ids that no longer exist.
  const { data } = useQuery<ListPluginsResponse>({
    queryKey: ["/api/v1/admin/plugins"],
    queryFn: () =>
      eveyFetch<ListPluginsResponse>({ url: "/api/v1/admin/plugins", method: "GET" }),
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
    if (!plugin.enabled || !plugin.id) continue;
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

  if (matches.length === 0) return null;

  return (
    <>
      {matches.map((m) => (
        <PluginErrorBoundary key={m.pluginId} pluginId={m.pluginId}>
          <PluginIframe
            pluginId={m.pluginId}
            src={m.entryPath}
            externalFetch={m.externalFetch}
            context={context}
          />
        </PluginErrorBoundary>
      ))}
    </>
  );
}
