import { Component, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { PluginIframe } from "./PluginIframe";

interface UIWidget {
  slot: string;
  src: string;
}

interface PluginInfo {
  id: string;
  enabled: boolean;
  uiWidgets?: UIWidget[];
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

  const { data } = useQuery<ListPluginsResponse>({
    queryKey: ["plugins", "list"],
    queryFn: () =>
      eveyFetch<ListPluginsResponse>({ url: "/api/v1/admin/plugins", method: "GET" }),
    staleTime: 60_000,
    enabled: isAuthenticated,
  });

  const plugins = data?.plugins ?? [];
  const matches: { pluginId: string; widgetUrl: string }[] = [];

  for (const plugin of plugins) {
    if (!plugin.enabled || !plugin.id) continue;
    for (const widget of plugin.uiWidgets ?? []) {
      if (widget.slot === name) {
        matches.push({
          pluginId: plugin.id,
          widgetUrl: `/api/v1/plugins/${plugin.id}/widget.js`,
        });
      }
    }
  }

  if (matches.length === 0) return null;

  return (
    <>
      {matches.map((m) => (
        <PluginErrorBoundary key={m.pluginId} pluginId={m.pluginId}>
          <PluginIframe pluginId={m.pluginId} src={m.widgetUrl} context={context} />
        </PluginErrorBoundary>
      ))}
    </>
  );
}
