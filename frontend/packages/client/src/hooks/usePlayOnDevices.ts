import { useQuery, useMutation } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";

export interface PlayOnDevice {
  id: string;
  name: string;
  type: string;
  connectedAt: string;
}

const QUERY_KEY = ["play-on-devices"] as const;

export interface PlayOnDevicesOptions {
  // Poll only while a picker is on screen; a hidden sheet mounted on every
  // media page polled /api/v1/devices 6x/min for a UI nobody had opened.
  enabled?: boolean;
  // This install's own device id: "play on myself" makes no sense as a target.
  selfId?: string | null;
}

// Lists the user's currently-connected Play On targets (first-party clients
// holding a persistent WebSocket - TV app, phone, a browser tab). Devices come
// and go on their own (TV powered off, app closed): a 10-second refetch keeps
// the picker in sync without spamming.
export function usePlayOnDevices(opts: PlayOnDevicesOptions = {}) {
  const { selfId } = opts;
  return useQuery({
    queryKey: QUERY_KEY,
    queryFn: () =>
      eveyFetch<{ devices: PlayOnDevice[] }>({ url: "/api/v1/devices", method: "GET" }).then((r) => r.devices ?? []),
    select: (devices) => (selfId ? devices.filter((d) => d.id !== selfId) : devices),
    enabled: opts.enabled ?? true,
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
