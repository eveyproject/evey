import { useSyncExternalStore } from "react";
import { onlineManager } from "@tanstack/react-query";

/**
 * Reads the browser's own connectivity flag. Missing in some non-browser
 * environments (older jsdom, non-window contexts); default to `true` there
 * rather than invent an offline state we cannot justify - the same
 * fail-open choice the mobile bridge makes on a probe error.
 */
export function readNavigatorOnline(): boolean {
  if (typeof navigator === "undefined" || typeof navigator.onLine !== "boolean") return true;
  return navigator.onLine;
}

/**
 * Wires the browser's connectivity signal into React Query's onlineManager,
 * mirroring the mobile bridge (apps/mobile/src/lib/online-manager.ts) so
 * `refetchOnReconnect` behaves the same on both platforms.
 *
 * React Query's own browser setup already listens for `online`/`offline`
 * window events, but it seeds its internal flag to `true` unconditionally and
 * only corrects it on the next event. A tab opened (or restored from bfcache)
 * while already offline never gets an `offline` event - nothing changed since
 * load - so every query would keep retrying against a network that isn't
 * there. Reading `navigator.onLine` once at setup, on top of the same
 * `online`/`offline` listeners, closes that gap.
 */
export function setupOnlineManager(): void {
  onlineManager.setEventListener((setOnline) => {
    setOnline(readNavigatorOnline());

    const onlineListener = () => setOnline(true);
    const offlineListener = () => setOnline(false);
    window.addEventListener("online", onlineListener);
    window.addEventListener("offline", offlineListener);

    return () => {
      window.removeEventListener("online", onlineListener);
      window.removeEventListener("offline", offlineListener);
    };
  });
}

/**
 * Whether the browser currently believes it has connectivity. Reads React
 * Query's onlineManager (fed by `setupOnlineManager`, called once at app
 * start) rather than `navigator.onLine` directly, so the offline banner and
 * the query layer can never disagree.
 */
export function useOnlineStatus(): boolean {
  return useSyncExternalStore(
    (onChange) => onlineManager.subscribe(onChange),
    () => onlineManager.isOnline(),
    () => true,
  );
}
