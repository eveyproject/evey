import { useCallback, useMemo } from "react";
import { eveyFetch } from "@evey/api";
import { EMPTY_SYNCPLAY_STATE, useSyncPlaySocket } from "./useSyncPlaySocket";

// TV is a SyncPlay JOIN-only client (see the join screen for why room
// creation is deliberately not offered here).

export interface SyncplayJoinResponse {
  roomId: string;
  mediaId: string;
}

// Plain REST helper, deliberately NOT part of the hook: the join screen only
// needs the room lookup (the server's JoinByCode RPC already adds the caller
// as a member - see syncplay_handler.go's h.hub.JoinRoom call - independent
// of any websocket), then it hands off to the player screen via a
// `?roomId=` navigation param. The player's OWN useSyncplay instance opens
// the actual websocket with connect() below once it mounts with that param.
// Doing it this way (REST join now, connect() later on the screen that
// actually has a live player) avoids opening a websocket on the join screen
// just to tear it down a navigation later.
export async function joinSyncplayRoomByCode(code: string): Promise<SyncplayJoinResponse> {
  return eveyFetch<SyncplayJoinResponse>({
    url: "/api/v1/syncplay/join",
    method: "POST",
    data: { code },
  });
}

export type { SyncPlayMember as SyncplayMember, SyncPlayState as SyncplayState } from "./useSyncPlaySocket";

export interface UseSyncplayOptions {
  /** The server the room lives on: each client resolves its own. */
  serverUrl: string;
  // Reads the player's current position at the instant an incoming message
  // is handled, so a sub-second drift only applies the play/pause transition
  // (reconcilePosition, ../lib/syncplayReconcile.ts).
  getLocalPositionMs: () => number;
  onPlay?: (positionMs: number, shouldSeek: boolean) => void;
  onPause?: (positionMs: number, shouldSeek: boolean) => void;
  // A host's explicit seek is deliberate intent, always applied verbatim.
  onSeek?: (positionMs: number) => void;
}

// The televisions join rooms, they never create one: the room websocket is
// the shared one (useSyncPlaySocket), opened by the player screen once it has
// a live player.
export function useSyncplay(opts: UseSyncplayOptions) {
  const { state, setState, connect: open, disconnect, send } = useSyncPlaySocket(opts);

  const connect = useCallback(
    (roomId: string) => {
      setState({ ...EMPTY_SYNCPLAY_STATE, roomId });
      open(roomId);
    },
    [open, setState],
  );

  const leaveRoom = useCallback(async () => {
    const roomId = disconnect();
    if (roomId) {
      try {
        await eveyFetch<void>({ url: `/api/v1/syncplay/rooms/${roomId}/leave`, method: "DELETE" });
      } catch {
        // Best-effort: the server-side janitor sweeps a room whose last
        // member never sent this (see hub.go StartJanitor).
      }
    }
  }, [disconnect]);

  // Sent regardless of host status: the server alone enforces host-only
  // authority (hub.go HandleCommand drops a non-host's play/pause/seek).
  const sendPlay = useCallback((ms: number) => send({ type: "play", positionMs: ms }), [send]);
  const sendPause = useCallback((ms: number) => send({ type: "pause", positionMs: ms }), [send]);

  // Memoised: the player screen feeds this object into the deps of its remote
  // key handler, which must not be re-registered on every render.
  return useMemo(
    () => ({ state, connect, leaveRoom, sendPlay, sendPause }),
    [state, connect, leaveRoom, sendPlay, sendPause],
  );
}
