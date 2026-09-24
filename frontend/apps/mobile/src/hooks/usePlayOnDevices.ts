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
    void SecureStore.getItemAsync(DEVICE_ID_KEY).then((id) => {
      if (!cancelled) setSelfId(id);
    });
    return () => { cancelled = true; };
  }, []);
  return useSharedPlayOnDevices({ enabled: opts?.enabled, selfId });
}
