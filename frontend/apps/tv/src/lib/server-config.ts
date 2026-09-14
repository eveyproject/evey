import * as SecureStore from "expo-secure-store";

const SERVER_URL_KEY = "evey-server-url";

export async function getServerUrl(): Promise<string | null> {
  return SecureStore.getItemAsync(SERVER_URL_KEY);
}

export async function setServerUrl(url: string): Promise<void> {
  const normalized = url.replace(/\/+$/, "");
  await SecureStore.setItemAsync(SERVER_URL_KEY, normalized);
}

export async function clearServerUrl(): Promise<void> {
  await SecureStore.deleteItemAsync(SERVER_URL_KEY);
}

export async function pingServer(url: string): Promise<boolean> {
  return (await probeServerHealth(url)).ok;
}

// probeServerHealth is pingServer plus the name the server advertises on
// /healthz (admin "Server name"), so the caller that just reached it can keep
// the saved-server row current. A non-JSON 200 still counts as reachable.
export async function probeServerHealth(url: string): Promise<{ ok: boolean; name?: string }> {
  try {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 5000);
    const res = await fetch(`${url}/healthz`, { signal: controller.signal });
    clearTimeout(timeout);
    if (!res.ok) return { ok: false };
    return { ok: true, name: await healthName(res) };
  } catch {
    return { ok: false };
  }
}

// healthName pulls a non-blank string `name` out of a /healthz body.
export async function healthName(res: Response): Promise<string | undefined> {
  try {
    const body: unknown = await res.json();
    const name = (body as { name?: unknown } | null)?.name;
    return typeof name === "string" && name.trim() !== "" ? name.trim() : undefined;
  } catch {
    return undefined;
  }
}
