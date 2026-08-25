import { useEffect, useState } from "react";
import { useQuery, useMutation } from "@tanstack/react-query";
import * as SecureStore from "expo-secure-store";
import { eveyFetch } from "@evey/api";

export interface PlayOnDevice {
  id: string;
  name: string;
  type: string;
  connectedAt: string;
}

const QUERY_KEY = ["play-on-devices"] as const;

// Same persisted key useDeviceRegistration.ts stamps this install's own
// device id under - read-only here, just to drop this device out of its own
// Play On list ("play on myself" makes no sense as a target).
const DEVICE_ID_KEY = "evey-device-id";

// Mirrors apps/web/src/hooks/usePlayOnDevices.ts. Lists the user's
// currently-connected Play On targets (first-party clients holding a
// persistent WebSocket - TV app, another phone, a browser tab), minus this
// install itself.
export function usePlayOnDevices(opts?: { enabled?: boolean }) {
  const [selfId, setSelfId] = useState<string | null>(null);
  useEffect(() => {
    let cancelled = false;
    void SecureStore.getItemAsync(DEVICE_ID_KEY).then((id) => {
      if (!cancelled) setSelfId(id);
    });
    return () => { cancelled = true; };
  }, []);

  return useQuery({
    queryKey: QUERY_KEY,
    queryFn: () =>
      eveyFetch<{ devices: PlayOnDevice[] }>({ url: "/api/v1/devices", method: "GET" }).then(
        (r) => r.devices ?? [],
      ),
    select: (devices) => (selfId ? devices.filter((d) => d.id !== selfId) : devices),
    // Devices come and go on their own (TV powered off, app closed) - a
    // 10-second refetch keeps the picker in sync without spamming. The poll
    // only runs while a caller says the picker is actually on screen: the
    // sheet is mounted (hidden) on every media page, and an unconditional
    // interval polled /api/v1/devices 6x/min for a UI nobody had opened.
    enabled: opts?.enabled ?? true,
    refetchInterval: 10_000,
    staleTime: 5_000,
  });
}

export interface PlayOnCommand {
  type: "loadAndPlay" | "play" | "pause" | "seek";
  mediaId?: string;
  positionMs?: number;
}

// Sends a playback command to one of the user's devices. A 404 means the
// device has disconnected since the sheet was opened - the caller can
// refetch the device list to refresh the UI.
export function usePlayOnCommand() {
  return useMutation({
    mutationFn: ({ deviceId, command }: { deviceId: string; command: PlayOnCommand }) =>
      eveyFetch<{ message: string }>({
        url: `/api/v1/devices/${encodeURIComponent(deviceId)}/command`,
        method: "POST",
        data: command,
      }),
  });
}
