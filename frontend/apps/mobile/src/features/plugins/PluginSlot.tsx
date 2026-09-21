import { View, Text, StyleSheet, useWindowDimensions } from "react-native";
// From "/legacy": the root's file helpers throw at runtime in
// expo-file-system 55. See downloads.ts.
import * as FileSystem from "expo-file-system/legacy";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { isAllowedNavPath, resolveSlotEntries, usePluginList, usePluginSlotEntries, type SlotEntry } from "@evey/client";
import { PluginWebView, type BundleCache } from "@evey/client/native/PluginWebView";
import { useTheme } from "../../providers/ThemeProvider";
import { useServerUrl } from "../../hooks/useServerUrl";
import { FONT_SIZE } from "@evey/theme";

// The host (WebView, bridge, RPC, protocol rules) is @evey/client's; what is
// this app's is below: the disk cache for bundles, the phone's slot height,
// its router for plugin navigation, and its theme.
export { isAllowedNavPath, usePluginSlotEntries };
export type { SlotEntry } from "@evey/client";

// Persistent cache for plugin bundles. The Weather plugin bundle is 1.3 MB
// (it ships react-three/fiber + postprocessing + three for the animated 3D
// background). Without a cache the user pays that download on every cold
// start.
//
// A plugin UPDATE keeps its id (in-place update preserves plg-… and config),
// so the cache key alone can never invalidate a stale bundle. The server's
// ETag is stored next to the bundle and every cache hit is revalidated in the
// background (stale-while-revalidate): the cached copy renders instantly, a
// conditional GET answers 304 when unchanged, and a changed bundle both
// refreshes the cache and swaps into the just-mounted widget.
const CACHE_DIR =
  ((FileSystem as unknown as { cacheDirectory?: string }).cacheDirectory ?? "") + "evey-plugins/";

function bundleCachePath(pluginId: string, entryPath: string): string {
  return `${CACHE_DIR}${pluginId}_${entryPath.replace(/\//g, "_")}`;
}

const diskBundleCache: BundleCache = {
  async read(pluginId, entryPath) {
    if (!CACHE_DIR.startsWith("file:")) return null;
    const path = bundleCachePath(pluginId, entryPath);
    try {
      const info = await FileSystem.getInfoAsync(path);
      if (!info.exists) return null;
      const code = await FileSystem.readAsStringAsync(path);
      let etag: string | null = null;
      try {
        const etagInfo = await FileSystem.getInfoAsync(path + ".etag");
        if (etagInfo.exists) etag = await FileSystem.readAsStringAsync(path + ".etag");
      } catch {
        // no stored ETag - revalidation will fetch the full body
      }
      return { code, etag };
    } catch {
      return null;
    }
  },
  async write(pluginId, entryPath, code, etag) {
    if (!CACHE_DIR.startsWith("file:")) return;
    try {
      await FileSystem.makeDirectoryAsync(CACHE_DIR, { intermediates: true });
    } catch {
      // already exists, fine
    }
    const path = bundleCachePath(pluginId, entryPath);
    try {
      await FileSystem.writeAsStringAsync(path, code);
      if (etag) await FileSystem.writeAsStringAsync(path + ".etag", etag);
    } catch {
      // disk full / cache evicted - non-fatal, just means slower next launch
    }
  },
};

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
