import {
  checkForUpdateAsync,
  fetchUpdateAsync,
  isEmbeddedLaunch,
  isEnabled,
  setUpdateURLAndRequestHeadersOverride,
  updateId,
} from "expo-updates";
import * as SecureStore from "expo-secure-store";
import type { ClientUpdateBinding } from "@evey/client/native/useClientUpdates";

/** expo-updates and the keystore, bound for the shared update logic. */
export const clientUpdateBinding: ClientUpdateBinding = {
  updates: {
    enabled: isEnabled,
    updateId,
    isEmbeddedLaunch,
    setUpdateURLAndRequestHeadersOverride,
    checkForUpdateAsync,
    fetchUpdateAsync,
  },
  store: {
    getItem: (key) => SecureStore.getItem(key),
    setItem: (key, value) => SecureStore.setItem(key, value),
  },
  app: "mobile",
  onError: (error) => console.warn(`[update] client update: ${String(error)}`),
};
