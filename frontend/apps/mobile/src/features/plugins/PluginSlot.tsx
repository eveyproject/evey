import { View, Text, StyleSheet, useWindowDimensions } from "react-native";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { isAllowedNavPath, resolveSlotEntries, usePluginList, usePluginSlotEntries, type SlotEntry } from "@evey/client";
import { PluginWebView } from "@evey/client/native/PluginWebView";
import { diskBundleCache } from "@evey/client/native/pluginBundleCache";
import { useTheme } from "../../providers/ThemeProvider";
import { useServerUrl } from "../../hooks/useServerUrl";
import { FONT_SIZE } from "@evey/theme";

// The host (WebView, bridge, RPC, protocol rules) is @evey/client's; what is
// this app's is below: the phone's slot height,
// its router for plugin navigation, and its theme.
export { isAllowedNavPath, usePluginSlotEntries };
export type { SlotEntry } from "@evey/client";

// The height the host recommends for a slot on this surface. A plugin that
// never calls evey.resize() opens at it, so every home widget lands the same
// size without knowing where it runs; a plugin with an opinion still resizes.
export function suggestedSlotHeight(slot: string, viewportWidth: number): number | undefined {
  if (slot !== "home.section") return undefined;
  return viewportWidth < 420 ? 216 : viewportWidth < 700 ? 248 : 280;
}

interface PluginSlotProps {
  /** Slot name declared in the plugin manifest (e.g. "home.section"). */
  name: string;
  /** Per-slot context forwarded as the init payload `context`. */
  context?: Record<string, unknown>;
  /** When set, only render the plugin with this id matching the slot. */
  pluginId?: string;
}

/**
 * Mobile parity with web's PluginSlot. Each matching plugin gets its own
 * WebView running the plugin's frontend bundle in isolation.
 */
export function PluginSlot({ name, context, pluginId }: PluginSlotProps) {
  const { data, isError } = usePluginList();
  const matches = resolveSlotEntries(data?.plugins, name, { pluginId });

  // A failed plugin LIST must not pose as "no plugins here" - same
  // failure-shown-as-empty contract as web/TV, same i18n key as the
  // per-module load error.
  if (isError) return <PluginSlotListError />;
  if (matches.length === 0) return null;

  return (
    <>
      {matches.map((m) => (
        <PluginSlotEntry key={m.pluginId} slot={name} entry={m} context={context} />
      ))}
    </>
  );
}

// One mounted plugin. The hooks the host needs (viewport, theme, server) run
// here, not in PluginSlot: a slot with no plugin must cost the screen nothing.
function PluginSlotEntry({ slot, entry, context }: { slot: string; entry: SlotEntry; context?: Record<string, unknown> }) {
  const theme = useTheme();
  const serverUrl = useServerUrl();
  const { width: windowWidth } = useWindowDimensions();
  return (
    <PluginWebView
      pluginId={entry.pluginId}
      slot={slot}
      entryPath={entry.entryPath}
      externalFetch={entry.externalFetch}
      context={context}
      surface="mobile"
      serverUrl={serverUrl}
      theme={{ text: theme.colors.text, textMuted: theme.colors.textMuted, accent: theme.colors.accent, bg: theme.colors.bg }}
      suggestedHeight={suggestedSlotHeight(slot, windowWidth)}
      bundleCache={diskBundleCache}
      onNavigate={(path) => {
        if (isAllowedNavPath(path)) router.push(path as never);
      }}
    />
  );
}

function PluginSlotListError() {
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
