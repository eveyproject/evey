import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { View, ActivityIndicator, Text, StyleSheet, InteractionManager, useWindowDimensions } from "react-native";
import { WebView, type WebViewMessageEvent } from "react-native-webview";
// From "/legacy": the root's file helpers throw at runtime in
// expo-file-system 55. See downloads.ts.
import * as FileSystem from "expo-file-system/legacy";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import { eveyFetch, eveyFetchRaw, adminPluginServiceIssuePluginToken } from "@evey/api";
import { useAuthStore, toast } from "@evey/store";
import { useTheme } from "../../providers/ThemeProvider";
import { useServerUrl } from "../../hooks/useServerUrl";
import { PLUGIN_BRIDGE_SCRIPT } from "./pluginBridge";
import { FONT_SIZE } from "@evey/theme";

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

async function readBundleCache(
  pluginId: string,
  entryPath: string,
): Promise<{ code: string; etag: string | null } | null> {
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
}

async function writeBundleCache(
  pluginId: string,
  entryPath: string,
  code: string,
  etag: string | null,
): Promise<void> {
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
}

interface FrontModule {
  slot: string;
  src: string;
}

interface PluginInfo {
  id: string;
  name?: string;
  enabled?: boolean;
  frontend?: FrontModule[];
  webRoot?: string;
  externalFetch?: string[];
}

interface ListPluginsResponse {
  plugins?: PluginInfo[];
}

// Plugin frontends may request client-side navigation via postMessage. Only
// non-sensitive paths are allowed: an attacker plugin must not be able to push
// the user to /admin/* (overlay + forced nav = phishing/clickjacking vector).
// Kept in lockstep with the web PluginIframe allow-list.
const ALLOWED_NAV_PREFIXES = [
  "/home",
  "/library",
  "/media",
  "/player",
  "/settings",
  "/search",
];

export function isAllowedNavPath(path: string): boolean {
  if (!path.startsWith("/")) return false;
  if (path.startsWith("//")) return false;
  if (path.length > 2048) return false;
  return ALLOWED_NAV_PREFIXES.some((p) => path === p || path.startsWith(p + "/") || path.startsWith(p + "?"));
}

// Gates what we accept at the HTML boundary (mirrors web PluginIframe's
// pluginIdShape). The server issues `plg-<24 hex>` slugs; re-asserting here
// means a malformed pluginId can never terminate the `<base href>` attribute
// or inject markup into the WebView document.
const pluginIdShape = /^plg-[a-zA-Z0-9_-]{1,64}$/;

export interface SlotEntry {
  pluginId: string;
  /** Display name (manifest name), falling back to the id. */
  name: string;
  entryPath: string;
  externalFetch: string[];
}

/**
 * Lists the enabled+running plugins that mount a frontend module in `slot`,
 * with their display name + resolved entry path. Used by the admin dashboard
 * to render a menu row per admin.nav plugin (each linking to its own dedicated
 * screen) - mobile parity with web's usePluginSlotEntries.
 */
