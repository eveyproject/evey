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
    throw new Error(`${response.status} ${response.statusText}`);
  }

  return response.json() as Promise<T>;
}
