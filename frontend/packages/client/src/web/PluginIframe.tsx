import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { toast, useAuthStore } from "@evey/store";
import { useTranslation } from "react-i18next";
import { eveyFetch, eveyFetchRaw } from "@evey/api";
import {
  buildInitPayload,
  clampSlotHeight,
  createPluginRpc,
  createToastLimiter,
  frameLoadMessage,
  isAllowedNavPath,
  isFrameShellMessage,
  isValidPluginId,
  parseHostMessage,
  pluginFrameUrl,
} from "@evey/client";
import { BRIDGE_SCRIPT } from "../plugins/bridgeScript";

// The protocol rules live in @evey/client; re-exported so the tests and the
// callers that grew up here keep their import.
export { isAllowedNavPath };

interface PluginIframeProps {
  pluginId: string;
  // Slot name from the manifest ("home.section"): decides the height the host
  // recommends in the init payload.
  slot: string;
  // src is the entry path RELATIVE to the plugin's web_root (e.g. "index.js"
  // when the manifest declared web_root: "dist" and frontend.src: "dist/index.js").
  // PluginSlot strips the prefix; PluginIframe just appends to the assets base.
  src: string;
  externalFetch?: string[];
  context?: Record<string, unknown>;
  /**
   * Where a plugin's navigation request goes. Each client owns its router:
   * the browser app hands its own navigate, the browser television hands the
   * hash router's.
   */
  onNavigate: (path: string) => void;
  /** What the plugin is told it runs on; a television lays itself out bigger. */
  surface?: "web" | "tv";
  /** The slot height this host recommends, in the plugin's own pixels. */
  suggestedHeight?: number;
  /** Host pixels per plugin pixel: one per dp, as the Android television's WebView. */
  pixelScale?: number;
}

function getThemeVars(): string {
  const computed = getComputedStyle(document.documentElement);
  const vars: string[] = [];
  for (const sheet of document.styleSheets) {
    try {
      for (const rule of sheet.cssRules) {
        if (rule instanceof CSSStyleRule && rule.selectorText === ":root") {
          for (let i = 0; i < rule.style.length; i++) {
            const prop = rule.style[i];
            if (prop.startsWith("--")) {
              vars.push(`${prop}: ${computed.getPropertyValue(prop).trim()};`);
            }
          }
        }
      }
    } catch {
      // cross-origin stylesheets - skip
    }
  }
  return vars.join("\n    ");
}

function detectThemeMode(): "dark" | "light" {
  const bg = getComputedStyle(document.documentElement).getPropertyValue("--bg").trim();
  if (!bg) return "dark";
  const temp = document.createElement("div");
  temp.style.color = bg;
  document.body.appendChild(temp);
  const rgb = getComputedStyle(temp).color;
  temp.remove();
  const match = rgb.match(/\d+/g);
  if (!match) return "dark";
  const luminance = (Number(match[0]) * 299 + Number(match[1]) * 587 + Number(match[2]) * 114) / 1000;
  return luminance > 128 ? "light" : "dark";
}

// The frame's stylesheet: the host theme's variables, so a plugin styled
// with var(--text) looks native.
function frameCss(): string {
  return `:root {\n    ${getThemeVars()}\n  }\n  body { color: var(--text, #f1f5f9); }`;
}

// The height the host recommends for a slot on this surface. A plugin that
// never calls evey.resize() opens at it, so every home widget lands the same
// size without knowing where it runs; a plugin with an opinion still resizes.
export function suggestedSlotHeight(slot: string, viewportWidth: number): number | undefined {
  if (slot !== "home.section") return undefined;
  return viewportWidth < 420 ? 216 : viewportWidth < 700 ? 248 : 280;
}