export function usePluginSlotEntries(slot: string): SlotEntry[] {
  const isAuthenticated = useAuthStore((s) => !!s.accessToken);
  const { data } = useQuery<ListPluginsResponse>({
    queryKey: ["/api/v1/plugins"],
    queryFn: () => eveyFetch<ListPluginsResponse>({ url: "/api/v1/plugins", method: "GET" }),
    staleTime: 60_000,
    enabled: isAuthenticated,
  });
  const entries: SlotEntry[] = [];
  for (const p of data?.plugins ?? []) {
    if (p.enabled === false || !p.id) continue;
    const webRoot = p.webRoot ?? "";
    const externalFetch = p.externalFetch ?? [];
    for (const mod of p.frontend ?? []) {
      if (mod.slot !== slot) continue;
      let entry = mod.src;
      if (webRoot && webRoot !== ".") {
        const prefix = webRoot.endsWith("/") ? webRoot : webRoot + "/";
        if (entry.startsWith(prefix)) entry = entry.slice(prefix.length);
        else if (entry === webRoot) entry = "";
      }
      entries.push({ pluginId: p.id, name: p.name || p.id, entryPath: entry, externalFetch });
    }
  }
  return entries;
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
 * WebView running the plugin's frontend bundle in isolation. The WebView
 * postMessage bridge mirrors the web iframe protocol 1:1 so plugins built
 * for web run unchanged on mobile.
 */
export function PluginSlot({ name, context, pluginId }: PluginSlotProps) {
  const isAuthenticated = useAuthStore((s) => !!s.accessToken);

  const { data, isError } = useQuery<ListPluginsResponse>({
    queryKey: ["/api/v1/plugins"],
    queryFn: () => eveyFetch<ListPluginsResponse>({ url: "/api/v1/plugins", method: "GET" }),
    staleTime: 60_000,
    enabled: isAuthenticated,
  });

  const plugins = data?.plugins ?? [];
  const matches: { pluginId: string; entryPath: string; externalFetch: string[] }[] = [];

  for (const plugin of plugins) {
    if (plugin.enabled === false || !plugin.id) continue;
    if (pluginId && plugin.id !== pluginId) continue;
    const webRoot = plugin.webRoot ?? "";
    const externalFetch = plugin.externalFetch ?? [];
    for (const mod of plugin.frontend ?? []) {
      if (mod.slot !== name) continue;
      let entry = mod.src;
      if (webRoot && webRoot !== ".") {
        const prefix = webRoot.endsWith("/") ? webRoot : webRoot + "/";
        if (entry.startsWith(prefix)) entry = entry.slice(prefix.length);
        else if (entry === webRoot) entry = "";
      }
      matches.push({ pluginId: plugin.id, entryPath: entry, externalFetch });
    }
  }

  // A failed plugin LIST must not pose as "no plugins here" - same
  // failure-shown-as-empty contract as web/TV, same i18n key as the
  // per-module load error.
  if (isError) return <PluginSlotListError />;
  if (matches.length === 0) return null;

  return (
    <>
      {matches.map((m) => (
        <PluginWebView
          slot={name}
          key={m.pluginId}
          pluginId={m.pluginId}
          entryPath={m.entryPath}
          externalFetch={m.externalFetch}
          context={context}
        />
      ))}
    </>
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

// The height the host recommends for a slot on this surface. A plugin that
// never calls evey.resize() opens at it, so every home widget lands the same
// size without knowing where it runs; a plugin with an opinion still resizes.
export function suggestedSlotHeight(slot: string, viewportWidth: number): number | undefined {
  if (slot !== "home.section") return undefined;
  return viewportWidth < 420 ? 216 : viewportWidth < 700 ? 248 : 280;
}

interface PluginWebViewProps {
  pluginId: string;
  slot: string;
  entryPath: string;
  externalFetch: string[];
  context?: Record<string, unknown>;
}

const TOAST_MAX = 3;
const TOAST_WINDOW_MS = 1000;

function PluginWebView({ pluginId, slot, entryPath, externalFetch, context }: PluginWebViewProps) {
  const { t, i18n } = useTranslation();
  const theme = useTheme();
  const accessToken = useAuthStore((s) => s.accessToken);
  const serverUrl = useServerUrl();
  const webviewRef = useRef<WebView>(null);
  const [bundleCode, setBundleCode] = useState<string | null>(null);
  const [pluginConfig, setPluginConfig] = useState<Record<string, unknown>>({});
  const { width: windowWidth } = useWindowDimensions();
  const suggestedHeight = suggestedSlotHeight(slot, windowWidth);
  const [height, setHeight] = useState(suggestedHeight ?? 120);
  const [error, setError] = useState(false);
  // Three-stage gate before we mount the WebView and download a bundle that
  // can run into 1+ MB on plugins that ship a 3D engine:
  //   1. `interactionsDone` - wait for any in-flight RN animation/transition
  //      to settle (InteractionManager).
  //   2. `inViewport` - wait until the host laid out our container (height>0
  //      means the parent scroll/list resolved our position). Keeps a plugin
  //      below the fold from running until the user scrolls there.
  //   3. `cacheHit` - if a cached bundle is on disk, skip the network
  //      altogether and feed it straight to the WebView.
  const [interactionsDone, setInteractionsDone] = useState(false);
  const [inViewport, setInViewport] = useState(false);
  const armed = interactionsDone && inViewport;
  const toastTimestamps = useRef<number[]>([]);
  const initSent = useRef(false);

  // Scoped plugin token used to proxy the WebView's prefs/public RPCs. Held by
  // the host (never exposed to the WebView JS); the server enforces the
  // plugin's granted scopes deny-by-default. Mirrors the web PluginIframe.
  const pluginTokenRef = useRef<string | null>(null);
  const ensurePluginToken = useCallback(async (): Promise<string | null> => {
    if (pluginTokenRef.current) return pluginTokenRef.current;
    try {
      const r = await adminPluginServiceIssuePluginToken(pluginId, {});
      pluginTokenRef.current = r.token ?? null;
    } catch {
      pluginTokenRef.current = null;
    }
    return pluginTokenRef.current;
  }, [pluginId]);

  const callPluginEndpoint = useCallback((method: string, args: Record<string, unknown>, token: string): Promise<Response> => {
    const enc = encodeURIComponent;
    const h = { Authorization: `Bearer ${token}` };
    switch (method) {
      case "prefs.get":
        return fetch(`${serverUrl}/api/v1/plugin-prefs/${enc(String(args.key ?? ""))}`, { headers: h });
      case "prefs.set":
        return fetch(`${serverUrl}/api/v1/plugin-prefs/${enc(String(args.key ?? ""))}`, { method: "PUT", headers: { ...h, "Content-Type": "application/json" }, body: JSON.stringify({ value: String(args.value ?? "") }) });
      case "prefs.delete":
        return fetch(`${serverUrl}/api/v1/plugin-prefs/${enc(String(args.key ?? ""))}`, { method: "DELETE", headers: h });
      case "prefs.list":
        return fetch(`${serverUrl}/api/v1/plugin-prefs`, { headers: h });
      case "media.get":
        return fetch(`${serverUrl}/api/v1/public/media/${enc(String(args.id ?? ""))}`, { headers: h });
      case "media.list":
        return fetch(`${serverUrl}/api/v1/public/media${args.libraryId ? `?libraryId=${enc(String(args.libraryId))}` : ""}`, { headers: h });
      case "media.poster":
        // The user's own session (no plugin token): the artwork handler applies
        // this user's library ACL + parental filter, so a plugin can only reach
        // posters the user can already see. Mirrors web PluginIframe's media.poster.
        return fetch(`${serverUrl}/api/v1/media/${enc(String(args.id ?? ""))}/artwork/poster`, {
          headers: accessToken ? { Authorization: `Bearer ${accessToken}` } : {},
        });
      case "assets.read": {
        // Reads one of the plugin's OWN web_root assets. Strict path guard
        // (the server also rejects traversal, this is defense in depth); the
        // cache-buster forces a fresh copy despite the immutable Cache-Control
        // on plugin assets. Mirrors web PluginIframe's assets.read.
        const p = String(args.path ?? "");
        if (!p || p.length > 256 || p.includes("..") || p.startsWith("/") || p.includes("\\") || !/^[a-zA-Z0-9._/-]+$/.test(p)) {
          return Promise.reject(new Error("invalid asset path"));
        }
        const encPath = p.split("/").map(enc).join("/");
        return eveyFetchRaw({ url: `/api/v1/plugins/${enc(pluginId)}/assets/${encPath}?_=${Date.now()}`, method: "GET", authToken: token });
      }
      default:
        return Promise.reject(new Error(`method not permitted: ${method}`));
    }
  }, [pluginId, serverUrl, accessToken]);

  const pluginRpcCall = useCallback(async (method: string, args: Record<string, unknown>): Promise<unknown> => {
    let token = await ensurePluginToken();
    if (!token) throw new Error("plugin token unavailable");
    let res = await callPluginEndpoint(method, args, token);
    if (res.status === 401) {
      pluginTokenRef.current = null;
      token = await ensurePluginToken();
      if (token) res = await callPluginEndpoint(method, args, token);
    }
    if (!res.ok) {
      let m = String(res.status);
      try { const b = (await res.json()) as { message?: string }; if (b.message) m = b.message; } catch { /* ignore */ }
      throw new Error(m);
    }
    // assets.read/media.poster return binary; hand it back as a data: URL so
    // the WebView can use it directly (data URLs work as WebGL/img sources
    // without any CORS/taint concern). Mirrors web PluginIframe's pluginRpcCall,
    // except there is no canvas in RN to downscale media.poster - the full-size
    // artwork (~1.3 MB typ.) is sent as-is; only warn past ~1.5 MB.
    if (method === "assets.read" || method === "media.poster") {
      const blob = await res.blob();
      if (method === "media.poster" && blob.size > 1_500_000 && process.env.NODE_ENV !== "production") {
        console.warn("PluginSlot media.poster: large image, no downscale on RN", blob.size);
      }
      return await new Promise<string>((resolve, reject) => {
        const fr = new FileReader();
        fr.onload = () => resolve(String(fr.result));
        fr.onerror = () => reject(new Error("asset read failed"));
        fr.readAsDataURL(blob);
      });
    }
    const text = await res.text();
    return text ? JSON.parse(text) : {};
  }, [ensurePluginToken, callPluginEndpoint]);

  useEffect(() => {
    const handle = InteractionManager.runAfterInteractions(() => setInteractionsDone(true));
    return () => handle.cancel();
  }, []);

  const handleLayout = useCallback((e: { nativeEvent: { layout: { height: number } } }) => {
    if (e.nativeEvent.layout.height > 0) setInViewport(true);
  }, []);

  // Fetch the plugin's saved config; same shape and endpoint as web.
  useEffect(() => {
    if (!accessToken || !armed) return;
    let cancelled = false;
    (async () => {
      try {
        const r = await eveyFetch<{ values?: Record<string, unknown> }>({
          url: `/api/v1/plugins/${encodeURIComponent(pluginId)}/config`,
          method: "GET",
        });
        if (!cancelled) setPluginConfig(r.values ?? {});
      } catch (err) {
        if (process.env.NODE_ENV !== "production") console.warn("PluginSlot config fetch failed", err);
      }
    })();
    return () => { cancelled = true; };
  }, [pluginId, accessToken, armed]);

  // Read disk cache first, fall back to network. Authorization header keeps
  // the plugin token out of any URL. The bundle is then inlined into the
  // WebView's HTML.
  useEffect(() => {
    if (!accessToken || !serverUrl || !armed) return;
    let cancelled = false;
    const controller = new AbortController();
    (async () => {
      // 1. Try cache - instant on warm starts.
      const cached = await readBundleCache(pluginId, entryPath);
      if (cancelled) return;
      if (cached) setBundleCode(cached.code);
      // 2. Cache miss: blocking download. Cache hit: background revalidation
      // (If-None-Match) - 304 means the cached copy is current; 200 means the
      // plugin was updated in place (same id!), so refresh both the cache and
      // the widget.
      try {
        const url = `${serverUrl}/api/v1/plugins/${encodeURIComponent(pluginId)}/assets/${entryPath}`;
        const headers: Record<string, string> = { Authorization: `Bearer ${accessToken}` };
        if (cached?.etag) headers["If-None-Match"] = cached.etag;
        const res = await fetch(url, { headers, signal: controller.signal });
        if (res.status === 304) return; // cached copy confirmed current
        if (!res.ok) throw new Error(`fetch failed: ${res.status}`);
        const text = await res.text();
        if (cancelled) return;
        setBundleCode(text);
        // Best-effort persist - cache bookkeeping (ETag extraction included)
        // must never undo a bundle that just rendered successfully.
        try {
          void writeBundleCache(pluginId, entryPath, text, res.headers.get("etag"));
        } catch {
          // cache is optional - just means a full download next launch
        }
      } catch (err: unknown) {
        if (err instanceof Error && err.name === "AbortError") return;
        // With a cached bundle already rendering, a failed revalidation
        // (offline, server restart) must not blank the widget.
        if (!cancelled && !cached) setError(true);
      }
    })();
    return () => { cancelled = true; controller.abort(); };
  }, [pluginId, entryPath, accessToken, serverUrl, armed]);

  const isLightTheme = useMemo(() => {
    const bg = theme.colors.bg.replace("#", "");
    if (bg.length < 6) return false;
    const r = parseInt(bg.slice(0, 2), 16);
    const g = parseInt(bg.slice(2, 4), 16);
    const b = parseInt(bg.slice(4, 6), 16);
    return (r * 299 + g * 587 + b * 114) / 1000 > 128;
  }, [theme.colors.bg]);

  const html = useMemo(() => {
    if (!bundleCode) return null;
    if (!pluginIdShape.test(pluginId)) return null;
    const connectSrc = externalFetch.length > 0
      ? externalFetch.map((h) => (/^https?:\/\//i.test(h) ? h : `https://${h}`)).join(" ")
      : "'none'";
    const baseHref = `${serverUrl ?? ""}/api/v1/plugins/${pluginId}/assets/`;
    const csp = [
      "default-src 'none'",
      "script-src 'unsafe-inline'",
      "style-src 'unsafe-inline'",
      `connect-src ${connectSrc}`,
      "img-src data: blob: " + (serverUrl ?? ""),
      "font-src data:",
    ].join("; ");
    const inlineSafe = bundleCode.replace(/<\/script/gi, "<\\/script");
    return `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta http-equiv="Content-Security-Policy" content="${csp}">
  <base href="${baseHref}">
  <style>
    *,*::before,*::after{box-sizing:border-box;}
    /* Force the document and body to fill the WebView. Without explicit
     * width:100%, the body collapses to the natural width of its largest
     * child, and a plugin's "width:100%" container then resolves against
     * that collapsed body - Three.js canvases ended up at ~80px wide. */
    html,body{margin:0;padding:0;width:100%;height:100%;background:transparent;color:${theme.colors.text};font-family:-apple-system,BlinkMacSystemFont,sans-serif;font-size:14px;}
  </style>
  <script>${PLUGIN_BRIDGE_SCRIPT}<\/script>
</head>
<body>
  <script>${inlineSafe}<\/script>
</body>
</html>`;
  }, [bundleCode, externalFetch, pluginId, serverUrl, theme.colors.text]);

  const handleMessage = useCallback((e: WebViewMessageEvent) => {
    let msg: Record<string, unknown>;
    try {
      msg = JSON.parse(e.nativeEvent.data);
    } catch {
      return;
    }
    const type = msg.type;
    if (type === "ready") {
      if (initSent.current) return;
      initSent.current = true;
      const initPayload = JSON.stringify({
        type: "init",
        context: context ?? {},
        config: pluginConfig,
        locale: i18n.language,
        theme: isLightTheme ? "light" : "dark",
        surface: "mobile",
        suggestedHeight,
      });
      // Forward to the WebView's window via injectJavaScript so the bridge
      // listener picks it up. Wrap in postMessage so the same handler that
      // receives parent messages on web works unchanged.
      webviewRef.current?.injectJavaScript(
        `window.postMessage(${initPayload}, "*"); true;`,
      );
      return;
    }
    if (type === "navigate" && typeof msg.path === "string" && isAllowedNavPath(msg.path)) {
      router.push(msg.path as never);
      return;
    }
    if (type === "toast" && typeof msg.message === "string") {
      const now = Date.now();
      const recent = toastTimestamps.current.filter((t) => now - t < TOAST_WINDOW_MS);
      if (recent.length >= TOAST_MAX) { toastTimestamps.current = recent; return; }
      recent.push(now);
      toastTimestamps.current = recent;
      const message = msg.message.slice(0, 200);
      const level = msg.level as "success" | "error" | "info";
      if (level === "success") toast.success(message);
      else if (level === "error") toast.error(message);
      else toast.info(message);
      return;
    }
    if (type === "resize" && typeof msg.height === "number" && msg.height > 0) {
      setHeight(Math.max(60, Math.min(800, msg.height)));
      return;
    }
    if (type === "rpc") {
      const id = msg.id;
      const method = typeof msg.method === "string" ? msg.method : "";
      const args = (msg.args as Record<string, unknown>) ?? {};
      const reply = (payload: Record<string, unknown>) =>
        webviewRef.current?.injectJavaScript(`window.postMessage(${JSON.stringify(payload)}, "*"); true;`);
      pluginRpcCall(method, args)
        .then((data) => reply({ type: "rpc-result", id, ok: true, data }))
        .catch((err: Error) => reply({ type: "rpc-result", id, ok: false, error: err.message }));
      return;
    }
  }, [context, pluginConfig, i18n.language, isLightTheme, pluginRpcCall]);

  if (!accessToken) return null;

  if (error) {
    return (
      <View style={styles.error} onLayout={handleLayout}>
        <Text style={{ color: theme.colors.textMuted, fontSize: FONT_SIZE.xs }}>
          {t("admin.plugin.frontLoadError", "Failed to load plugin")}
        </Text>
      </View>
    );
  }

  if (!html) {
    return (
      <View style={[styles.loading, { height, width: "100%" }]} onLayout={handleLayout}>
        <ActivityIndicator size="small" color={theme.colors.accent} />
      </View>
    );
  }

  return (
    <View style={{ height, width: "100%" }} onLayout={handleLayout}>
      <WebView
        ref={webviewRef}
        originWhitelist={["*"]}
        source={{ html, baseUrl: serverUrl ?? "" }}
        onMessage={handleMessage}
        javaScriptEnabled
        domStorageEnabled={false}
        scrollEnabled={false}
        showsHorizontalScrollIndicator={false}
        showsVerticalScrollIndicator={false}
        // Hardware layer + GPU process so WebGL contexts (three.js / Babylon
        // / canvas 3D) actually have a backing renderer. Without these
        // Android falls back to software rendering and the canvas is silently
        // empty - the weather plugin's 3D background was a black void.
        androidLayerType="hardware"
        // Required so https://api.open-meteo.com fetches resolve over the
        // app's https origin (mixed-content default = NEVER_ALLOW).
        mixedContentMode="always"
        // Required for animated content + plays-inline media inside the iframe.
        mediaPlaybackRequiresUserAction={false}
        allowsInlineMediaPlayback
        style={{ backgroundColor: "transparent", flex: 1 }}
        // Prevent the WebView from navigating away if a plugin link is clicked
        // - host-side router.push is the only allowed navigation surface.
        onShouldStartLoadWithRequest={(req) => {
          if (req.url === "about:blank" || req.url.startsWith("about:")) return true;
          if (req.url.startsWith(`${serverUrl ?? ""}/api/v1/plugins/`)) return true;
          return false;
        }}
        onError={() => setError(true)}
        onHttpError={() => setError(true)}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  error: {
    paddingVertical: 8,
    paddingHorizontal: 16,
  },
  loading: {
    alignItems: "center",
    justifyContent: "center",
  },
});
