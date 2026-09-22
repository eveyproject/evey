import { useDeviceRegistrationCore, type DeviceRegistrationOptions } from "../lib/deviceRegistrationCore";

// The browser's stable per-install id. A television browser keeps its local
// storage across restarts, so the Play On picker shows one entry, not one per
// power cycle. A private window that refuses storage falls back to an id that
// lives as long as the page.
const DEVICE_ID_KEY = "evey-device-id";
let memoryId = "";

function newId(): string {
  return typeof crypto !== "undefined" && typeof crypto.randomUUID === "function"
    ? crypto.randomUUID()
    : `dev-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`;
}

export async function browserDeviceId(): Promise<string> {
  try {
    const existing = localStorage.getItem(DEVICE_ID_KEY);
    if (existing) return existing;
    const id = newId();
    localStorage.setItem(DEVICE_ID_KEY, id);
    return id;
  } catch {
    memoryId = memoryId || newId();
    return memoryId;
  }
}

/** The Play On registration for a browser client, on the shared core. */
export function useDeviceRegistration(options: Omit<DeviceRegistrationOptions, "getDeviceId" | "subscribeBackground">) {
  useDeviceRegistrationCore({
    ...options,
    getDeviceId: browserDeviceId,
    subscribeBackground: (onChange) => {
      const handler = () => onChange(document.visibilityState === "hidden");
      document.addEventListener("visibilitychange", handler);
      return () => document.removeEventListener("visibilitychange", handler);
    },
  });
}
