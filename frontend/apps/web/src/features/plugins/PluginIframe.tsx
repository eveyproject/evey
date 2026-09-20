import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { toast, useAuthStore } from "@evey/store";
import { useTranslation } from "react-i18next";
import { eveyFetch, eveyFetchRaw, adminPluginServiceIssuePluginToken } from "@evey/api";
import { BRIDGE_SCRIPT } from "./front-bridge";

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

// Plugin frontends may request client-side navigation via postMessage. Only
// non-sensitive paths are allowed: an attacker plugin must not be able to push
// the user to /admin/*, /auth/*, /setup/*, or /profile/security/* (overlay +
// nav = phishing vector). Protocol-relative URLs ("//evil.com"), absolute URLs,
// and javascript:/data: schemes are rejected so the iframe cannot pivot the
// top window away from the app origin.
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

// pluginIdShape gates what we accept at the HTML boundary. The server
// issues `plg-<24 hex>` slugs (`id.New("plg-")`); we re-assert here so a
// malformed pluginId (typo, manipulated state, future bug) can never
// terminate the `<base href>` attribute or inject markup. The pattern is
// deliberately permissive within `plg-…` (lowercase + digits + `_`/`-`,
// bounded length) so tests and human-readable dev slugs keep working while
// every HTML-special character (`"`, `<`, `>`, space, `/`, ...) is refused.
// An invalid id short-circuits to an empty string and the iframe renders
// nothing.
const pluginIdShape = /^plg-[a-zA-Z0-9_-]{1,64}$/;

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
  if (!pluginIdShape.test(pluginId)) {
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
  // CSP origins must be absolute (scheme + host). The manifest declares
  // bare hostnames in external_fetch (api.open-meteo.com); prefix https://
  // so the browser doesn't resolve them against the iframe's about:srcdoc
  // base, which would yield invalid `about://hostname` directives.
  const connectSrc = externalFetch.length > 0
    ? externalFetch.map((h) => (/^https?:\/\//i.test(h) ? h : `https://${h}`)).join(" ")
    : "'none'";
  const csp = [
    "default-src 'none'",
    `script-src ${hostOrigin} 'unsafe-inline'`,
    `style-src ${hostOrigin} 'unsafe-inline'`,
    `connect-src ${connectSrc}`,
    `img-src ${hostOrigin} data: blob:`,
    `font-src ${hostOrigin} data:`,
  ].join("; ");
  // Defuse any literal `</script>` in the bundle text so the HTML parser
  // doesn't terminate the inline script early. The replacement is HTML-only
  // (the JS tokenizer treats `<\/script>` as the same string).
  const inlineSafe = bundleCode.replace(/<\/script/gi, "<\\/script");
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
  <script>${inlineSafe}<\/script>
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

  // Sliding-window rate limit for toasts: a plugin can emit at most TOAST_MAX
  // toasts per TOAST_WINDOW_MS. Drop silently past the cap - a noisy plugin
  // must not be able to fill the screen or thrash the toast store.
  const TOAST_MAX = 3;
  const TOAST_WINDOW_MS = 1000;
  const toastTimestamps = useRef<number[]>([]);

  // Scoped plugin token used to proxy the iframe's prefs/public RPCs. Held by
  // the parent (never exposed to the iframe JS); the server enforces the
  // plugin's granted scopes on it (deny-by-default). Lazily issued + re-issued
  // on expiry.
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

  // Maps a bridge method to its endpoint, called with the plugin token so the
  // server gates it by the granted scope. Only this allowlist is reachable.
  const callPluginEndpoint = useCallback((method: string, args: Record<string, unknown>, token: string): Promise<Response> => {
    const enc = encodeURIComponent;
    switch (method) {
      case "prefs.get":
        return eveyFetchRaw({ url: `/api/v1/plugin-prefs/${enc(String(args.key ?? ""))}`, method: "GET", authToken: token });
      case "prefs.set":
        return eveyFetchRaw({ url: `/api/v1/plugin-prefs/${enc(String(args.key ?? ""))}`, method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ value: String(args.value ?? "") }), authToken: token });
      case "prefs.delete":
        return eveyFetchRaw({ url: `/api/v1/plugin-prefs/${enc(String(args.key ?? ""))}`, method: "DELETE", authToken: token });
      case "prefs.list":
        return eveyFetchRaw({ url: `/api/v1/plugin-prefs`, method: "GET", authToken: token });
      case "media.get":
        return eveyFetchRaw({ url: `/api/v1/public/media/${enc(String(args.id ?? ""))}`, method: "GET", authToken: token });
      case "media.list":
        return eveyFetchRaw({ url: `/api/v1/public/media${args.libraryId ? `?libraryId=${enc(String(args.libraryId))}` : ""}`, method: "GET", authToken: token });
      case "media.poster":
        // The user's own session (no plugin token): the artwork handler applies
        // this user's library ACL + parental filter, so a plugin can only reach
        // posters the user can already see (same exposure as media.list).
        return eveyFetchRaw({ url: `/api/v1/media/${enc(String(args.id ?? ""))}/artwork/poster`, method: "GET" });
      case "assets.read": {
        // Reads one of the plugin's OWN web_root assets. Strict path guard
        // (the server also rejects traversal, this is defense in depth). The
        // cache-buster predates the server's no-cache+ETag policy; kept as
        // belt-and-braces for intermediaries that ignore revalidation.
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
  }, [pluginId]);

  const pluginRpcCall = useCallback(async (method: string, args: Record<string, unknown>): Promise<unknown> => {
    let token = await ensurePluginToken();
    if (!token) throw new Error("plugin token unavailable");
    let res = await callPluginEndpoint(method, args, token);
    if (res.status === 401) {
      pluginTokenRef.current = null; // expired - re-issue once
      token = await ensurePluginToken();
      if (token) res = await callPluginEndpoint(method, args, token);
    }
    if (!res.ok) {
      let msg = `${res.status}`;
      try {
        const b = (await res.clone().json()) as { message?: string };
        if (b.message) msg = b.message;
      } catch { /* ignore */ }
      throw new Error(msg);
    }
    // assets.read returns binary; hand it back as a data: URL so the iframe can
    // use it directly (data URLs are same-origin, safe for WebGL textures).
    if (method === "assets.read") {
      const blob = await res.blob();
      return await new Promise<string>((resolve, reject) => {
        const fr = new FileReader();
        fr.onload = () => resolve(String(fr.result));
        fr.onerror = () => reject(new Error("asset read failed"));
        fr.readAsDataURL(blob);
      });
    }
    if (method === "media.poster") {
      // Downscale to a thumbnail in the parent (unsandboxed, same-origin as the
      // fetched artwork -> no canvas taint) so the postMessage payload stays
      // small: a full poster is ~1.3 MB, a wall of them would freeze the UI.
      const blob = await res.blob();
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
    const text = await res.text();
    return text ? JSON.parse(text) : {};
  }, [ensurePluginToken, callPluginEndpoint]);

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

      const data = e.data as Record<string, unknown>;
      if (!data || typeof data.type !== "string") return;

      switch (data.type) {
        case "ready":
          iframe.contentWindow?.postMessage(
            {
              type: "init",
              context: contextRef.current ?? {},
              config: pluginConfig,
              locale: i18n.language,
              theme: detectThemeMode(),
              surface: "web",
              ...(suggestedHeight !== undefined ? { suggestedHeight } : {}),
            },
            "*",
          );
          break;
        case "navigate":
          if (typeof data.path === "string" && isAllowedNavPath(data.path)) {
            navigate({ to: data.path });
          }
          break;
        case "toast":
          if (typeof data.message === "string") {
            const now = Date.now();
            const recent = toastTimestamps.current.filter((t) => now - t < TOAST_WINDOW_MS);
            if (recent.length >= TOAST_MAX) {
              toastTimestamps.current = recent;
              break; // drop silently - over the rate cap
            }
            recent.push(now);
            toastTimestamps.current = recent;
            const msg = data.message.slice(0, 200);
            const level = data.level as "success" | "error" | "info";
            if (level === "success") toast.success(msg);
            else if (level === "error") toast.error(msg);
            else toast.info(msg);
          }
          break;
        case "resize":
          if (typeof data.height === "number" && data.height >= 0) {
            // Clamp to [50, 800] to prevent visual DoS via unbounded resize;
            // 0 collapses the slot (a plugin with nothing to show).
            setHeight(data.height === 0 ? 0 : Math.max(50, Math.min(800, data.height)));
          }
          break;
        case "rpc": {
          const id = data.id;
          const method = typeof data.method === "string" ? data.method : "";
          const args = (data.args as Record<string, unknown>) ?? {};
          pluginRpcCall(method, args)
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
    [pluginConfig, i18n.language, navigate, pluginRpcCall],
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
