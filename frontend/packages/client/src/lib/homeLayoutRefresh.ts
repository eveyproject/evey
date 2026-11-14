import type { QueryClient } from "@tanstack/react-query";

// The home layout lives on the server, per profile, but each client kept the
// copy it fetched: the phone and the television never refetch on their own
// (the home tab stays mounted), the web has refetchOnWindowFocus off. A layout
// changed on one device therefore did not show on the others until a restart.
// Coming back to the app is when the viewer looks, so that is when it reloads.
export const FEED_CONFIG_QUERY_KEY = ["/api/v1/feeds/config"] as const;

/** The browser's "back to the foreground": the page becoming visible again. */
export function onDocumentVisible(listener: () => void): () => void {
  const handler = () => {
    if (document.visibilityState === "visible") listener();
  };
  document.addEventListener("visibilitychange", handler);
  return () => document.removeEventListener("visibilitychange", handler);
}

// The profile list carries the theme, the languages and the playback
// preferences: everything a viewer sets once and expects on every device.
const PROFILE_LIST_KEY = /^\/api\/v1\/users\/[^/]+\/profiles$/;

/** What a settingsChanged push names; anything else reloads both. */
export type SettingsScope = "profile" | "homeLayout";

/**
 * Marks the settings another device may have changed as stale, so every
 * screen showing them reloads. Without a scope, both the profile and the
 * home layout reload.
 */
export function refreshSettings(queryClient: QueryClient, scope?: string): void {
  if (scope !== "homeLayout") {
    void queryClient.invalidateQueries({
      predicate: (q) => typeof q.queryKey[0] === "string" && PROFILE_LIST_KEY.test(q.queryKey[0]),
    });
  }
  if (scope !== "profile") {
    void queryClient.invalidateQueries({ queryKey: FEED_CONFIG_QUERY_KEY });
  }
}

/**
 * Reloads the settings when the app comes back to the foreground: a phone
 * closes its socket in the background and a hidden tab may have missed the
 * push, and coming back is when the viewer looks.
 */
export function refreshSettingsOnForeground(
  queryClient: QueryClient,
  /** Subscribes to the app coming back to the foreground; returns the unsubscribe. */
  onForeground: (listener: () => void) => () => void,
): () => void {
  return onForeground(() => refreshSettings(queryClient));
}

type SettingsListener = (scope: string | undefined) => void;
const settingsListeners = new Set<SettingsListener>();

/**
 * Announces, inside this app, that a setting changed on another device. The
 * devices socket calls it; it knows nothing of the query cache.
 */
export function emitSettingsChanged(scope: string | undefined): void {
  for (const listener of settingsListeners) listener(scope);
}

export function subscribeSettingsChanged(listener: SettingsListener): () => void {
  settingsListeners.add(listener);
  return () => { settingsListeners.delete(listener); };
}

/**
 * Keeps the settings this client shows in step with the server: reloaded the
 * moment another device changes one, and again on return to the foreground
 * for whatever was missed meanwhile.
 */
export function followSettingsChanges(
  queryClient: QueryClient,
  onForeground: (listener: () => void) => () => void,
): () => void {
  const offPush = subscribeSettingsChanged((scope) => refreshSettings(queryClient, scope));
  const offForeground = refreshSettingsOnForeground(queryClient, onForeground);
  return () => { offPush(); offForeground(); };
}
