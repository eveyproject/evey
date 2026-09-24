// From "/legacy": the root's file helpers throw at runtime in
// expo-file-system 55 (see apps/mobile downloads.ts).
import * as FileSystem from "expo-file-system/legacy";
import type { BundleCache } from "./PluginWebView";

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

export const diskBundleCache: BundleCache = {
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
