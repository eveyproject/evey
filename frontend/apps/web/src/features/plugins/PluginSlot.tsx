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
  const matches: { pluginId: string; frontUrl: string }[] = [];

  for (const plugin of plugins) {
    if (!plugin.enabled || !plugin.id) continue;
    for (const mod of plugin.frontend ?? []) {
      if (mod.slot === name) {
        matches.push({
          pluginId: plugin.id,
          frontUrl: `/api/v1/plugins/${plugin.id}/front.js`,
        });
      }
    }
  }

  if (matches.length === 0) return null;

  return (
    <>
      {matches.map((m) => (
        <PluginErrorBoundary key={m.pluginId} pluginId={m.pluginId}>
          <PluginIframe pluginId={m.pluginId} src={m.frontUrl} context={context} />
        </PluginErrorBoundary>
      ))}
    </>
  );
}
