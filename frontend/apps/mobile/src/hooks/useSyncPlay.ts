import { useCallback } from "react";
import { eveyFetch } from "@evey/api";
import { useSyncPlaySocket } from "@evey/client";
import { useServerUrl } from "./useServerUrl";

// The room websocket is @evey/client's (useSyncPlaySocket); the REST calls
// that create, join and leave a room are this app's.
export type { SyncPlayMember, SyncPlayState } from "@evey/client";

interface UseSyncPlayOptions {
  onPlay?: (positionMs: number) => void;
  onPause?: (positionMs: number) => void;
  onSeek?: (positionMs: number) => void;
  onMemberJoined?: (userId: string, username: string) => void;
  onMemberLeft?: (userId: string) => void;
  onTrackChange?: (trackIndex: number, mediaId: string) => void;
  // A "watch together" guest has no JWT: it authenticates the room ws with the
  // media-scoped token from POST /syncplay/join-code instead (web parity).
  guestToken?: string;
}

interface GetRoomResponse {
  id?: string;
  code?: string;
  mediaId?: string;
  hostId?: string;
  members?: { userId?: string; username?: string }[];
  queue?: string[];
  trackIndex?: number;
  playlistId?: string;
  mediaType?: string;
}

export function useSyncPlay(opts: UseSyncPlayOptions = {}) {
  const serverUrl = useServerUrl();
  const { state, setState, connect, disconnect, send } = useSyncPlaySocket({ ...opts, serverUrl });

  const createRoom = useCallback(async (mediaId: string, mediaType?: string): Promise<string> => {
    const data = await eveyFetch<{ id: string; code: string }>({
      url: "/api/v1/syncplay/rooms",
      method: "POST",
      data: { mediaId, name: mediaType === "music" ? "Listen Together" : "Watch Together", mediaType },
    });
    setState({ roomId: data.id, roomCode: data.code, members: [], connected: false, isHost: true, queue: [], trackIndex: 0 });
    connect(data.id);
    return data.id;
  }, [connect, setState]);

  const createRoomFromPlaylist = useCallback(async (playlistId: string, name?: string): Promise<string> => {
    const data = await eveyFetch<{ id: string; code: string }>({
      url: "/api/v1/syncplay/rooms",
      method: "POST",
      data: { playlistId, name },
    });
    let queue: string[] = [];
    let trackIndex = 0;
    try {
      const info = await eveyFetch<GetRoomResponse>({
        url: `/api/v1/syncplay/rooms/${data.id}`,
        method: "GET",
      });
      queue = info.queue ?? [];
      trackIndex = info.trackIndex ?? 0;
    } catch (err) {
      // Degraded, not fatal: the room opens with an empty queue. Say so -
      // "the playlist room lost its tracks" is undiagnosable from silence.
      console.warn("[SyncPlay] room info fetch failed, starting with an empty queue:", err);
    }
    setState({ roomId: data.id, roomCode: data.code, members: [], connected: false, isHost: true, queue, trackIndex });
    connect(data.id);
    return data.id;
  }, [connect, setState]);

  const joinByCode = useCallback(async (code: string): Promise<{ roomId: string; mediaId: string }> => {
    const data = await eveyFetch<{ roomId: string; mediaId: string }>({
      url: "/api/v1/syncplay/join",
      method: "POST",
      data: { code },
    });
    let queue: string[] = [];
    let trackIndex = 0;
    try {
      const info = await eveyFetch<GetRoomResponse>({
        url: `/api/v1/syncplay/rooms/${data.roomId}`,
        method: "GET",
      });
      queue = info.queue ?? [];
      trackIndex = info.trackIndex ?? 0;
    } catch (err) {
      console.warn("[SyncPlay] room info fetch failed, joining with an empty queue:", err);
    }
    setState({ roomId: data.roomId, roomCode: null, members: [], connected: false, isHost: false, queue, trackIndex });
    connect(data.roomId);
    return { roomId: data.roomId, mediaId: data.mediaId };
  }, [connect, setState]);

  // Connect to a room the caller already resolved (a guest gets roomId from the
  // join-code response, so it skips the JWT-gated joinByCode POST).
  const joinRoom = useCallback((roomId: string) => {
    setState((prev) => ({ ...prev, roomId, isHost: false }));
    connect(roomId);
  }, [connect, setState]);

  const leaveRoom = useCallback(async () => {
    const roomId = disconnect();
    if (roomId) {
      try {
        await eveyFetch<void>({ url: `/api/v1/syncplay/rooms/${roomId}/leave`, method: "DELETE" });
      } catch (err) {
        // Best-effort: the server expires the membership anyway. Logged so a
        // "ghost member" report can be traced to this failing call.
        console.warn("[SyncPlay] leave room failed (membership will expire server-side):", err);
      }
    }
  }, [disconnect]);

  return {
    state,
    createRoom,
    createRoomFromPlaylist,
    joinByCode,
    joinRoom,
    leaveRoom,
    sendPlay: (ms: number) => send({ type: "play", positionMs: ms }),
    sendPause: (ms: number) => send({ type: "pause", positionMs: ms }),
    sendSeek: (ms: number) => send({ type: "seek", positionMs: ms }),
    sendNext: () => send({ type: "next" }),
    sendPrev: () => send({ type: "prev" }),
    sendSetIndex: (index: number) => send({ type: "setIndex", trackIndex: index }),
    sendResync: (ms: number) => send({ type: "resync", positionMs: ms }),
    sendCatchUp: () => send({ type: "catch_up" }),
  };
}
