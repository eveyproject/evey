import { onlineManager } from "@tanstack/react-query";
import * as Network from "expo-network";
import { AppState } from "react-native";

/** How often the network state is re-read while the app is in the foreground. */
const POLL_MS = 15_000;

/**
 * Reads the OS network state the way the app defines "online": connected AND
 * with reachable internet. A device sitting on a wifi captive portal reports
 * `isConnected=true, isInternetReachable=false`, and treating that as online is
 * how an app ends up insisting it can talk to a server it cannot reach.
 */
async function isOnline(): Promise<boolean> {
  try {
    const state = await Network.getNetworkStateAsync();
    return (state.isConnected ?? false) && (state.isInternetReachable ?? state.isConnected ?? false);
  } catch {
    // expo-network can throw on the very first call after a cold start.
    // Assume online and let the next tick correct us: a false "offline" would
    // pause every query.
    return true;
  }
}

/**
 * Feeds the OS network state into React Query's onlineManager.
 *
 * Without this bridge `refetchOnReconnect` never fires, because React Query has
 * no idea the device went offline: it defaults to a browser `online` event that
 * React Native does not emit. The visible symptom is coming out of a tunnel to
 * stale screens that only refresh on a manual pull.
 *
 * Polling rather than a subscription because expo-network has no stable change
 * listener on Android (the experimental one was dropped in SDK 50+). The
 * foreground-transition check is what makes it feel immediate: returning to the
 * app is when a user notices staleness, and it is also the moment connectivity
 * has most likely changed.
 */
export function setupOnlineManager(): void {
  onlineManager.setEventListener((setOnline) => {
    let cancelled = false;

    const check = async () => {
      const ok = await isOnline();
      if (!cancelled) setOnline(ok);
    };

    void check();
    const interval = setInterval(() => void check(), POLL_MS);
    const sub = AppState.addEventListener("change", (next) => {
      if (next === "active") void check();
    });

    return () => {
      cancelled = true;
      clearInterval(interval);
      sub.remove();
    };
  });
}
