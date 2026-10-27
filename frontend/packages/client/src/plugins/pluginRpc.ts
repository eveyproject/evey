import { adminPluginServiceIssuePluginToken } from "@evey/api";

// The RPC half of the plugin protocol: `evey.prefs`, `evey.media` and
// `evey.assets` calls from the plugin frontend, answered by the host with a
// scoped plugin token the plugin never sees. The server enforces the plugin's
// granted scopes on that token (deny-by-default); this module only decides
// WHICH endpoint a method maps to, and that only this allowlist is reachable.

export interface PluginRequest {
  /** API path, relative to the server (`/api/v1/...`). */
  path: string;
  method: "GET" | "PUT" | "DELETE";
  headers?: Record<string, string>;
  body?: string;
  /**
   * The plugin token for the call, or undefined for the user's own session
   * (media.poster: the artwork handler applies this user's library ACL and
   * parental filter, so a plugin only reaches posters the user can see).
   */
  authToken?: string;
}

export interface PluginRpcTransport {
  /** Sends one request. Each host has its own way to reach the server. */
  request(req: PluginRequest): Promise<Response>;
  /** Issues a scoped token for this plugin. Defaults to the API's endpoint. */
  issueToken?: (pluginId: string) => Promise<string | null>;
  /**
   * Turns a poster into something the plugin can draw. The web downscales
   * through a canvas (a full poster is ~1.3 MB, a wall of them would freeze
   * the UI); a WebView has no canvas on the host side and hands it over as
   * is. Defaults to a data: URL of the blob.
   */
  posterToDataUrl?: (blob: Blob) => Promise<string>;
}

// Strict path guard for a plugin's OWN web_root assets (the server also
// rejects traversal, this is defense in depth).
export function isValidAssetPath(p: string): boolean {
  return !!p && p.length <= 256 && !p.includes("..") && !p.startsWith("/") && !p.includes("\\") && /^[a-zA-Z0-9._/-]+$/.test(p);
}

/** Maps a bridge method to its request. Null for a method outside the allowlist. */
export function pluginRequestFor(pluginId: string, method: string, args: Record<string, unknown>, token: string): PluginRequest | null {
  const enc = encodeURIComponent;
  switch (method) {
    case "prefs.get":
      return { path: `/api/v1/plugin-prefs/${enc(String(args.key ?? ""))}`, method: "GET", authToken: token };
    case "prefs.set":
      return {
        path: `/api/v1/plugin-prefs/${enc(String(args.key ?? ""))}`,
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ value: String(args.value ?? "") }),
        authToken: token,
      };
    case "prefs.delete":
      return { path: `/api/v1/plugin-prefs/${enc(String(args.key ?? ""))}`, method: "DELETE", authToken: token };
    case "prefs.list":
      return { path: `/api/v1/plugin-prefs`, method: "GET", authToken: token };
    case "media.get":
      return { path: `/api/v1/public/media/${enc(String(args.id ?? ""))}`, method: "GET", authToken: token };
    case "media.list":
      return {
        path: `/api/v1/public/media${args.libraryId ? `?libraryId=${enc(String(args.libraryId))}` : ""}`,
        method: "GET",
        authToken: token,
      };
    case "media.poster":
      return { path: `/api/v1/media/${enc(String(args.id ?? ""))}/artwork/poster`, method: "GET" };
    case "assets.read": {
      const p = String(args.path ?? "");
      if (!isValidAssetPath(p)) return null;
      const encPath = p.split("/").map(enc).join("/");
      // The cache-buster predates the server's no-cache+ETag policy; kept as
      // belt-and-braces for intermediaries that ignore revalidation.
      return { path: `/api/v1/plugins/${enc(pluginId)}/assets/${encPath}?_=${Date.now()}`, method: "GET", authToken: token };
    }
    default:
      return null;
  }
}

function blobToDataUrl(blob: Blob): Promise<string> {
  return new Promise<string>((resolve, reject) => {
    const fr = new FileReader();
    fr.onload = () => resolve(String(fr.result));
    fr.onerror = () => reject(new Error("asset read failed"));
    fr.readAsDataURL(blob);
  });
}

/**
 * One RPC endpoint per mounted plugin. Holds the scoped token (lazily issued,
 * re-issued once on a 401) and answers `call(method, args)` with what the
 * plugin's promise resolves to.
 */
export function createPluginRpc(pluginId: string, transport: PluginRpcTransport) {
  let token: string | null = null;
  // The issue in flight: a widget fires several calls at boot and each one
  // issued its own token. The first issue is shared by every caller that finds it.
  let issuing: Promise<string | null> | null = null;
  const issue = transport.issueToken ?? (async (id: string) => (await adminPluginServiceIssuePluginToken(id, {})).token ?? null);

  function ensureToken(): Promise<string | null> {
    if (token) return Promise.resolve(token);
    if (!issuing) {
      issuing = issue(pluginId)
        .catch(() => null)
        .then((t) => {
          token = t;
          issuing = null;
          return t;
        });
    }
    return issuing;
  }

  async function send(method: string, args: Record<string, unknown>, tok: string): Promise<Response> {
    const req = pluginRequestFor(pluginId, method, args, tok);
    if (!req) {
      throw new Error(method === "assets.read" ? "invalid asset path" : `method not permitted: ${method}`);
    }
    return transport.request(req);
  }

  async function call(method: string, args: Record<string, unknown>): Promise<unknown> {
    let tok = await ensureToken();
    if (!tok) throw new Error("plugin token unavailable");
    let res = await send(method, args, tok);
    if (res.status === 401) {
      token = null; // expired - re-issue once
      tok = await ensureToken();
      if (tok) res = await send(method, args, tok);
    }
    if (!res.ok) {
      let msg = String(res.status);
      try {
        const b = (await res.clone().json()) as { message?: string };
        if (b.message) msg = b.message;
      } catch {
        /* no JSON body */
      }
      throw new Error(msg);
    }
    // assets.read returns binary; a data: URL is same-origin inside the
    // sandbox and safe as a WebGL texture.
    if (method === "assets.read") return blobToDataUrl(await res.blob());
    if (method === "media.poster") return (transport.posterToDataUrl ?? blobToDataUrl)(await res.blob());
    const text = await res.text();
    return text ? JSON.parse(text) : {};
  }

  return { call };
}
