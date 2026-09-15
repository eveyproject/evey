import { useCallback, useEffect, useRef, useState } from "react";
import { useAuthStore } from "@evey/store";
import { eveyFetch } from "@evey/api";
import { useServerUrl } from "./useServerUrl";

export interface SyncPlayMember {
  userId: string;
  username: string;
}

export interface SyncPlayState {
  roomId: string | null;
  roomCode: string | null;
  members: SyncPlayMember[];
  connected: boolean;
  isHost: boolean;
  queue: string[];
  trackIndex: number;
}

interface SyncMessage {
  type: string;
  positionMs?: number;
  userId?: string;
  username?: string;
  playing?: boolean;
  trackIndex?: number;
  mediaId?: string;
}

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
  const [state, setState] = useState<SyncPlayState>({ roomId: null, roomCode: null, members: [], connected: false, isHost: false, queue: [], trackIndex: 0 });
  const wsRef = useRef<WebSocket | null>(null);
  const optsRef = useRef(opts);
  optsRef.current = opts;
  const leftVoluntarilyRef = useRef(false);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const reconnectAttemptRef = useRef(0);
  const messageQueueRef = useRef<string[]>([]);

  const connect = useCallback((roomId: string) => {
    // Cancel any reconnect already scheduled by a previous connection's onclose,
    // and detach the old socket's onclose before the deliberate close - otherwise
    // switching rooms fires a stale reconnect to the OLD room (the guards reset
    // just below would let the old socket's onclose re-arm it). Mirrors the web fix.
    if (reconnectTimerRef.current) {
      clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = null;
    }
    leftVoluntarilyRef.current = false;
    reconnectAttemptRef.current = 0;
    if (wsRef.current) { wsRef.current.onclose = null; wsRef.current.close(); wsRef.current = null; }
    // A guest authenticates the room ws with its media-scoped join-code token;
    // a logged-in member uses the account JWT.
    const token = optsRef.current.guestToken ?? useAuthStore.getState().accessToken;
    const wsBase = serverUrl.replace(/^http/, "ws");
    const url = `${wsBase}/api/v1/syncplay/rooms/${roomId}/ws`;
    const ws = new WebSocket(url, [`Bearer.${token ?? ""}`]);
    wsRef.current = ws;

    ws.onopen = () => {
      reconnectAttemptRef.current = 0;
      setState((prev) => ({ ...prev, connected: true }));
      for (const queued of messageQueueRef.current) {
        ws.send(queued);
      }
      messageQueueRef.current = [];
    };

    ws.onclose = () => {
      setState((prev) => ({ ...prev, connected: false }));
      if (!leftVoluntarilyRef.current && reconnectAttemptRef.current < 5) {
        const delay = Math.min(1000 * Math.pow(2, reconnectAttemptRef.current), 30000);
        reconnectAttemptRef.current += 1;
        reconnectTimerRef.current = setTimeout(() => connect(roomId), delay);
      }
    };

    ws.onmessage = (event) => {
      let msg: SyncMessage;
      try { msg = JSON.parse(event.data as string) as SyncMessage; } catch { return; }

      switch (msg.type) {
        case "play":
          optsRef.current.onPlay?.(msg.positionMs ?? 0);
          break;
        case "pause":
          optsRef.current.onPause?.(msg.positionMs ?? 0);
          break;
        case "seek":
          optsRef.current.onSeek?.(msg.positionMs ?? 0);
          break;
        case "state":
          if (msg.playing) optsRef.current.onPlay?.(msg.positionMs ?? 0);
          else optsRef.current.onPause?.(msg.positionMs ?? 0);
          break;
        case "next":
        case "prev":
        case "setIndex":
          if (msg.trackIndex !== undefined && msg.mediaId) {
            setState((prev) => ({ ...prev, trackIndex: msg.trackIndex! }));
            optsRef.current.onTrackChange?.(msg.trackIndex, msg.mediaId);
          }
          break;
        case "member_joined":
          if (msg.userId && msg.username) {
            setState((prev) => ({
              ...prev,
              members: [...prev.members.filter((m) => m.userId !== msg.userId), { userId: msg.userId!, username: msg.username! }],
            }));
            optsRef.current.onMemberJoined?.(msg.userId, msg.username);
          }
          break;
        case "member_left":
          if (msg.userId) {
            setState((prev) => ({ ...prev, members: prev.members.filter((m) => m.userId !== msg.userId) }));
            optsRef.current.onMemberLeft?.(msg.userId);
          }
          break;
      }
    };
  }, [serverUrl]);

  useEffect(() => {
    return () => {
      leftVoluntarilyRef.current = true;
      if (reconnectTimerRef.current) {
        clearTimeout(reconnectTimerRef.current);
        reconnectTimerRef.current = null;
      }
      wsRef.current?.close();
      wsRef.current = null;
    };
  }, []);

  const send = useCallback((payload: Record<string, unknown>) => {
    const str = JSON.stringify(payload);
    const ws = wsRef.current;
    if (ws?.readyState === WebSocket.OPEN) {
      ws.send(str);
    } else if (ws?.readyState === WebSocket.CONNECTING) {
      messageQueueRef.current.push(str);
    }
  }, []);

  const createRoom = useCallback(async (mediaId: string, mediaType?: string): Promise<string> => {
    const data = await eveyFetch<{ id: string; code: string }>({
      url: "/api/v1/syncplay/rooms",
      method: "POST",
      data: { mediaId, name: mediaType === "music" ? "Listen Together" : "Watch Together", mediaType },
    });
    setState({ roomId: data.id, roomCode: data.code, members: [], connected: false, isHost: true, queue: [], trackIndex: 0 });
    connect(data.id);
    return data.id;
  }, [connect]);

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
  }, [connect]);

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
  }, [connect]);

  // Connect to a room the caller already resolved (a guest gets roomId from the
  // join-code response, so it skips the JWT-gated joinByCode POST).
  const joinRoom = useCallback((roomId: string) => {
    setState((prev) => ({ ...prev, roomId, isHost: false }));
    connect(roomId);
  }, [connect]);

  const leaveRoom = useCallback(async () => {
    leftVoluntarilyRef.current = true;
    if (reconnectTimerRef.current) {
      clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = null;
    }
    if (wsRef.current) { wsRef.current.close(); wsRef.current = null; }
    const roomId = state.roomId;
    setState({ roomId: null, roomCode: null, members: [], connected: false, isHost: false, queue: [], trackIndex: 0 });
    if (roomId) {
      try {
        await eveyFetch<void>({ url: `/api/v1/syncplay/rooms/${roomId}/leave`, method: "DELETE" });
      } catch (err) {
        // Best-effort: the server expires the membership anyway. Logged so a
        // "ghost member" report can be traced to this failing call.
        console.warn("[SyncPlay] leave room failed (membership will expire server-side):", err);
      }
    }
  }, [state.roomId]);

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
  };
}
