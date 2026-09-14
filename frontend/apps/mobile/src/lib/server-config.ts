import * as SecureStore from "expo-secure-store";
import { parseEveyHealth, type EveyHealth } from "./serverIdentity";

const SERVER_URL_KEY = "evey-server-url";

export async function getServerUrl(): Promise<string | null> {
  return SecureStore.getItemAsync(SERVER_URL_KEY);
}

// resolveServerUrl is THE way to learn which server this app talks to.
//
// There used to be two answers: the boot read the multi-server store first
// (falling back to the legacy SecureStore key), while initFetcher read ONLY
// the legacy key. Any divergence - removing the active server, a half-applied
// migration - left the API layer pointed at nothing (or at the wrong host)
// while the rest of the app looked healthy: every call then failed with a bare
// network error and, crucially, NOTHING reached the real server's logs
// (2026-07-29). One resolver, one answer.
export async function resolveServerUrl(
  activeServerUrl: string | null | undefined,
): Promise<string | null> {
  if (activeServerUrl) return activeServerUrl;
  return getServerUrl();
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
  return (await probeServerHealth(url, timeoutMs)).probe;
}

// probeServerHealth is probeServer plus the health payload on "ok", so a
// caller that just proved the server can also record the name it advertises.
export async function probeServerHealth(
  url: string,
  timeoutMs = 5000,
): Promise<{ probe: ServerProbe; health: EveyHealth | null }> {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const res = await fetch(`${url}/healthz`, { signal: controller.signal });
    if (!res.ok) return { probe: "unreachable", health: null };
    const body: unknown = await res.json().catch(() => null);
    const health = parseEveyHealth(body);
    return health ? { probe: "ok", health } : { probe: "not-evey", health: null };
  } catch {
    return { probe: "unreachable", health: null };
  } finally {
    clearTimeout(timeout);
  }
}
