import { useEffect, useState } from "react";
import * as SecureStore from "expo-secure-store";
import { usePlayOnDevices as useSharedPlayOnDevices } from "@evey/client";

// Shared implementation lives in @evey/client (packages/client/src/hooks/usePlayOnDevices.ts).
export { usePlayOnCommand, type PlayOnDevice, type PlayOnCommand } from "@evey/client";

// Same persisted key useDeviceRegistration stamps this install's own device
// id under - read-only here, to drop this phone out of its own Play On list.
const DEVICE_ID_KEY = "evey-device-id";

export function usePlayOnDevices(opts?: { enabled?: boolean }) {
  const [selfId, setSelfId] = useState<string | null>(null);
  useEffect(() => {
    let cancelled = false;
    // An unreadable id only means this install is not filtered out of its own list.
    void SecureStore.getItemAsync(DEVICE_ID_KEY).then((id) => {
      if (!cancelled) setSelfId(id);
    }).catch((err) => console.warn("evey-playon device id", err));
    return () => { cancelled = true; };
  }, []);
  return useSharedPlayOnDevices({ enabled: opts?.enabled, selfId });
}
