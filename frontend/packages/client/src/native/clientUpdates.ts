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
  /** When the running update was released, in milliseconds. */
  releasedAt: number | null;
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
const FLOOR_KEY = "evey.clientUpdate.floor";

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
 * The override is put back at every call: it lives in expo-updates' own
 * storage, which can drift from ours. The download only happens for a new
 * source, since expo-updates already checks the current one at every launch.
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
    updates.setUpdateURLAndRequestHeadersOverride({ updateUrl: url, requestHeaders: {} });
    if (store.getItem(SOURCE_KEY) === url) return "unchanged";
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

/**
 * No server is active any more: stop asking the last one. Without this the
 * app keeps contacting a server the user removed, at every launch and before
 * any sign-in.
 */
export function stopFollowingClientUpdateSource(deps: {
  updates: ClientUpdatesApi;
  store: ClientUpdateStore;
  onError?: (error: unknown) => void;
}): void {
  const { updates, store } = deps;
  if (!updates.enabled) return;
  try {
    updates.setUpdateURLAndRequestHeadersOverride(null);
    if (store.getItem(SOURCE_KEY)) store.setItem(SOURCE_KEY, "");
  } catch (error) {
    deps.onError?.(error);
  }
}

interface GuardState {
  id: string;
  deaths: number;
}

/** The newest release that settled on this device, 0 when none did. */
function readFloor(store: ClientUpdateStore): number {
  const floor = Number(store.getItem(FLOOR_KEY) ?? "0");
  return Number.isFinite(floor) ? floor : 0;
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
 * Also refuses to run a release older than one that already settled here.
 * The native side enforces the same floor before starting an update; this
 * is the second lock, kept by the code that is being protected.
 *
 * Never throws for the bundle built into the app: there is nothing behind it.
 */
export function clientUpdateBootGuard(deps: { updates: ClientUpdatesApi; store: ClientUpdateStore }): void {
  const { updates, store } = deps;
  if (!updates.enabled || updates.isEmbeddedLaunch || !updates.updateId) return;
  const id = updates.updateId;
  let deaths = 0;
  let floor = 0;
  try {
    floor = readFloor(store);
    const state = readGuard(store);
    deaths = state?.id === id ? state.deaths : 0;
    // Written before deciding: if this launch is the one that asks for the
    // rollback, the count restarts for whatever runs next.
    store.setItem(GUARD_KEY, JSON.stringify({ id, deaths: deaths >= CLIENT_UPDATE_MAX_EARLY_DEATHS ? 0 : deaths + 1 }));
  } catch {
    // A store that cannot be read or written must not take the app down.
    return;
  }
  if (updates.releasedAt !== null && updates.releasedAt < floor) {
    throw new Error(`${CLIENT_UPDATE_ROLLBACK_MARKER}: update ${id} is older than a release that already ran on this device`);
  }
  if (deaths >= CLIENT_UPDATE_MAX_EARLY_DEATHS) {
    throw new Error(`${CLIENT_UPDATE_ROLLBACK_MARKER}: update ${id} ended ${deaths} launches in a row without settling`);
  }
}

/**
 * Records that the running update settled: it stayed up, or the user left the
 * app on their own. Clears the count the boot guard keeps, and makes this
 * release the oldest one the guard lets run.
 */
export function clientUpdateMarkSettled(deps: { updates: ClientUpdatesApi; store: ClientUpdateStore }): void {
  const { updates, store } = deps;
  if (!updates.enabled || updates.isEmbeddedLaunch || !updates.updateId) return;
  try {
    if (updates.releasedAt !== null && updates.releasedAt > readFloor(store)) {
      store.setItem(FLOOR_KEY, String(updates.releasedAt));
    }
    const state = readGuard(store);
    // Nothing to clear: spare the keystore a write on every exit.
    if (state?.id === updates.updateId && state.deaths === 0) return;
    store.setItem(GUARD_KEY, JSON.stringify({ id: updates.updateId, deaths: 0 }));
  } catch {
    // best effort
  }
}

/**
 * Arms what settles a launch: the timer, and the user leaving the app. Call
 * right after the boot guard, outside React. A screen that never mounts (a
 * server that does not answer) must not turn a healthy bundle into one that
 * "died early" because the user pressed Home.
 */
export function watchClientUpdateSettle(deps: {
  updates: ClientUpdatesApi;
  store: ClientUpdateStore;
  /** Calls back whenever the app leaves the foreground. */
  onLeave: (listener: () => void) => void;
  schedule?: (callback: () => void, ms: number) => void;
}): void {
  const { updates, store } = deps;
  if (!updates.enabled || updates.isEmbeddedLaunch || !updates.updateId) return;
  const settle = () => clientUpdateMarkSettled({ updates, store });
  (deps.schedule ?? ((callback, ms) => void setTimeout(callback, ms)))(settle, CLIENT_UPDATE_SETTLE_MS);
  deps.onLeave(settle);
}