// Downscale a poster to a thumbnail in the parent (unsandboxed, same-origin
// as the fetched artwork -> no canvas taint) so the postMessage payload stays
// small: a full poster is ~1.3 MB, a wall of them would freeze the UI.
async function posterThumbnail(blob: Blob): Promise<string> {
  const bmp = await createImageBitmap(blob);
  const targetW = 320;
  const scale = Math.min(1, targetW / bmp.width);
  const cw = Math.max(1, Math.round(bmp.width * scale));
  const ch = Math.max(1, Math.round(bmp.height * scale));
  const canvas = document.createElement("canvas");
  canvas.width = cw;
  canvas.height = ch;
  const ctx = canvas.getContext("2d");
  if (!ctx) { bmp.close?.(); throw new Error("poster resize failed"); }
  ctx.drawImage(bmp, 0, 0, cw, ch);
  bmp.close?.();
  return canvas.toDataURL("image/jpeg", 0.82);
}

const MIN_SLOT_HEIGHT = 50;

export function PluginIframe({ pluginId, slot, src, externalFetch, context, onNavigate, surface = "web", suggestedHeight: hostSuggested, pixelScale = 1 }: PluginIframeProps) {
  const iframeRef = useRef<HTMLIFrameElement>(null);
  const [suggestedHeight] = useState(() => hostSuggested ?? suggestedSlotHeight(slot, window.innerWidth));
  const [height, setHeight] = useState(suggestedHeight ?? 100);
  const [error, setError] = useState(false);
  const [bundleCode, setBundleCode] = useState<string | null>(null);
  // Read by the message handler: the frame page can say it is ready before the
  // listener holding the fresh state is registered again.
  const bundleRef = useRef<string | null>(null);
  const [pluginConfig, setPluginConfig] = useState<Record<string, unknown>>({});
  const { t, i18n } = useTranslation();
  const accessToken = useAuthStore((s) => s.accessToken);

  // Fetch the plugin's saved config and forward it to the iframe in
  // the init message so front modules don't have to deal with auth themselves.
  // Accessible to any authenticated user - returns the values the admin configured.
  useEffect(() => {
    if (!accessToken) return;
    let cancelled = false;
    eveyFetch<{ values?: Record<string, unknown> }>({
      url: `/api/v1/plugins/${encodeURIComponent(pluginId)}/config`,
      method: "GET",
    })
      .then((data) => {
        if (cancelled) return;
        setPluginConfig(data.values ?? {});
      })
      .catch((err) => {
        if (!cancelled) {
          console.warn("plugin config fetch failed", err);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [pluginId, accessToken]);

  // Fetch the plugin bundle text via Authorization: Bearer header so the
  // scoped token never appears in a URL. It runs in /plugin-frame.html,
  // posted in once the page says it is ready (single-bundle in v1, no chunk
  // splitting). authorizeFrontJS server-side accepts the user access token.
  useEffect(() => {
    if (!accessToken) return;
    let cancelled = false;
    (async () => {
      try {
        // cache: "no-cache" = always revalidate with If-None-Match. The server
        // answers 304 when unchanged, so the cost is one conditional request -
        // and it repairs clients whose cache still holds a bundle stored under
        // the old immutable policy (those entries never revalidate on their
        // own, not even reliably on a hard refresh for JS-initiated fetches).
        const res = await eveyFetchRaw({
          url: `/api/v1/plugins/${encodeURIComponent(pluginId)}/assets/${src}`,
          method: "GET",
          cache: "no-cache",
        });
        if (!res.ok) {
          throw new Error(`plugin bundle fetch failed: ${res.status}`);
        }
        const code = await res.text();
        if (!cancelled) {
          bundleRef.current = code;
          setBundleCode(code);
        }
      } catch (e) {
        console.warn("plugin bundle fetch failed", e);
        if (!cancelled) setError(true);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [pluginId, src, accessToken]);

  // The toast cap and the RPC endpoint (scoped token, endpoint allowlist)
  // are the shared host rules; only the transport is this surface's.
  const limitedToast = useMemo(() => createToastLimiter(toast), []);
  const rpc = useMemo(
    () =>
      createPluginRpc(pluginId, {
        request: (req) =>
          eveyFetchRaw({ url: req.path, method: req.method, headers: req.headers, body: req.body, authToken: req.authToken }),
        posterToDataUrl: posterThumbnail,
      }),
    [pluginId],
  );

  // context is an inline object at every call site (context={{ mediaId }}), so
  // depending on it re-identifies handleMessage on every parent render and the
  // window "message" listener churns - on the player route that is every
  // SyncPlay event, with a window where an in-flight plugin RPC reply can land
  // between the removeEventListener and the re-add. It is only read when the
  // iframe says "ready", so a ref hands the current value without the churn.
  const contextRef = useRef(context);
  useEffect(() => { contextRef.current = context; }, [context]);

  const handleMessage = useCallback(
    (e: MessageEvent) => {
      const iframe = iframeRef.current;
      if (!iframe || e.source !== iframe.contentWindow) return;
      if (e.origin !== "null" && e.origin !== window.location.origin) return;

      // The frame page is ready: hand it the bridge and the bundle, in order.
      if (isFrameShellMessage(e.data)) {
        const bundle = bundleRef.current;
        if (bundle == null || !isValidPluginId(pluginId)) return;
        const base = `${window.location.origin}/api/v1/plugins/${pluginId}/assets/`;
        iframe.contentWindow?.postMessage(frameLoadMessage([BRIDGE_SCRIPT, bundle], frameCss(), base), "*");
        return;
      }

      const msg = parseHostMessage(e.data);
      if (!msg) return;

      switch (msg.type) {
        case "ready":
          iframe.contentWindow?.postMessage(
            buildInitPayload({
              context: contextRef.current,
              config: pluginConfig,
              locale: i18n.language,
              theme: detectThemeMode(),
              surface,
              suggestedHeight,
            }),
            "*",
          );
          break;
        case "navigate":
          if (isAllowedNavPath(msg.path)) {
            onNavigate(msg.path);
          }
          break;
        case "toast":
          limitedToast(msg.level, msg.message);
          break;
        case "resize":
          setHeight(clampSlotHeight(msg.height, MIN_SLOT_HEIGHT));
          break;
        case "rpc": {
          const id = msg.id;
          rpc.call(msg.method, msg.args)
            .then((result) =>
              iframe.contentWindow?.postMessage({ type: "rpc-result", id, ok: true, data: result }, "*"),
            )
            .catch((err: Error) =>
              iframe.contentWindow?.postMessage({ type: "rpc-result", id, ok: false, error: err.message }, "*"),
            );
          break;
        }
      }
    },
    [pluginConfig, i18n.language, onNavigate, rpc, limitedToast, suggestedHeight, surface, pluginId],
  );

  useEffect(() => {
    window.addEventListener("message", handleMessage);
    return () => window.removeEventListener("message", handleMessage);
  }, [handleMessage]);

  // An invalid id renders nothing (see isValidPluginId for the shape and why).
  // The frame is mounted once the bundle is here, so its handshake finds it.
  const frameSrc = useMemo(
    () => (bundleCode != null && isValidPluginId(pluginId) ? pluginFrameUrl(externalFetch ?? []) : null),
    [pluginId, bundleCode, externalFetch],
  );

  if (!accessToken) return null;

  if (error) {
    return (
      <div className="text-sm py-2" style={{ color: "var(--text-muted)" }}>
        {t("admin.plugin.frontLoadError")}
      </div>
    );
  }

  if (!frameSrc) return null;

  const frame = (
    <iframe
      ref={iframeRef}
      sandbox="allow-scripts"
      src={frameSrc}
      onError={() => setError(true)}
      style={{
        border: "none",
        width: pixelScale === 1 ? "100%" : `${100 / pixelScale}%`,
        height: `${height}px`,
        overflow: "hidden",
        display: "block",
        background: "transparent",
        colorScheme: "normal",
        ...(pixelScale === 1 ? {} : { transform: `scale(${pixelScale})`, transformOrigin: "0 0" }),
      }}
      title={`${t("admin.plugin.sandboxedFront")} - ${pluginId}`}
    />
  );
  if (pixelScale === 1) return frame;
  return <div style={{ height: `${height * pixelScale}px`, overflow: "hidden" }}>{frame}</div>;
}
