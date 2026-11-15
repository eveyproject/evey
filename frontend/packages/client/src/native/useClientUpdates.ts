import { useEffect, useState } from "react";
import { useServerListStore } from "@evey/store";
import {
  followClientUpdateSource,
  stopFollowingClientUpdateSource,
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
 * Keeps the app's update source on the active server, and off any server
 * once none is active. `isUpdatePending` comes from expo-updates' own state;
 * the return value says whether to offer a restart.
 */
export function useClientUpdates(binding: ClientUpdateBinding, isUpdatePending: boolean): { offerRestart: boolean; dismiss: () => void } {
  const serverUrl = useServerListStore((s) => (s.activeServerId ? s.servers[s.activeServerId]?.url ?? null : null));
  const [dismissed, setDismissed] = useState(false);
  const { updates, store, app, onError } = binding;

  useEffect(() => {
    if (!serverUrl) {
      stopFollowingClientUpdateSource({ updates, store, onError });
      return;
    }
    void followClientUpdateSource({ updates, store, serverUrl, app, onError });
  }, [serverUrl, updates, store, app, onError]);

  // A later download is a new offer.
  useEffect(() => {
    if (!isUpdatePending) setDismissed(false);
  }, [isUpdatePending]);

  return { offerRestart: isUpdatePending && !dismissed, dismiss: () => setDismissed(true) };
}
