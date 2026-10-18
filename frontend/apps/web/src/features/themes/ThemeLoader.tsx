import { useEffect, useRef } from "react";
import { useThemeStore } from "@evey/store";
import { useAuthStore } from "@evey/store";
import { eveyFetchRaw } from "@evey/api";
import { frameLoadMessage, isFrameShellMessage, pluginFrameUrl } from "@evey/client";
import { THEME_BRIDGE_SCRIPT } from "./theme-bridge";
import { applyThemeCSSPolicy } from "./theme-css-policy";

const THEME_LINK_ID_PREFIX = "evey-theme-css-";
const THEME_STYLE_ID_PREFIX = "evey-theme-injected-";
const THEME_IFRAME_ID_PREFIX = "evey-theme-iframe-";

function removeAllThemeAssets() {
  // Remove any leftover CSS links and iframes from previous theme activations.
  document.querySelectorAll(`[id^="${THEME_LINK_ID_PREFIX}"]`).forEach((el) => el.remove());
  document.querySelectorAll(`[id^="${THEME_STYLE_ID_PREFIX}"]`).forEach((el) => el.remove());
  document.querySelectorAll(`[id^="${THEME_IFRAME_ID_PREFIX}"]`).forEach((el) => el.remove());
}

function injectThemeCSS(pluginId: string) {
  const existingId = THEME_LINK_ID_PREFIX + pluginId;
  if (document.getElementById(existingId)) return;
  const link = document.createElement("link");
  link.id = existingId;
  link.rel = "stylesheet";
  link.href = `/api/v1/themes/${encodeURIComponent(pluginId)}/theme.css`;
  document.head.appendChild(link);
}

// ThemeLoader mounts at the top of the app (inside ThemeProvider) and handles:
//   1. Injecting theme.css <link> into <head>
//   2. Spawning a hidden sandbox iframe to execute theme.js when present
//   3. Listening for postMessage from the iframe and processing evey.theme.* calls
//
// The iframe uses sandbox="allow-scripts" (no allow-same-origin) so it runs in
// an opaque origin - it cannot read parent DOM or cookies, only postMessage.
// This is the most restrictive sandbox that still permits the bridge to work.
export function ThemeLoader() {
  const pluginId = useThemeStore((s) => s.pluginId);
  // Subscribe to PRESENCE of a token (boolean), not its value: this re-runs the
  // effect on a null->value transition (login / re-auth without reload, where
  // pluginId may already be hydrated from persisted state) so theme.js gets
  // probed, while a value->value rotation (silent refresh) leaves the boolean
  // unchanged and does NOT re-init the assets (no CSS flash).
  const isAuthed = useAuthStore((s) => !!s.accessToken);
  const iframeRef = useRef<HTMLIFrameElement | null>(null);
  // theme.js as fetched with the user's token, posted into the frame page
  // when it says it is ready (no token in any URL, no inline script).
  const themeCodeRef = useRef<string | null>(null);

  useEffect(() => {
    if (!pluginId) {
      removeAllThemeAssets();
      iframeRef.current = null;
      return;
    }

    removeAllThemeAssets();
    injectThemeCSS(pluginId);

    // Only attempt theme.js if the user is authenticated - the route requires auth.
    // Read the token directly from store state (not subscribed) so that token
    // rotation does not re-trigger this effect and re-initialise the theme assets.
    const accessToken = useAuthStore.getState().accessToken;
    if (!accessToken) return;

    let cancelled = false;

    void (async () => {
      try {
        // Probe for theme.js existence using a HEAD-equivalent GET that tolerates 404.
        // We use eveyFetchRaw (not eveyFetch) to handle 404 without throwing.
        const res = await eveyFetchRaw({
          url: `/api/v1/themes/${encodeURIComponent(pluginId)}/theme.js`,
          method: "GET",
        }).catch((err: { httpStatus?: number }) => {
          // 404 means no theme.js - that is fine, CSS-only themes are valid.
          if (err?.httpStatus === 404) return null;
          throw err;
        });

        if (cancelled || !res) return;
        const code = await res.text();
        if (cancelled) return;
        themeCodeRef.current = code;

        // theme.js exists - create a hidden sandbox iframe.
        const iframeId = THEME_IFRAME_ID_PREFIX + pluginId;
        if (document.getElementById(iframeId)) return;

        const iframe = document.createElement("iframe");
        iframe.id = iframeId;
        iframe.setAttribute("sandbox", "allow-scripts");
        // Hidden - purely an execution context, no visible UI from theme.js in V1.
        iframe.style.display = "none";
        iframe.style.position = "absolute";
        iframe.style.width = "0";
        iframe.style.height = "0";
        // Theme code reaches no network: the frame page's connect-src is 'none'.
        iframe.src = pluginFrameUrl([]);
        document.body.appendChild(iframe);
        iframeRef.current = iframe;
      } catch {
        // Non-fatal - CSS still works if JS setup fails.
      }
    })();

    return () => {
      cancelled = true;
    };
  // Deps: pluginId + isAuthed (token PRESENCE). The token VALUE is read via
  // getState() inside the effect so a rotation doesn't re-init assets, but a
  // null->value login still re-runs the effect via the isAuthed boolean.
  }, [pluginId, isAuthed]);

  // postMessage listener - validated against the iframe's contentWindow before acting.
  useEffect(() => {
    function handleMessage(e: MessageEvent) {
      const iframe = iframeRef.current;
      // Guard: only accept messages from the active theme iframe.
      // sandbox="allow-scripts" iframes have an opaque "null" origin - we can't
      // use e.origin for same-origin checks, so we compare e.source instead.
      if (!iframe || e.source !== iframe.contentWindow) return;

      if (isFrameShellMessage(e.data)) {
        const code = themeCodeRef.current;
        if (code != null) iframe.contentWindow?.postMessage(frameLoadMessage([THEME_BRIDGE_SCRIPT, code]), "*");
        return;
      }

      const data = e.data as Record<string, unknown>;
      if (!data || typeof data.type !== "string") return;

      switch (data.type) {
        case "theme.css": {
          if (typeof data.rules !== "string") break;
          const { css: sanitized, rejected } = applyThemeCSSPolicy(data.rules);
          if (rejected.length) {
            console.warn(
              `[evey-theme] ${rejected.length} declaration(s) dropped - evey.theme.css() takes custom properties only:`,
              rejected.slice(0, 5),
            );
          }
          if (!sanitized) break;
          const currentPluginId = useThemeStore.getState().pluginId;
          if (!currentPluginId) break;
          const styleId = THEME_STYLE_ID_PREFIX + currentPluginId;
          let style = document.getElementById(styleId) as HTMLStyleElement | null;
          if (!style) {
            style = document.createElement("style");
            style.id = styleId;
            document.head.appendChild(style);
          }
          style.textContent = sanitized;
          break;
        }
        case "theme.log": {
          const level = data.level as string;
          const msg = `[evey-theme] ${String(data.msg ?? "")}`;
          if (level === "error") console.error(msg);
          else if (level === "warn") console.warn(msg);
          else if (level === "debug") console.debug(msg);
          else console.info(msg);
          break;
        }
        case "theme.registerComponent":
        case "theme.observe": {
          // Not implemented in V1 - the bridge already sent the not-implemented ack.
          break;
        }
      }
    }

    window.addEventListener("message", handleMessage);
    return () => window.removeEventListener("message", handleMessage);
  }, []);

  return null;
}
