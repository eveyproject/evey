// The host side of the plugin protocol, shared by the three hosts: the web's
// sandboxed iframe, the phone's WebView and the television's WebView. What
// differs between them (how the document is mounted, how a message travels)
// stays in each host; what must NOT differ (which paths a plugin may send the
// user to, what a plugin id may look like, how far a plugin may grow, how
// often it may toast) lives here, once.

// Plugin frontends may request client-side navigation. Only non-sensitive
// paths are allowed: an attacker plugin must not be able to push the user to
// /admin/*, /auth/*, /setup/*, or /profile/security/* (overlay + nav =
// phishing vector). Protocol-relative URLs ("//evil.com"), absolute URLs, and
// javascript:/data: schemes are rejected so a plugin cannot pivot the host
// away from the app.
const ALLOWED_NAV_PREFIXES = ["/home", "/library", "/media", "/player", "/settings", "/search"];

export function isAllowedNavPath(path: string): boolean {
  if (!path.startsWith("/")) return false;
  if (path.startsWith("//")) return false;
  if (path.length > 2048) return false;
  return ALLOWED_NAV_PREFIXES.some((p) => path === p || path.startsWith(p + "/") || path.startsWith(p + "?"));
}

// Gates what a host accepts at the HTML boundary. The server issues
// `plg-<24 hex>` slugs (`id.New("plg-")`); re-asserting here means a malformed
// pluginId (typo, manipulated state, future bug) can never terminate the
// `<base href>` attribute or inject markup. The pattern is deliberately
// permissive within `plg-…` (lowercase + digits + `_`/`-`, bounded length) so
// tests and human-readable dev slugs keep working while every HTML-special
// character (`"`, `<`, `>`, space, `/`, ...) is refused.
const pluginIdShape = /^plg-[a-zA-Z0-9_-]{1,64}$/;

export function isValidPluginId(pluginId: string): boolean {
  return pluginIdShape.test(pluginId);
}

/**
 * The `connect-src` a plugin document gets: the admin-approved external hosts,
 * or nothing. CSP origins must be absolute (scheme + host); the manifest
 * declares bare hostnames (api.open-meteo.com), so https:// is prefixed rather
 * than letting the browser resolve them against the document's base.
 */
export function buildConnectSrc(externalFetch: string[]): string {
  if (externalFetch.length === 0) return "'none'";
  return externalFetch.map((h) => (/^https?:\/\//i.test(h) ? h : `https://${h}`)).join(" ");
}

/**
 * Defuses any literal `</script>` in a bundle so the HTML parser does not end
 * the inline script early. HTML-only: the JS tokenizer reads `<\/script>` as
 * the same string.
 */
export function inlineScriptSafe(bundleCode: string): string {
  return bundleCode.replace(/<\/script/gi, "<\\/script");
}

export const SLOT_MAX_HEIGHT = 800;

/**
 * The height a plugin asked for, bounded: a cap against visual DoS through an
 * unbounded resize, a floor so a slot never renders unusable, and 0 kept as
 * is - it collapses the slot, for a plugin with nothing to show (the
 * librarian on a clean shelf must not leave a blank strip).
 */
export function clampSlotHeight(requested: number, minHeight: number): number {
  if (requested === 0) return 0;
  return Math.max(minHeight, Math.min(SLOT_MAX_HEIGHT, requested));
}

export type ToastLevel = "success" | "error" | "info";

export interface ToastSink {
  success(message: string): void;
  error(message: string): void;
  info(message: string): void;
}

const TOAST_MAX = 3;
const TOAST_WINDOW_MS = 1000;
const TOAST_MAX_LENGTH = 200;

/**
 * A sliding-window limiter for a plugin's toasts: at most TOAST_MAX per
 * TOAST_WINDOW_MS, dropped silently past the cap - a noisy plugin must not be
 * able to fill the screen or thrash the toast store. One limiter per mounted
 * plugin.
 */
export function createToastLimiter(sink: ToastSink, now: () => number = Date.now) {
  let timestamps: number[] = [];
  return (level: unknown, message: string): boolean => {
    const at = now();
    const recent = timestamps.filter((t) => at - t < TOAST_WINDOW_MS);
    if (recent.length >= TOAST_MAX) {
      timestamps = recent;
      return false;
    }
    recent.push(at);
    timestamps = recent;
    const text = message.slice(0, TOAST_MAX_LENGTH);
    if (level === "success") sink.success(text);
    else if (level === "error") sink.error(text);
    else sink.info(text);
    return true;
  };
}

/** What a plugin may send its host. Anything else is ignored. */
export type HostMessage =
  | { type: "ready" }
  | { type: "navigate"; path: string }
  | { type: "toast"; level: unknown; message: string }
  | { type: "resize"; height: number }
  | { type: "rpc"; id: unknown; method: string; args: Record<string, unknown> };

/**
 * Reads one message off the wire. The iframe posts objects, a WebView posts
 * JSON strings; both land here. Null for anything that is not a well-formed
 * message of a known type, so a host never branches on a half-shaped frame.
 */
export function parseHostMessage(raw: unknown): HostMessage | null {
  let data: unknown = raw;
  if (typeof data === "string") {
    try {
      data = JSON.parse(data);
    } catch {
      return null;
    }
  }
  if (!data || typeof data !== "object") return null;
  const m = data as Record<string, unknown>;
  switch (m.type) {
    case "ready":
      return { type: "ready" };
    case "navigate":
      return typeof m.path === "string" ? { type: "navigate", path: m.path } : null;
    case "toast":
      return typeof m.message === "string" ? { type: "toast", level: m.level, message: m.message } : null;
    case "resize":
      return typeof m.height === "number" && m.height >= 0 ? { type: "resize", height: m.height } : null;
    case "rpc":
      return {
        type: "rpc",
        id: m.id,
        method: typeof m.method === "string" ? m.method : "",
        args: (m.args as Record<string, unknown>) ?? {},
      };
    default:
      return null;
  }
}

export interface InitPayloadInput {
  context: Record<string, unknown> | undefined;
  config: Record<string, unknown>;
  locale: string;
  theme: "dark" | "light";
  /** "web", "mobile" or "tv": a plugin lays itself out for the surface it runs on. */
  surface: string;
  /** The height the host recommends; a plugin that never resizes opens at it. */
  suggestedHeight?: number;
  /** The panel height, for a plugin that budgets against the whole screen (TV). */
  viewportHeight?: number;
}

/** The `init` message a host answers `ready` with. */
export function buildInitPayload(input: InitPayloadInput): Record<string, unknown> {
  return {
    type: "init",
    context: input.context ?? {},
    config: input.config,
    locale: input.locale,
    theme: input.theme,
    surface: input.surface,
    ...(input.suggestedHeight !== undefined ? { suggestedHeight: input.suggestedHeight } : {}),
    ...(input.viewportHeight !== undefined ? { viewportHeight: input.viewportHeight } : {}),
  };
}

/**
 * Whether a background colour reads as a light theme, from its hex value -
 * the plugin gets "light" or "dark" and picks its own palette.
 */
export function isLightBackground(hex: string): boolean {
  const bg = hex.replace("#", "");
  if (bg.length < 6) return false;
  const r = parseInt(bg.slice(0, 2), 16);
  const g = parseInt(bg.slice(2, 4), 16);
  const b = parseInt(bg.slice(4, 6), 16);
  return (r * 299 + g * 587 + b * 114) / 1000 > 128;
}
