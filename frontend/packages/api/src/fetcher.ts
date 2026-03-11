// Custom fetch wrapper used by all orval-generated hooks
// Injects Authorization header from the auth store automatically

let getToken: (() => string | null) | null = null;

export function setTokenProvider(fn: () => string | null) {
  getToken = fn;
}

export interface FetcherRequest {
  url: string;
  method: string;
  headers?: Record<string, string>;
  data?: unknown;
  params?: Record<string, unknown>;
  signal?: AbortSignal;
}

export async function eveyFetch<T>(request: FetcherRequest): Promise<T> {
  const token = getToken?.();
  const headers: Record<string, string> = { ...(request.headers ?? {}) };

  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
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

  const response = await fetch(url, {
    method: request.method,
    headers,
    body: request.data !== undefined ? JSON.stringify(request.data) : undefined,
    signal: request.signal,
  });

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
    const err = new Error(message) as Error & { httpStatus: number };
    err.httpStatus = response.status;
    throw err;
  }

  if (response.status === 204 || response.headers.get("content-length") === "0") {
    return undefined as T;
  }

  return response.json() as Promise<T>;
}
