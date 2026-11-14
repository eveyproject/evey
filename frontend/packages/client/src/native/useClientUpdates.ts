import { useEffect, useState } from "react";
import { AppState } from "react-native";
import { useServerListStore } from "@evey/store";
import {
  CLIENT_UPDATE_SETTLE_MS,
  clientUpdateMarkSettled,
  followClientUpdateSource,
  type ClientUpdateApp,
  type ClientUpdatesApi,
  type ClientUpdateStore,
} from "./clientUpdates";

export interface ClientUpdateBinding {
  updates: ClientUpdatesApi;
  store: ClientUpdateStore;
  app: ClientUpdateApp;
  onError?: (error: unknown) => void;
}

/**
 * Keeps the app's update source on the active server and tells the boot
 * guard when the running bundle settled. `isUpdatePending` comes from
 * expo-updates' own state; the return value says whether to offer a restart.
 */
export function useClientUpdates(binding: ClientUpdateBinding, isUpdatePending: boolean): { offerRestart: boolean; dismiss: () => void } {
  const serverUrl = useServerListStore((s) => (s.activeServerId ? s.servers[s.activeServerId]?.url ?? null : null));
  const [dismissed, setDismissed] = useState(false);
  const { updates, store, app, onError } = binding;

  useEffect(() => {
    if (!serverUrl) return;
    void followClientUpdateSource({ updates, store, serverUrl, app, onError });
  }, [serverUrl, updates, store, app, onError]);

  useEffect(() => {
    const settle = () => clientUpdateMarkSettled({ updates, store });
    const timer = setTimeout(settle, CLIENT_UPDATE_SETTLE_MS);
    // Leaving the app is the user's doing, not a crash: it settles the launch
    // even when it comes before the timer.
    const sub = AppState.addEventListener("change", (state) => {
      if (state !== "active") settle();
    });
    return () => {
      clearTimeout(timer);
      sub.remove();
    };
  }, [updates, store]);

  // A later download is a new offer.
  useEffect(() => {
    if (!isUpdatePending) setDismissed(false);
  }, [isUpdatePending]);

  return { offerRestart: isUpdatePending && !dismissed, dismiss: () => setDismissed(true) };
}
