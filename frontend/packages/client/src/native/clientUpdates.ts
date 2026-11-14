// The phone and TV apps load their JavaScript from the Evey instance they are
// connected to, so a client runs the bundle built with its server. This file
// is the app-side logic; it touches expo-updates and the key-value store only
// through the two interfaces below, which each app binds to the real modules.
//
// Bundles are signed by the project key and the apps verify them against the
// certificate compiled in: an instance can only hand out published code.

export type ClientUpdateApp = "mobile" | "tv";

/** The subset of expo-updates this logic needs. */
export interface ClientUpdatesApi {
  /** False in development builds: expo-updates is then inert. */
  enabled: boolean;
  updateId: string | null;
  isEmbeddedLaunch: boolean;
  setUpdateURLAndRequestHeadersOverride(
    override: { updateUrl: string; requestHeaders: Record<string, string> } | null,
  ): void;
  checkForUpdateAsync(): Promise<{ isAvailable: boolean }>;
  fetchUpdateAsync(): Promise<{ isNew: boolean }>;
}

/** Synchronous key-value store: the boot guard runs before anything can await. */
export interface ClientUpdateStore {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
}

const SOURCE_KEY = "evey.clientUpdate.source";
const GUARD_KEY = "evey.clientUpdate.guard";

/**
 * Thrown by the boot guard. The patched expo-updates recognises it in a fatal
 * error raised before the first render and drops the running update for the
 * previous one (or the bundle built into the app).
 */
export const CLIENT_UPDATE_ROLLBACK_MARKER = "EVEY_CLIENT_UPDATE_ROLLBACK";

/** An update that dies this many launches in a row without settling is dropped. */
export const CLIENT_UPDATE_MAX_EARLY_DEATHS = 3;

/** How long a launch must stay up before it counts as settled. */
export const CLIENT_UPDATE_SETTLE_MS = 10_000;

export function clientUpdateManifestUrl(serverUrl: string, app: ClientUpdateApp): string {
  return `${serverUrl.replace(/\/+$/, "")}/api/v1/client-updates/${app}/manifest`;
}

export type ClientUpdateSourceResult = "disabled" | "unchanged" | "up-to-date" | "ready" | "failed";

/**
 * Points the app at `serverUrl` for its updates and, when that is a new
 * source, downloads what the instance offers. "ready" means a bundle is
 * waiting for the next start (or a reload).
 *
 * Nothing happens while the source is unchanged: expo-updates already checks
 * it at every launch.
 */
export async function followClientUpdateSource(deps: {
  updates: ClientUpdatesApi;
  store: ClientUpdateStore;
  serverUrl: string;
  app: ClientUpdateApp;
  onError?: (error: unknown) => void;
}): Promise<ClientUpdateSourceResult> {
  const { updates, store, serverUrl, app } = deps;
  if (!updates.enabled) return "disabled";
  const url = clientUpdateManifestUrl(serverUrl, app);
  try {
    if (store.getItem(SOURCE_KEY) === url) return "unchanged";
    updates.setUpdateURLAndRequestHeadersOverride({ updateUrl: url, requestHeaders: {} });
    // Recorded once the override is in place and before the network: a
    // failed download must not make every later launch redo the switch, the
    // launch-time check picks it up from here.
    store.setItem(SOURCE_KEY, url);
    const check = await updates.checkForUpdateAsync();
    if (!check.isAvailable) return "up-to-date";
    const fetched = await updates.fetchUpdateAsync();
    return fetched.isNew ? "ready" : "up-to-date";
  } catch (error) {
    deps.onError?.(error);
    return "failed";
  }
}

interface GuardState {
  id: string;
  deaths: number;
}

function readGuard(store: ClientUpdateStore): GuardState | null {
  try {
    const parsed: unknown = JSON.parse(store.getItem(GUARD_KEY) ?? "null");
    const state = parsed as Partial<GuardState> | null;
    if (state && typeof state.id === "string" && typeof state.deaths === "number") {
      return { id: state.id, deaths: state.deaths };
    }
  } catch {
    // unreadable state counts as no state
  }
  return null;
}

/**
 * Call synchronously at the very start of the bundle, before the app renders.
 *
 * Counts the launches of a downloaded update that ended without settling. An
 * update that renders and then crashes at every launch is invisible to
 * expo-updates, which only rolls back crashes that happen before the first
 * render; this guard turns the next launch into exactly that, by throwing.
 *
 * Never throws for the bundle built into the app: there is nothing behind it.
 */
export function clientUpdateBootGuard(deps: { updates: ClientUpdatesApi; store: ClientUpdateStore }): void {
  const { updates, store } = deps;
  if (!updates.enabled || updates.isEmbeddedLaunch || !updates.updateId) return;
  const id = updates.updateId;
  let deaths = 0;
  try {
    const state = readGuard(store);
    deaths = state?.id === id ? state.deaths : 0;
    // Written before deciding: if this launch is the one that asks for the
    // rollback, the count restarts for whatever runs next.
    store.setItem(GUARD_KEY, JSON.stringify({ id, deaths: deaths >= CLIENT_UPDATE_MAX_EARLY_DEATHS ? 0 : deaths + 1 }));
  } catch {
    // A store that cannot be read or written must not take the app down.
    return;
  }
  if (deaths >= CLIENT_UPDATE_MAX_EARLY_DEATHS) {
    throw new Error(`${CLIENT_UPDATE_ROLLBACK_MARKER}: update ${id} ended ${deaths} launches in a row without settling`);
  }
}

/**
 * Records that the running update settled: it stayed up, or the user left the
 * app on their own. Clears the count the boot guard keeps.
 */
export function clientUpdateMarkSettled(deps: { updates: ClientUpdatesApi; store: ClientUpdateStore }): void {
  const { updates, store } = deps;
  if (!updates.enabled || updates.isEmbeddedLaunch || !updates.updateId) return;
  try {
    store.setItem(GUARD_KEY, JSON.stringify({ id: updates.updateId, deaths: 0 }));
  } catch {
    // best effort
  }
}
