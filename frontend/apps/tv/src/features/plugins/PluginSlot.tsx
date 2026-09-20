import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { View, ActivityIndicator, Text, StyleSheet, InteractionManager, useWindowDimensions } from "react-native";
import { WebView, type WebViewMessageEvent } from "react-native-webview";
import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import { eveyFetch, eveyFetchRaw, adminPluginServiceIssuePluginToken } from "@evey/api";
import { useAuthStore, toast } from "@evey/store";
import { useTheme } from "../../providers/ThemeProvider";
import { CARD_WIDTH_FEATURED, POSTER_RATIO, LABEL_BLOCK_HEIGHT } from "../../lib/tvLayout";
import { useServerUrl } from "../../hooks/useServerUrl";
import { PLUGIN_BRIDGE_SCRIPT } from "./pluginBridge";
import { FONT_SIZE } from "@evey/theme";
import { logger } from "../../lib/log";

// What must stay visible UNDER a widget: a full poster row, its title and its
// label. The ceiling is whatever is left between where the widget actually
// starts and that row - measured, not guessed, so it follows both the poster
// size and whatever chrome the host screen happens to put above.
const HOME_CHROME_HEIGHT = 184;
// 24sp of type plus its 16dp bottom margin - the estimate of 30 was short by a
// line, which is what clipped the year under each poster.
const ROW_TITLE_HEIGHT = 45;
const FIRST_ROW_HEIGHT = CARD_WIDTH_FEATURED * POSTER_RATIO + LABEL_BLOCK_HEIGHT + ROW_TITLE_HEIGHT;

const log = logger("plugin");

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

// Gates what we accept at the HTML boundary (mirrors web PluginIframe's
// pluginIdShape / mobile PluginSlot). The server issues `plg-<24 hex>` slugs;
// re-asserting here means a malformed pluginId can never terminate the
// `<base href>` attribute or inject markup into the WebView document.
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
 * with their display name + resolved entry path. Used by the settings screen
 * to render a menu row per plugin (each linking to its own dedicated screen) -
 * TV parity with web/mobile's usePluginSlotEntries.
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
  /**
   * Skip plugins whose id is in this set. The home screen uses it to render
   * only the newly-installed plugins the feed config does not place yet; the
   * rest are rendered at their configured position by a PluginSlot with
   * `pluginId` set. Without it the two calls render the same plugin twice.
   */
  excludeRegistered?: Set<string>;
}

/**
 * TV parity with web/mobile's PluginSlot. Each matching plugin gets its own
 * WebView running the plugin's frontend bundle in isolation. The WebView is
 * display-only on TV - plugins render content, they never trap the D-pad.
 * That is enforced natively (plugins/withWidgetFocusGuard.js): `focusable`
 * only exists on React Native's plain View, so the prop below does nothing to
 * the Android WebView, which is focusable by default.
 */
