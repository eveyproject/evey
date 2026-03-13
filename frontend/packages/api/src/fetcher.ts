// Custom fetch wrapper used by all orval-generated hooks
// Injects Authorization header from the auth store automatically

let getToken: (() => string | null) | null = null;
let updateTokens: ((access: string) => void) | null = null;

export function setTokenProvider(fn: () => string | null) { getToken = fn; }
export function setTokensUpdater(fn: (access: string) => void) { updateTokens = fn; }

export interface FetcherRequest {
  url: string;
  method: string;
  headers?: Record<string, string>;
  data?: unknown;
  params?: Record<string, unknown>;
  signal?: AbortSignal;
}

let refreshPromise: Promise<string> | null = null;

/** Reset module state — for tests only */
export function _resetFetcherState() { refreshPromise = null; }

async function attemptRefresh(): Promise<string> {
  if (!refreshPromise) {
    refreshPromise = (async () => {
      const res = await fetch("/api/v1/auth/refresh", {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
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

export async function eveyFetch<T>(request: FetcherRequest): Promise<T> {
  const token = getToken?.();
  const headers: Record<string, string> = { ...(request.headers ?? {}) };
  if (token) headers["Authorization"] = `Bearer ${token}`;

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

  let response = await doFetch(token ?? undefined);

  if (response.status === 401 && !url.includes("/auth/refresh")) {
    try {
      const newToken = await attemptRefresh();
      response = await doFetch(newToken);
    } catch {
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
      const body = await response.json() as { message?: string; code?: number };
      if (body.message) message = body.message;
    } catch {
      // ignore parse errors
    }
    const err = Object.assign(new Error(message), { httpStatus: response.status });
    throw err;
  }

  if (response.status === 204 || response.headers.get("content-length") === "0") {
    return undefined as T;
  }

  return response.json() as Promise<T>;
}
