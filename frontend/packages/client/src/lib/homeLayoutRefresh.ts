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

export function refreshHomeLayoutOnForeground(
  queryClient: QueryClient,
  /** Subscribes to the app coming back to the foreground; returns the unsubscribe. */
  onForeground: (listener: () => void) => () => void,
): () => void {
  return onForeground(() => {
    void queryClient.invalidateQueries({ queryKey: FEED_CONFIG_QUERY_KEY });
  });
}
