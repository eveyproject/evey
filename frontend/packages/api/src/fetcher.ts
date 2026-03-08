// Custom fetch wrapper used by all orval-generated hooks
// Injects Authorization header from the auth store automatically

let getToken: (() => string | null) | null = null;

export function setTokenProvider(fn: () => string | null) {
  getToken = fn;
}

export async function eveyFetch<T>(
  url: string,
  options: RequestInit = {}
): Promise<T> {
  const token = getToken?.();
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...(options.headers as Record<string, string>),
  };
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }

  const response = await fetch(url, { ...options, headers });

  if (response.status === 401) {
    // token expired — trigger logout via store
    window.dispatchEvent(new Event("evey:unauthorized"));
  }

  if (!response.ok) {
    throw new Error(`${response.status} ${response.statusText}`);
  }

  return response.json() as Promise<T>;
}
