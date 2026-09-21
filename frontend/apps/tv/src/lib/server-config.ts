// Shared implementation lives in @evey/client (packages/client/src/native/server-config.ts).
// This module keeps the path the screens and their tests import, and the
// shape this app's screens read: {ok, name} rather than the phone's
// {probe, health}.
import { probeServerHealth as probeShared } from "@evey/client/native/server-config";

export { getServerUrl, setServerUrl, clearServerUrl, pingServer, resolveServerUrl, probeServer } from "@evey/client/native/server-config";
export type { ServerProbe } from "@evey/client/native/server-config";

/**
 * pingServer plus the name the server advertises on /healthz (admin "Server
 * name"), so the caller that just reached it can keep the saved-server row
 * current. Only an Evey answer counts as reachable - see serverIdentity.
 */
export async function probeServerHealth(url: string): Promise<{ ok: boolean; name?: string }> {
  const { probe, health } = await probeShared(url);
  return probe === "ok" ? { ok: true, name: health?.name } : { ok: false };
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
