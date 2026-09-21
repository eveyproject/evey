import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { View, ActivityIndicator, Text, StyleSheet, InteractionManager } from "react-native";
import { WebView, type WebViewMessageEvent } from "react-native-webview";
import { useTranslation } from "react-i18next";
import { eveyFetch, eveyFetchRaw } from "@evey/api";
import { useAuthStore, toast } from "@evey/store";
import { BRIDGE_SCRIPT } from "../plugins/bridgeScript";
import {
  buildConnectSrc,
  buildInitPayload,
  clampSlotHeight,
  createToastLimiter,
  inlineScriptSafe,
  isLightBackground,
  isValidPluginId,
  parseHostMessage,
} from "../plugins/hostProtocol";
import { createPluginRpc } from "../plugins/pluginRpc";
import { clientLogger } from "../log";

const log = clientLogger("plugin");

/** A persistent store for downloaded bundles; the phone has one, the TV does not. */
export interface BundleCache {
  read(pluginId: string, entryPath: string): Promise<{ code: string; etag: string | null } | null>;
  write(pluginId: string, entryPath: string, code: string, etag: string | null): Promise<void>;
}

export interface PluginWebViewTheme {
  text: string;
  textMuted: string;
  accent: string;
  bg: string;
}

export interface PluginWebViewProps {
  pluginId: string;
  slot: string;
  entryPath: string;
  externalFetch: string[];
  context?: Record<string, unknown>;
  /** "mobile" or "tv": a plugin lays itself out for the surface it runs on. */
  surface: string;
  serverUrl: string;
  theme: PluginWebViewTheme;
  /** The height the host recommends for this slot; a plugin that never resizes opens at it. */
  suggestedHeight?: number;
  /** The panel height, when the surface tells plugins about it (TV). */
  viewportHeight?: number;
  /** Disk cache for the bundle (stale-while-revalidate on the ETag); none = network every mount. */
  bundleCache?: BundleCache;
  /** Where a plugin's evey.navigate() may take the user; absent = navigation is dropped. */
  onNavigate?: (path: string) => void;
  /** Keep the slot out of the D-pad focus order (TV). */
  focusable?: boolean;
}

/**
 * A plugin's frontend bundle in its own WebView, on the phone and on the
 * television. The postMessage bridge mirrors the web iframe protocol 1:1 so
 * plugins built for web run unchanged here; the rules of that protocol (what
 * a plugin may do, how far it may grow, how often it may toast, which
 * endpoints it may reach) come from the shared host modules.
 */
