import { useEffect, useState } from "react";
import * as Network from "expo-network";
import { AppState } from "react-native";

/**
 * Lightweight online detector. Polls `Network.getNetworkStateAsync()` on
 * mount, when the app comes back to the foreground, and once every 15s.
 *
 * Why polling rather than a subscription? `expo-network` doesn't expose
 * a stable change-listener on Android (the `Network.addNetworkStateListener`
 * API was experimental and was dropped in SDK 50+); polling at this rate
 * is cheap and survives airplane-mode toggles in practice.
 *
 * Returns `true` when the OS reports an active connection AND that
 * connection has reachable internet. A device on a wifi captive portal
 * (`isConnected=true, isInternetReachable=false`) reads as offline so the
 * UI doesn't lie about the user's ability to talk to the server.
 */
export function useOnlineStatus(): boolean {
  const [online, setOnline] = useState(true);

  useEffect(() => {
    let cancelled = false;

    const check = async () => {
      try {
        const state = await Network.getNetworkStateAsync();
        if (cancelled) return;
        const ok =
          (state.isConnected ?? false) &&
          (state.isInternetReachable ?? state.isConnected ?? false);
        setOnline(ok);
      } catch {
        // expo-network can throw on the very first call after a cold start;
        // assume online and let the next tick correct us.
        if (!cancelled) setOnline(true);
      }
    };

    void check();
    const interval = setInterval(check, 15_000);
    const sub = AppState.addEventListener("change", (next) => {
      if (next === "active") void check();
    });

    return () => {
      cancelled = true;
      clearInterval(interval);
      sub.remove();
    };
  }, []);

  return online;
}
