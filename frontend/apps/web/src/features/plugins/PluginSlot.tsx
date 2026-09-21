import { Component, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { resolveSlotEntries, usePluginList } from "@evey/client";
import { PluginIframe } from "./PluginIframe";

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
  // Same query as the admin Plugins page, so install/enable/uninstall
  // mutations there invalidate this list too - otherwise the home page keeps
  // rendering iframes for plugin ids that no longer exist.
  const { data, isError } = usePluginList();
  const matches = resolveSlotEntries(data?.plugins, name, { pluginId, excludeRegistered });

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
