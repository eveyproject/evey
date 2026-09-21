import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { toast, useAuthStore } from "@evey/store";
import { useTranslation } from "react-i18next";
import { eveyFetch, eveyFetchRaw } from "@evey/api";
import {
  buildConnectSrc,
  buildInitPayload,
  clampSlotHeight,
  createPluginRpc,
  createToastLimiter,
  inlineScriptSafe,
  isAllowedNavPath,
  isValidPluginId,
  parseHostMessage,
} from "@evey/client";
import { BRIDGE_SCRIPT } from "./front-bridge";

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

// buildSrcdoc renders the iframe's HTML. The plugin bundle is INLINED into
// a <script> tag rather than fetched via <script src=…?token=…> to keep the
// scoped plugin token out of any URL (Referer / browser history / server-log
// leak). The parent fetches the bundle text via Authorization: Bearer header
// and embeds it here. v1 plugins are single-bundle (no dynamic-import code
// splitting) so an inline <script> covers the full module.
//
// The meta CSP is the actual enforcement boundary inside the iframe: the
// CSP header on the JS response is cosmetic since CSP is per-document.
// `connect-src` is fed from the plugin's external_fetch allow-list so the
// front module can only reach hosts the admin has approved.
function buildSrcdoc(pluginId: string, bundleCode: string, externalFetch: string[]): string {
  // An invalid id short-circuits to an empty string and the iframe renders
  // nothing (see isValidPluginId for the shape and why).
  if (!isValidPluginId(pluginId)) {
    return "";
  }
  const themeVars = getThemeVars();
  // Use ABSOLUTE URLs for the base href. Firefox in an about:srcdoc iframe
  // resolves path-only URLs against `about:srcdoc` (not the parent host),
  // which leaves relative paths unresolvable. Absolute URLs sidestep the
  // difference. The base remains useful for any same-origin <img>/<link>
  // the bundle injects at runtime.
  const hostOriginForUrl = typeof window !== "undefined" ? window.location.origin : "";
  const baseHref = `${hostOriginForUrl}/api/v1/plugins/${pluginId}/assets/`;
  // Sandbox iframe (allow-scripts only, no allow-same-origin) gets an opaque
  // origin where 'self' resolves to "no origin matches". Anything cross-
  // origin, including scripts loaded from this very evey host, must be
  // allowed via explicit origin. Use window.location.origin so the CSP
  // matches the parent host the iframe is embedded in.
  const hostOrigin = typeof window !== "undefined" ? window.location.origin : "";
  const csp = [
    "default-src 'none'",
    `script-src ${hostOrigin} 'unsafe-inline'`,
    `style-src ${hostOrigin} 'unsafe-inline'`,
    `connect-src ${buildConnectSrc(externalFetch)}`,
    `img-src ${hostOrigin} data: blob:`,
    `font-src ${hostOrigin} data:`,
  ].join("; ");
  return `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta http-equiv="Content-Security-Policy" content="${csp}">
  <base href="${baseHref}">
  <style>
    *, *::before, *::after { box-sizing: border-box; }
    html, body { margin: 0; padding: 0; background: transparent; font-family: system-ui, -apple-system, sans-serif; }
    :root {
    ${themeVars}
    }
    body { color: var(--text, #f1f5f9); }
  </style>
  <script>${BRIDGE_SCRIPT}<\/script>
</head>
<body>
  <script>${inlineScriptSafe(bundleCode)}<\/script>
</body>
</html>`;
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

export function PluginIframe({ pluginId, slot, src, externalFetch, context }: PluginIframeProps) {
  const iframeRef = useRef<HTMLIFrameElement>(null);
  const [suggestedHeight] = useState(() => suggestedSlotHeight(slot, window.innerWidth));
  const [height, setHeight] = useState(suggestedHeight ?? 100);
  const [error, setError] = useState(false);
  const [bundleCode, setBundleCode] = useState<string | null>(null);
  const [pluginConfig, setPluginConfig] = useState<Record<string, unknown>>({});
  const navigate = useNavigate();
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
  // scoped token never appears in a URL. The bundle is then inlined into
  // the iframe srcdoc as a <script> tag (single-bundle in v1, no chunk
  // splitting). authorizeFrontJS server-side accepts the user access token
  // here, identical to how it accepted the plugin token in the URL form.
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
        if (!cancelled) setBundleCode(code);
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
              surface: "web",
              suggestedHeight,
            }),
            "*",
          );
          break;
        case "navigate":
          if (isAllowedNavPath(msg.path)) {
            navigate({ to: msg.path });
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
    [pluginConfig, i18n.language, navigate, rpc, limitedToast, suggestedHeight],
  );

  useEffect(() => {
    window.addEventListener("message", handleMessage);
    return () => window.removeEventListener("message", handleMessage);
  }, [handleMessage]);

  const srcdoc = useMemo(
    () => (bundleCode != null ? buildSrcdoc(pluginId, bundleCode, externalFetch ?? []) : null),
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

  if (!srcdoc) return null;

  return (
    <iframe
      ref={iframeRef}
      sandbox="allow-scripts"
      srcDoc={srcdoc}
      onError={() => setError(true)}
      style={{
        border: "none",
        width: "100%",
        height: `${height}px`,
        overflow: "hidden",
        display: "block",
        background: "transparent",
        colorScheme: "normal",
      }}
      title={`${t("admin.plugin.sandboxedFront")} - ${pluginId}`}
    />
  );
}