export function PluginSlot({ name, context, pluginId, excludeRegistered }: PluginSlotProps) {
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
    if (excludeRegistered?.has(plugin.id)) continue;
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
  // failure-shown-as-empty contract as web/mobile, same i18n key as the
  // per-module load error.
  if (isError) return <PluginSlotError />;
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
  // A widget asks for the height its own layout wants, sized for a phone card;
  // on a television that number ate the screen before the first row (owner,
  // 2026-09-20). The ceiling is what is left once a FULL poster row fits under
  // it - header, row title and label included - which is the criterion a TV
  // home is judged on, not a taste-based percentage. A canvas widget re-renders
  // smaller (the bridge nudges a resize); a DOM one is clipped, the lesser evil.
  const { height: windowHeight } = useWindowDimensions();
  // Chrome above the first widget on the home screen, measured on a 1080p
  // panel: search field, Customize row and the paddings between them. Read from
  // onLayout first, but that fires before the siblings are positioned and
  // reports 0, so the measured value is the floor of the two.
  const maxWidgetHeight = Math.max(90, Math.round(windowHeight - HOME_CHROME_HEIGHT - FIRST_ROW_HEIGHT));
  const [error, setError] = useState(false);
  // Two-stage gate before we mount the WebView and download a bundle:
  //   1. `interactionsDone` - wait for any in-flight RN animation/transition
  //      to settle (InteractionManager).
  //   2. `inViewport` - wait until the host laid out our container (height>0
  //      means the parent scroll/list resolved our position). Keeps a plugin
  //      below the fold from running until the user scrolls there.
  // Mirrors mobile's PluginSlot (which also persists the bundle to disk;
  // TV has no expo-file-system dependency so it just refetches each mount).
  const [interactionsDone, setInteractionsDone] = useState(false);
  const [inViewport, setInViewport] = useState(false);
  const armed = interactionsDone && inViewport;
  const toastTimestamps = useRef<number[]>([]);
  const initSent = useRef(false);

  // Scoped plugin token used to proxy the WebView's prefs/public RPCs. Held by
  // the host (never exposed to the WebView JS); the server enforces the
  // plugin's granted scopes deny-by-default. Mirrors web/mobile PluginSlot.
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
        // on plugin assets. Mirrors web/mobile PluginSlot's assets.read.
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
    // without any CORS/taint concern). Mirrors web/mobile PluginSlot's
    // pluginRpcCall, except there is no canvas in RN to downscale media.poster -
    // the full-size artwork (~1.3 MB typ.) is sent as-is; only warn past ~1.5 MB.
    if (method === "assets.read" || method === "media.poster") {
      const blob = await res.blob();
      if (method === "media.poster" && blob.size > 1_500_000 && process.env.NODE_ENV !== "production") {
        log.warn("media.poster: large image, no downscale on RN", blob.size);
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

  // Fetch the plugin's saved config; same shape and endpoint as web/mobile.
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

  // Authorization header keeps the plugin token out of any URL. The bundle
  // is then inlined into the WebView's HTML.
  useEffect(() => {
    if (!accessToken || !serverUrl || !armed) return;
    let cancelled = false;
    const controller = new AbortController();
    (async () => {
      try {
        const url = `${serverUrl}/api/v1/plugins/${encodeURIComponent(pluginId)}/assets/${entryPath}`;
        const res = await fetch(url, {
          headers: { Authorization: `Bearer ${accessToken}` },
          signal: controller.signal,
        });
        if (!res.ok) throw new Error(`fetch failed: ${res.status}`);
        const text = await res.text();
        if (cancelled) return;
        setBundleCode(text);
      } catch (err: unknown) {
        if (err instanceof Error && err.name === "AbortError") return;
        if (!cancelled) setError(true);
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
      });
      webviewRef.current?.injectJavaScript(
        `window.postMessage(${initPayload}, "*"); true;`,
      );
      return;
    }
    if (type === "navigate" && typeof msg.path === "string") {
      // TV routes are simpler than web/mobile's nav surface; only home is
      // wired to plugin slots today, so plugin-triggered navigation isn't
      // supported yet - drop it rather than guess an unsafe destination.
      return;
    }
    if (type === "toast" && typeof msg.message === "string") {
      const now = Date.now();
      const recent = toastTimestamps.current.filter((ts) => now - ts < TOAST_WINDOW_MS);
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
      // Over the ceiling, CROPPING the widget cuts its text mid-glyph (the
      // weather scene lost half its location line, 2026-09-20). Scale the
      // guest document down instead: `zoom` is a layout property, so the
      // widget re-reports a height that fits and nothing is clipped.
      if (msg.height > maxWidgetHeight) {
        const ratio = Math.max(0.45, maxWidgetHeight / msg.height);
        webviewRef.current?.injectJavaScript(
          `document.documentElement.style.zoom=${ratio};true;`,
        );
      }
      setHeight(Math.max(60, Math.min(maxWidgetHeight, msg.height)));
      return;
    }
    if (type === "rpc") {
      const id = msg.id;
      const method = typeof msg.method === "string" ? msg.method : "";
      const args = (msg.args as Record<string, unknown>) ?? {};
      const reply = (payload: Record<string, unknown>) =>
        webviewRef.current?.injectJavaScript(`window.postMessage(${JSON.stringify(payload)}, "*"); true;`);
      pluginRpcCall(method, args)
        .then((resData) => reply({ type: "rpc-result", id, ok: true, data: resData }))
        .catch((err: Error) => reply({ type: "rpc-result", id, ok: false, error: err.message }));
      return;
    }
  }, [context, pluginConfig, i18n.language, isLightTheme, pluginRpcCall, maxWidgetHeight]);

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
    <View style={{ height, width: "100%" }} onLayout={handleLayout} focusable={false}>
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
        // Display-only: it has no remote-control affordance of its own. Kept
        // for intent and for the day the prop is honoured; what actually keeps
        // the D-pad out is the native guard (see the doc comment above).
        focusable={false}
        androidLayerType="hardware"
        mixedContentMode="always"
        mediaPlaybackRequiresUserAction={false}
        allowsInlineMediaPlayback
        style={{ backgroundColor: "transparent", flex: 1 }}
        // Prevent the WebView from navigating away if a plugin link is clicked.
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
