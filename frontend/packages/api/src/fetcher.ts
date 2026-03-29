// Custom fetch wrapper used by all orval-generated hooks
// Injects Authorization header from the auth store automatically

let getToken: (() => string | null) | null = null;
let updateTokens: ((access: string) => void) | null = null;

function getCSRFToken(): string {
  const match = document.cookie.match(/(?:^|;\s*)_csrf=([^;]*)/);
  return match ? match[1] : "";
}

export function setTokenProvider(fn: () => string | null) { getToken = fn; }
export function setTokensUpdater(fn: (access: string) => void) {
  updateTokens = (token) => { refreshFailed = false; fn(token); };
}

export interface FetcherRequest {
  url: string;
  method: string;
  headers?: Record<string, string>;
  data?: unknown;
  params?: Record<string, unknown>;
  signal?: AbortSignal;
}

let refreshPromise: Promise<string> | null = null;
let refreshFailed = false;

/** Reset module state — for tests only */
export function _resetFetcherState() { refreshPromise = null; refreshFailed = false; }

async function attemptRefresh(): Promise<string> {
  if (!refreshPromise) {
    refreshPromise = (async () => {
      const csrfToken = getCSRFToken();
      const refreshHeaders: Record<string, string> = { "Content-Type": "application/json" };
      if (csrfToken) refreshHeaders["X-CSRF-Token"] = csrfToken;
      const res = await fetch("/api/v1/auth/refresh", {
        method: "POST",
        credentials: "include",
        headers: refreshHeaders,
        body: JSON.stringify({}),
      });
      if (!res.ok) throw Object.assign(new Error("refresh failed"), { httpStatus: res.status });
      const data = await res.json() as { accessToken: string };
      updateTokens?.(data.accessToken);
      return data.accessToken;
    })().finally(() => { refreshPromise = null; });
  }
  return refreshPromise;
}

export interface RawFetcherRequest {
  url: string;
  method: string;
  headers?: Record<string, string>;
  body?: BodyInit;
  signal?: AbortSignal;
}

async function fetchWithRefresh(
  url: string,
  doFetch: (authToken: string | undefined) => Promise<Response>,
  initialToken: string | undefined,
): Promise<Response> {
  let response = await doFetch(initialToken);

  if (response.status === 401 && !url.includes("/auth/refresh")) {
    if (refreshFailed) {
      window.dispatchEvent(new Event("evey:unauthorized"));
      throw Object.assign(new Error("Unauthorized"), { httpStatus: 401 });
    }
    try {
      const newToken = await attemptRefresh();
      refreshFailed = false;
      response = await doFetch(newToken);
    } catch {
      refreshFailed = true;
      window.dispatchEvent(new Event("evey:unauthorized"));
      throw Object.assign(new Error("Unauthorized"), { httpStatus: 401 });
    }
  }

  if (response.status === 401) {
    window.dispatchEvent(new Event("evey:unauthorized"));
  }

  if (!response.ok) {
    let message = `${response.status} ${response.statusText}`;
    try {
      const body = await response.clone().json() as { message?: string };
      if (body.message) message = body.message;
    } catch {
      // ignore parse errors
    }
    throw Object.assign(new Error(message), { httpStatus: response.status });
  }

  return response;
}

export async function eveyFetchRaw(request: RawFetcherRequest): Promise<Response> {
  const token = getToken?.();
  const headers: Record<string, string> = { ...(request.headers ?? {}) };
  if (token) headers["Authorization"] = `Bearer ${token}`;
  const method = request.method.toUpperCase();
  if (method === "POST" || method === "PUT" || method === "PATCH" || method === "DELETE") {
    const csrf = getCSRFToken();
    if (csrf) headers["X-CSRF-Token"] = csrf;
  }

  const doFetch = (authToken: string | undefined) =>
    fetch(request.url, {
      method: request.method,
      headers: authToken ? { ...headers, Authorization: `Bearer ${authToken}` } : headers,
      body: request.body,
      signal: request.signal,
    });

  return fetchWithRefresh(request.url, doFetch, token ?? undefined);
}

export async function eveyFetch<T>(request: FetcherRequest): Promise<T> {
  const token = getToken?.();
  const headers: Record<string, string> = { ...(request.headers ?? {}) };
  if (token) headers["Authorization"] = `Bearer ${token}`;
  const method = request.method.toUpperCase();
  if (method === "POST" || method === "PUT" || method === "PATCH" || method === "DELETE") {
    const csrf = getCSRFToken();
    if (csrf) headers["X-CSRF-Token"] = csrf;
  }

  let url = request.url;
  if (request.params) {
    const qs = new URLSearchParams(
      Object.entries(request.params)
        .filter(([, v]) => v !== undefined && v !== null)
        .map(([k, v]) => [k, String(v)])
    ).toString();
    if (qs) url = `${url}?${qs}`;
  }

  const doFetch = (authToken: string | undefined) =>
    fetch(url, {
      method: request.method,
      headers: authToken ? { ...headers, Authorization: `Bearer ${authToken}` } : headers,
      body: request.data !== undefined ? JSON.stringify(request.data) : undefined,
      signal: request.signal,
    });

  const response = await fetchWithRefresh(url, doFetch, token ?? undefined);

  if (response.status === 204 || response.headers.get("content-length") === "0") {
    return undefined as T;
  }

  // grpc-gateway server-streaming responses are NDJSON (one JSON object per line).
  // Parse the last non-empty line to handle both regular and streaming responses.
  const text = await response.text();
  const lines = text.split("\n").filter((l) => l.trim());
  if (lines.length === 0) return undefined as T;
  return JSON.parse(lines[lines.length - 1]) as T;
}
