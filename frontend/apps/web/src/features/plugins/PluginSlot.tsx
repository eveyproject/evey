import { Component, useEffect, useState, type ReactNode } from "react";
import { eveyFetch } from "@evey/api";
import { loadPlugin, getPluginsForSlot } from "./plugin-registry";

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
  const [ready, setReady] = useState(false);

  useEffect(() => {
    let cancelled = false;

    async function load() {
      try {
        const data = await eveyFetch<ListPluginsResponse>({
          url: "/api/v1/admin/plugins",
          method: "GET",
        });

        const plugins = data?.plugins ?? [];
        const promises: Promise<void>[] = [];

        for (const plugin of plugins) {
          if (!plugin.enabled || !plugin.id) continue;
          for (const widget of plugin.uiWidgets ?? []) {
            if (widget.slot === name) {
              promises.push(
                loadPlugin(plugin.id, widget.slot, `/api/v1/plugins/${plugin.id}/widget.js`)
              );
            }
          }
        }

        await Promise.allSettled(promises);
      } catch {
        // silently fail — plugins are optional
      }
      if (!cancelled) setReady(true);
    }

    void load();
    return () => { cancelled = true; };
  }, [name]);

  if (!ready) return null;

  const widgets = getPluginsForSlot(name);
  if (widgets.length === 0) return null;

  return (
    <>
      {widgets.map((w) => {
        const Widget = w.component;
        return (
          <PluginErrorBoundary key={w.pluginId} pluginId={w.pluginId}>
            <Widget {...(context ?? {})} />
          </PluginErrorBoundary>
        );
      })}
    </>
  );
}
