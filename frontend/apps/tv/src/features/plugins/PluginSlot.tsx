import { View, Text, StyleSheet, useWindowDimensions } from "react-native";
import { useTranslation } from "react-i18next";
import { resolveSlotEntries, usePluginList, usePluginSlotEntries, type SlotEntry } from "@evey/client";
import { PluginWebView } from "@evey/client/native/PluginWebView";
import { useTheme } from "../../providers/ThemeProvider";
import { useServerUrl } from "../../hooks/useServerUrl";
import { homeWidgetBudget } from "../../lib/tvLayout";
import { FONT_SIZE } from "@evey/theme";

// The host (WebView, bridge, RPC, protocol rules) is @evey/client's; what is
// this app's is below: the television's slot budget, its focus rule, and
// the fact that plugin navigation is not wired on this surface yet.
export { usePluginSlotEntries };
export type { SlotEntry } from "@evey/client";

// The height the host recommends for a slot on this surface: the home's
// widget budget, so every home widget lands the same size without knowing
// where it runs; a plugin with an opinion still resizes.
export function suggestedSlotHeight(slot: string, panelHeight: number): number | undefined {
  if (slot !== "home.section") return undefined;
  return homeWidgetBudget(panelHeight);
}

interface PluginSlotProps {
  /** Slot name declared in the plugin manifest (e.g. "home.section"). */
  name: string;
  /** Per-slot context forwarded as the init payload `context`. */
  context?: Record<string, unknown>;
  /** When set, only render the plugin with this id matching the slot. */
  pluginId?: string;
  /** Skip these plugins (the home renders them elsewhere, at their own position). */
  excludeRegistered?: Set<string>;
}

/**
 * TV parity with web's PluginSlot (ADR 0021, phase 1). Each matching plugin
 * gets its own WebView running the plugin's frontend bundle in isolation.
 * Slots are display-only here: a WebView does not take part in D-pad focus.
 */
export function PluginSlot({ name, context, pluginId, excludeRegistered }: PluginSlotProps) {
  const { data, isError } = usePluginList();
  const matches = resolveSlotEntries(data?.plugins, name, { pluginId, excludeRegistered });

  // A failed plugin LIST must not pose as "no plugins here" - same
  // failure-shown-as-empty contract as web/mobile, same i18n key as the
  // per-module load error.
  if (isError) return <PluginSlotError />;
  if (matches.length === 0) return null;

  return (
    <>
      {matches.map((m) => (
        <PluginSlotEntry key={m.pluginId} slot={name} entry={m} context={context} />
      ))}
    </>
  );
}

// One mounted plugin. The hooks the host needs (panel, theme, server) run
// here, not in PluginSlot: a slot with no plugin must cost the screen nothing.
// TV routes are simpler than web/mobile's nav surface; only home is wired to
// plugin slots today, so plugin-triggered navigation isn't supported yet -
// dropped (no onNavigate) rather than guessing an unsafe destination.
function PluginSlotEntry({ slot, entry, context }: { slot: string; entry: SlotEntry; context?: Record<string, unknown> }) {
  const theme = useTheme();
  const serverUrl = useServerUrl();
  const { height: windowHeight } = useWindowDimensions();
  return (
    <PluginWebView
      pluginId={entry.pluginId}
      slot={slot}
      entryPath={entry.entryPath}
      externalFetch={entry.externalFetch}
      context={context}
      surface="tv"
      serverUrl={serverUrl}
      theme={{ text: theme.colors.text, textMuted: theme.colors.textMuted, accent: theme.colors.accent, bg: theme.colors.bg }}
      suggestedHeight={suggestedSlotHeight(slot, windowHeight)}
      viewportHeight={Math.round(windowHeight)}
      focusable={false}
    />
  );
}

function PluginSlotError() {
  const { t } = useTranslation();
  const theme = useTheme();
  return (
    <View style={styles.error}>
      <Text style={{ color: theme.colors.textMuted, fontSize: FONT_SIZE.xs }}>
        {t("admin.plugin.frontLoadError", "Failed to load plugin")}
      </Text>
    </View>
  );
}

const styles = StyleSheet.create({
  error: {
    paddingVertical: 8,
    paddingHorizontal: 16,
  },
});
