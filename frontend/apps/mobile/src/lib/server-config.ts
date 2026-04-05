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

const CAST_BASE_URL_KEY = "evey-cast-base-url";

export async function getCastBaseUrl(): Promise<string | null> {
  return SecureStore.getItemAsync(CAST_BASE_URL_KEY);
}

export async function setCastBaseUrl(url: string): Promise<void> {
  const normalized = url.replace(/\/+$/, "");
  await SecureStore.setItemAsync(CAST_BASE_URL_KEY, normalized);
}

export async function clearCastBaseUrl(): Promise<void> {
  await SecureStore.deleteItemAsync(CAST_BASE_URL_KEY);
}

export async function pingServer(url: string): Promise<boolean> {
  try {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 5000);
    const res = await fetch(`${url}/healthz`, { signal: controller.signal });
    clearTimeout(timeout);
    return res.ok;
  } catch {
    return false;
  }
}
