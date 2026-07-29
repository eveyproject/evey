import * as SecureStore from "expo-secure-store";
import { parseEveyHealth } from "./serverIdentity";

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

// pingServer answers "is an EVEY server reachable at url" - not merely "does
// something answer 200 there". See serverIdentity.ts for why the distinction
// cost a beta tester an afternoon.
export async function pingServer(url: string): Promise<boolean> {
  return (await probeServer(url)) === "ok";
}

export type ServerProbe = "ok" | "not-evey" | "unreachable";

// probeServer distinguishes the two failures the UI must NOT conflate: a host
// that never answers, and a host that answers but is not Evey (captive
// portal, ISP DNS hijack, router page).
export async function probeServer(url: string, timeoutMs = 5000): Promise<ServerProbe> {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const res = await fetch(`${url}/healthz`, { signal: controller.signal });
    if (!res.ok) return "unreachable";
    const body: unknown = await res.json().catch(() => null);
    return parseEveyHealth(body) ? "ok" : "not-evey";
  } catch {
    return "unreachable";
  } finally {
    clearTimeout(timeout);
  }
}
