import { useSyncExternalStore } from "react";
import { onlineManager } from "@tanstack/react-query";

/**
 * Whether the device can currently reach the server.
 *
 * Reads React Query's onlineManager instead of polling the OS itself. The
 * manager is fed by `setupOnlineManager()` (src/lib/online-manager.ts), which
 * runs once at app start.
 *
 * This hook used to own its own poller, and it was the only consumer of the
 * answer: React Query never learned the device had gone offline, so
 * `refetchOnReconnect` never fired and coming out of a tunnel left every screen
 * stale until a manual pull. Sharing one source also removes the window where
 * the offline banner and the query layer disagreed.
 *
 * Returns `false` on a captive portal (connected, no reachable internet): see
 * online-manager.ts for why that counts as offline.
 */
export function useOnlineStatus(): boolean {
  return useSyncExternalStore(
    (onChange) => onlineManager.subscribe(onChange),
    () => onlineManager.isOnline(),
    () => true,
  );
}
