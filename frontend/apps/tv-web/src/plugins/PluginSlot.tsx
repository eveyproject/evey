import { Component, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { homeWidgetBudget, resolveSlotEntries, usePluginList } from "@evey/client";
import { PluginIframe } from "@evey/client/web";
import { navigate, parseHash } from "../app/router";
import { dpPx } from "../lib/uiScale";

class PluginErrorBoundary extends Component<{ children: ReactNode }, { hasError: boolean }> {
  state = { hasError: false };
  static getDerivedStateFromError() { return { hasError: true }; }
  render() { return this.state.hasError ? null : this.props.children; }
}

/**
 * A plugin's module on this screen, in the browser app's iframe host. A plugin
 * asking to navigate goes through the hash router here.
 */
export function PluginSlot({ name, context, pluginId, excludeRegistered }: {
  name: string;
  context?: Record<string, unknown>;
  pluginId?: string;
  excludeRegistered?: Set<string>;
}) {
  const { t } = useTranslation();
  const { data, isError } = usePluginList();
  const matches = resolveSlotEntries(data?.plugins, name, { pluginId, excludeRegistered });

  // A failed plugin list must not pose as "no plugins here".
  if (isError) return <p className="muted">{t("admin.plugin.frontLoadError")}</p>;
  if (matches.length === 0) return null;

  // Laid out as on the Android television: the TV surface, one plugin pixel
  // per dp, and the home widget given the room that keeps the first row whole.
  const scale = dpPx();
  const suggestedHeight = name === "home.section" ? homeWidgetBudget(window.innerHeight / scale) : undefined;

  return (
    <>
      {matches.map((m) => (
        <PluginErrorBoundary key={m.pluginId}>
          <PluginIframe
            pluginId={m.pluginId}
            slot={name}
            src={m.entryPath}
            externalFetch={m.externalFetch}
            context={context}
            onNavigate={(path) => navigate(parseHash(`#${path}`))}
            surface="tv"
            suggestedHeight={suggestedHeight}
            pixelScale={scale}
          />
        </PluginErrorBoundary>
      ))}
    </>
  );
}
