import { AppState } from "react-native";
import * as SecureStore from "expo-secure-store";
import { useDeviceRegistrationCore, type DeviceRegistrationOptions } from "../lib/deviceRegistrationCore";

export type { DeviceRegistrationOptions };

// Stable per-install ID. Generated once and persisted so reconnects (app
// resume, network blip) are treated by the server as the SAME device rather
// than piling up duplicates in the user's Play On picker.
const DEVICE_ID_KEY = "evey-device-id";

async function getStableDeviceId(): Promise<string> {
  const existing = await SecureStore.getItemAsync(DEVICE_ID_KEY);
  if (existing) return existing;
  const id =
    typeof crypto !== "undefined" && typeof crypto.randomUUID === "function"
      ? crypto.randomUUID()
      : `dev-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`;
  await SecureStore.setItemAsync(DEVICE_ID_KEY, id);
  return id;
}

/** The native Play On registration: the shared core, with this platform's id and foreground signal. */
export function useDeviceRegistration(options: Omit<DeviceRegistrationOptions, "getDeviceId" | "subscribeBackground">) {
  useDeviceRegistrationCore({
    ...options,
    getDeviceId: getStableDeviceId,
    subscribeBackground: (onChange) => {
      const sub = AppState.addEventListener("change", (next) => onChange(next !== "active"));
      return () => sub.remove();
    },
  });
}
