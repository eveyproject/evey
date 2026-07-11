import { useQuery, useMutation } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";

export interface PlayOnDevice {
  id: string;
  name: string;
  type: string;
  connectedAt: string;
}

const QUERY_KEY = ["play-on-devices"] as const;

// Lists the user's currently-connected Play On targets (first-party
// clients holding a persistent WebSocket - TV app, mobile, another browser
// tab). The list is per-user; refreshing on demand because devices
// register/unregister out-of-band as they (re)connect.
export function usePlayOnDevices() {
  return useQuery({
    queryKey: QUERY_KEY,
    queryFn: () => eveyFetch<{ devices: PlayOnDevice[] }>({ url: "/api/v1/devices", method: "GET" })
      .then((r) => r.devices ?? []),
    // Devices come and go on their own (TV powered off, app closed) -
    // a 10-second refetch keeps the picker in sync without spamming.
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
// device has disconnected since the picker was opened - the caller can
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