export function PluginWebView({
  pluginId,
  entryPath,
  externalFetch,
  context,
  surface,
  serverUrl,
  theme,
  suggestedHeight,
  viewportHeight,
  bundleCache,
  onNavigate,
  focusable,
}: PluginWebViewProps) {
  const { t, i18n } = useTranslation();
  const accessToken = useAuthStore((s) => s.accessToken);
  const webviewRef = useRef<WebView>(null);
  const [bundleCode, setBundleCode] = useState<string | null>(null);
  const [pluginConfig, setPluginConfig] = useState<Record<string, unknown>>({});
  const [height, setHeight] = useState(suggestedHeight ?? 120);
  const [error, setError] = useState(false);
  // Three-stage gate before we mount the WebView and download a bundle that
  // can run into 1+ MB on plugins that ship a 3D engine:
  //   1. `interactionsDone` - wait for any in-flight RN animation/transition
  //      to settle (InteractionManager).
  //   2. `inViewport` - wait until the host laid out our container (height>0
  //      means the parent scroll/list resolved our position). Keeps a plugin
  //      below the fold from running until the user scrolls there.
  //   3. the cache - if a cached bundle is on disk, skip the network
  //      altogether and feed it straight to the WebView.
  const [interactionsDone, setInteractionsDone] = useState(false);
  const [inViewport, setInViewport] = useState(false);
  const armed = interactionsDone && inViewport;
  const initSent = useRef(false);

  // The toast cap and the RPC endpoint (scoped token, endpoint allowlist)
  // are the shared host rules; only the transport is this surface's: plain
  // fetch against the server URL, the plugin token as a Bearer header.
  const limitedToast = useMemo(() => createToastLimiter(toast), []);
  const rpc = useMemo(
    () =>
      createPluginRpc(pluginId, {
        request: (req) => {
          if (req.path.startsWith("/api/v1/plugins/")) {
            // The plugin's own assets go through the fetcher (its refresh, its
            // headers); the prefs and public endpoints take the raw token.
            return eveyFetchRaw({ url: req.path, method: req.method, headers: req.headers, body: req.body, authToken: req.authToken });
          }
          const headers: Record<string, string> = { ...req.headers };
          const bearer = req.authToken ?? accessToken;
          if (bearer) headers.Authorization = `Bearer ${bearer}`;
          return fetch(`${serverUrl}${req.path}`, { method: req.method, headers, body: req.body });
        },
        // No canvas in RN to downscale media.poster: the full-size artwork
        // (~1.3 MB typ.) is sent as is; only warn past ~1.5 MB.
        posterToDataUrl: (blob) => {
          if (blob.size > 1_500_000 && process.env.NODE_ENV !== "production") {
            log.warn("media.poster: large image, no downscale on RN", blob.size);
          }
          return new Promise<string>((resolve, reject) => {
            const fr = new FileReader();
            fr.onload = () => resolve(String(fr.result));
            fr.onerror = () => reject(new Error("asset read failed"));
            fr.readAsDataURL(blob);
          });
        },
      }),
    [pluginId, serverUrl, accessToken],
  );

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
        if (process.env.NODE_ENV !== "production") log.warn("config fetch failed", err);
      }
    })();
    return () => { cancelled = true; };
  }, [pluginId, accessToken, armed]);

  // Read the disk cache first, fall back to network. Authorization header
  // keeps the plugin token out of any URL. The bundle is then inlined into
  // the WebView's HTML.
  useEffect(() => {
    if (!accessToken || !serverUrl || !armed) return;
    let cancelled = false;
    const controller = new AbortController();
    (async () => {
      // 1. Try the cache - instant on warm starts.
      const cached = bundleCache ? await bundleCache.read(pluginId, entryPath) : null;
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
          void bundleCache?.write(pluginId, entryPath, text, res.headers.get("etag"));
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
  }, [pluginId, entryPath, accessToken, serverUrl, armed, bundleCache]);

  const isLightTheme = useMemo(() => isLightBackground(theme.bg), [theme.bg]);

  const html = useMemo(() => {
    if (!bundleCode) return null;
    if (!isValidPluginId(pluginId)) return null;
    const baseHref = `${serverUrl}/api/v1/plugins/${pluginId}/assets/`;
    const csp = [
      "default-src 'none'",
      "script-src 'unsafe-inline'",
      "style-src 'unsafe-inline'",
      `connect-src ${buildConnectSrc(externalFetch)}`,
      "img-src data: blob: " + serverUrl,
      "font-src data:",
    ].join("; ");
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
    html,body{margin:0;padding:0;width:100%;height:100%;background:transparent;color:${theme.text};font-family:-apple-system,BlinkMacSystemFont,sans-serif;font-size:14px;}
  </style>
  <script>${BRIDGE_SCRIPT}</script>
</head>
<body>
  <script>${inlineScriptSafe(bundleCode)}</script>
</body>
</html>`;
  }, [bundleCode, externalFetch, pluginId, serverUrl, theme.text]);

  // Host -> guest travels as an injected window.postMessage, so the same
  // bridge listener that receives parent messages on web works unchanged.
  const post = useCallback((payload: Record<string, unknown>) => {
    webviewRef.current?.injectJavaScript(`window.postMessage(${JSON.stringify(payload)}, "*"); true;`);
  }, []);

  const handleMessage = useCallback((e: WebViewMessageEvent) => {
    const msg = parseHostMessage(e.nativeEvent.data);
    if (!msg) return;
    switch (msg.type) {
      case "ready":
        if (initSent.current) return;
        initSent.current = true;
        post(buildInitPayload({
          context,
          config: pluginConfig,
          locale: i18n.language,
          theme: isLightTheme ? "light" : "dark",
          surface,
          suggestedHeight,
          viewportHeight,
        }));
        return;
      case "navigate":
        onNavigate?.(msg.path);
        return;
      case "toast":
        limitedToast(msg.level, msg.message);
        return;
      case "resize":
        setHeight(clampSlotHeight(msg.height, 60));
        return;
      case "rpc": {
        const id = msg.id;
        rpc.call(msg.method, msg.args)
          .then((data) => post({ type: "rpc-result", id, ok: true, data }))
          .catch((err: Error) => post({ type: "rpc-result", id, ok: false, error: err.message }));
        return;
      }
    }
  }, [context, pluginConfig, i18n.language, isLightTheme, surface, suggestedHeight, viewportHeight, onNavigate, limitedToast, rpc, post]);

  if (!accessToken) return null;

  if (error) {
    return (
      <View style={styles.error} onLayout={handleLayout}>
        <Text style={{ color: theme.textMuted, fontSize: 12 }}>
          {t("admin.plugin.frontLoadError", "Failed to load plugin")}
        </Text>
      </View>
    );
  }

  if (!html) {
    return (
      <View style={[styles.loading, { height, width: "100%" }]} onLayout={handleLayout}>
        <ActivityIndicator size="small" color={theme.accent} />
      </View>
    );
  }

  return (
    <View style={{ height, width: "100%", overflow: "hidden" }} onLayout={handleLayout} focusable={focusable}>
      <WebView
        ref={webviewRef}
        originWhitelist={["*"]}
        source={{ html, baseUrl: serverUrl }}
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
        focusable={focusable}
        // Prevent the WebView from navigating away if a plugin link is clicked
        // - the host's own router is the only allowed navigation surface.
        onShouldStartLoadWithRequest={(req) => {
          if (req.url === "about:blank" || req.url.startsWith("about:")) return true;
          if (req.url.startsWith(`${serverUrl}/api/v1/plugins/`)) return true;
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
