import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { View, ActivityIndicator, Text, StyleSheet, InteractionManager } from "react-native";
import { WebView, type WebViewMessageEvent } from "react-native-webview";
import * as FileSystem from "expo-file-system";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";
import { useAuthStore, toast } from "@evey/store";
import { useTheme } from "../../providers/ThemeProvider";
import { useServerUrl } from "../../hooks/useServerUrl";
import { PLUGIN_BRIDGE_SCRIPT } from "./pluginBridge";

// Persistent cache for plugin bundles. The Weather plugin bundle is 1.3 MB
// (it ships react-three/fiber + postprocessing + three for the animated 3D
// background). Without a cache the user pays that download on every cold
// start. The cache key includes the plugin id (which changes on update) so
// new bundles invalidate cleanly.
const CACHE_DIR =
  ((FileSystem as unknown as { cacheDirectory?: string }).cacheDirectory ?? "") + "evey-plugins/";

async function readBundleCache(pluginId: string, entryPath: string): Promise<string | null> {
  if (!CACHE_DIR.startsWith("file:")) return null;
  const path = `${CACHE_DIR}${pluginId}_${entryPath.replace(/\//g, "_")}`;
  try {
    const info = await FileSystem.getInfoAsync(path);
    if (!info.exists) return null;
    return await FileSystem.readAsStringAsync(path);
  } catch {
    return null;
  }
}

async function writeBundleCache(pluginId: string, entryPath: string, code: string): Promise<void> {
  if (!CACHE_DIR.startsWith("file:")) return;
  try {
    await FileSystem.makeDirectoryAsync(CACHE_DIR, { intermediates: true });
  } catch {
    // already exists, fine
  }
  const path = `${CACHE_DIR}${pluginId}_${entryPath.replace(/\//g, "_")}`;
  try {
    await FileSystem.writeAsStringAsync(path, code);
  } catch {
    // disk full / cache evicted — non-fatal, just means slower next launch
  }
}

interface FrontModule {
  slot: string;
  src: string;
}

interface PluginInfo {
  id: string;
  enabled?: boolean;
  frontend?: FrontModule[];
  webRoot?: string;
  externalFetch?: string[];
}

interface ListPluginsResponse {
  plugins?: PluginInfo[];
}

const ALLOWED_NAV_PREFIXES = [
  "/home",
  "/library",
  "/media",
  "/player",
  "/settings",
  "/search",
];

function isAllowedNavPath(path: string): boolean {
  if (!path.startsWith("/")) return false;
  if (path.startsWith("//")) return false;
  if (path.length > 2048) return false;
  return ALLOWED_NAV_PREFIXES.some((p) => path === p || path.startsWith(p + "/") || path.startsWith(p + "?"));
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

  const { data } = useQuery<ListPluginsResponse>({
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

  if (matches.length === 0) return null;

  return (
    <>
      {matches.map((m) => (
        <PluginWebView
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

interface PluginWebViewProps {
  pluginId: string;
  entryPath: string;
  externalFetch: string[];
  context?: Record<string, unknown>;
}

const TOAST_MAX = 3;
const TOAST_WINDOW_MS = 1000;

function PluginWebView({ pluginId, entryPath, externalFetch, context }: PluginWebViewProps) {
  const { t, i18n } = useTranslation();
  const theme = useTheme();
  const accessToken = useAuthStore((s) => s.accessToken);
  const serverUrl = useServerUrl();
  const webviewRef = useRef<WebView>(null);
  const [bundleCode, setBundleCode] = useState<string | null>(null);
  const [pluginConfig, setPluginConfig] = useState<Record<string, unknown>>({});
  const [height, setHeight] = useState(120);
  const [error, setError] = useState(false);
  // Three-stage gate before we mount the WebView and download a bundle that
  // can run into 1+ MB on plugins that ship a 3D engine:
  //   1. `interactionsDone` — wait for any in-flight RN animation/transition
  //      to settle (InteractionManager).
  //   2. `inViewport` — wait until the host laid out our container (height>0
  //      means the parent scroll/list resolved our position). Keeps a plugin
  //      below the fold from running until the user scrolls there.
  //   3. `cacheHit` — if a cached bundle is on disk, skip the network
  //      altogether and feed it straight to the WebView.
  const [interactionsDone, setInteractionsDone] = useState(false);
  const [inViewport, setInViewport] = useState(false);
  const armed = interactionsDone && inViewport;
  const toastTimestamps = useRef<number[]>([]);
  const initSent = useRef(false);

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
    eveyFetch<{ values?: Record<string, unknown> }>({
      url: `/api/v1/plugins/${encodeURIComponent(pluginId)}/config`,
      method: "GET",
    })
      .then((r) => { if (!cancelled) setPluginConfig(r.values ?? {}); })
      .catch(() => {});
    return () => { cancelled = true; };
  }, [pluginId, accessToken, armed]);

  // Read disk cache first, fall back to network. Authorization header keeps
  // the plugin token out of any URL. The bundle is then inlined into the
  // WebView's HTML.
  useEffect(() => {
    if (!accessToken || !serverUrl || !armed) return;
    let cancelled = false;
    (async () => {
      // 1. Try cache — instant on warm starts.
      const cached = await readBundleCache(pluginId, entryPath);
      if (cancelled) return;
      if (cached) {
        setBundleCode(cached);
        return;
      }
      // 2. Cache miss — download and persist for next time.
      try {
        const url = `${serverUrl}/api/v1/plugins/${encodeURIComponent(pluginId)}/assets/${entryPath}`;
        const res = await fetch(url, {
          headers: { Authorization: `Bearer ${accessToken}` },
        });
        if (!res.ok) throw new Error(`fetch failed: ${res.status}`);
        const text = await res.text();
        if (cancelled) return;
        setBundleCode(text);
        // Best-effort write — don't await, don't fail the render path.
        void writeBundleCache(pluginId, entryPath, text);
      } catch {
        if (!cancelled) setError(true);
      }
    })();
    return () => { cancelled = true; };
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
    html,body{margin:0;padding:0;background:transparent;color:${theme.colors.text};font-family:-apple-system,BlinkMacSystemFont,sans-serif;font-size:14px;}
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
  }, [context, pluginConfig, i18n.language, isLightTheme]);

  if (!accessToken) return null;

  if (error) {
    return (
      <View style={styles.error} onLayout={handleLayout}>
        <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>
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
        // empty — the weather plugin's 3D background was a black void.
        androidLayerType="hardware"
        // Required so https://api.open-meteo.com fetches resolve over the
        // app's https origin (mixed-content default = NEVER_ALLOW).
        mixedContentMode="always"
        // Required for animated content + plays-inline media inside the iframe.
        mediaPlaybackRequiresUserAction={false}
        allowsInlineMediaPlayback
        style={{ backgroundColor: "transparent", flex: 1 }}
        // Prevent the WebView from navigating away if a plugin link is clicked
        // — host-side router.push is the only allowed navigation surface.
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
