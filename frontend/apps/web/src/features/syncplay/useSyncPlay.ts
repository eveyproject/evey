import { useCallback, useEffect, useRef, useState } from "react";
import { useAuthStore } from "@evey/store";
import {
  syncPlayServiceCreateRoom,
  syncPlayServiceJoinRoom,
  syncPlayServiceGetRoom,
  syncPlayServiceLeaveRoom,
  syncPlayServiceJoinByCode,
} from "@evey/api";

export interface SyncPlayMember {
  userId: string;
  username: string;
}

interface SyncPlayState {
  roomId: string | null;
  roomCode: string | null;
  members: SyncPlayMember[];
  connected: boolean;
  isHost: boolean;
  queue: string[];
  trackIndex: number;
}

interface SyncPlayControls {
  createRoom: (mediaId: string, mediaType?: string) => Promise<string>;
  createRoomFromPlaylist: (playlistId: string, name?: string) => Promise<string>;
  joinRoom: (roomId: string) => Promise<void>;
  joinByCode: (code: string) => Promise<{ roomId: string; mediaId: string; mediaType: string }>;
  leaveRoom: () => Promise<void>;
  sendPlay: (positionMs: number) => void;
  sendPause: (positionMs: number) => void;
  sendSeek: (positionMs: number) => void;
  sendNext: () => void;
  sendPrev: () => void;
  sendSetIndex: (index: number) => void;
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
}

export function useSyncPlay(opts: UseSyncPlayOptions = {}): [SyncPlayState, SyncPlayControls] {
  const accessToken = useAuthStore((s) => s.accessToken);
  const [state, setState] = useState<SyncPlayState>({ roomId: null, roomCode: null, members: [], connected: false, isHost: false, queue: [], trackIndex: 0 });
  const wsRef = useRef<WebSocket | null>(null);
  const myUserIdRef = useRef<string | null>(null);
  const optsRef = useRef(opts);
  optsRef.current = opts;
  const leftVoluntarilyRef = useRef(false);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const reconnectAttemptRef = useRef(0);
  const messageQueueRef = useRef<SyncMessage[]>([]);

  const connect = useCallback((roomId: string) => {
    leftVoluntarilyRef.current = false;
    reconnectAttemptRef.current = 0;
    messageQueueRef.current = [];
    if (wsRef.current) {
      wsRef.current.close();
      wsRef.current = null;
    }
    const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    const url = `${proto}//${window.location.host}/api/v1/syncplay/rooms/${roomId}/ws`;
    // Read fresh token from store on each connect/reconnect
    const freshToken = useAuthStore.getState().accessToken;
    const ws = new WebSocket(url, [`Bearer.${freshToken ?? ""}`]);
    wsRef.current = ws;

    ws.onopen = () => {
      reconnectAttemptRef.current = 0;
      setState((prev) => ({ ...prev, connected: true }));
      // Flush queued messages
      const queued = messageQueueRef.current.splice(0);
      for (const msg of queued) {
        ws.send(JSON.stringify(msg));
      }
    };

    ws.onclose = () => {
      setState((prev) => ({ ...prev, connected: false }));
      if (!leftVoluntarilyRef.current && reconnectAttemptRef.current < 5) {
        const delay = Math.min(1000 * Math.pow(2, reconnectAttemptRef.current), 30000);
        reconnectAttemptRef.current += 1;
        reconnectTimerRef.current = setTimeout(() => connect(roomId), delay);
      }
    };

    ws.onmessage = (event: MessageEvent<string>) => {
      let msg: SyncMessage;
      try {
        msg = JSON.parse(event.data) as SyncMessage;
      } catch {
        return;
      }

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
        case "state":
          if (msg.positionMs !== undefined) {
            optsRef.current.onSeek?.(msg.positionMs);
          }
          break;
      }
    };
  // eslint-disable-next-line react-hooks/exhaustive-deps -- token read fresh from store inside, not from closure
  }, []);

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

  const createRoom = useCallback(async (mediaId: string, mediaType?: string): Promise<string> => {
    const data = await syncPlayServiceCreateRoom({ mediaId, name: mediaType === "music" ? "Listen Together" : "Watch Together", mediaType });
    const id = data.id ?? "";
    setState({ roomId: id, roomCode: data.code ?? null, members: [], connected: false, isHost: true, queue: [], trackIndex: 0 });
    connect(id);
    return id;
  }, [connect]);

  const createRoomFromPlaylist = useCallback(async (playlistId: string, name?: string): Promise<string> => {
    const data = await syncPlayServiceCreateRoom({ playlistId, name });
    const id = data.id ?? "";
    // Fetch the room to populate queue/trackIndex
    const info = await syncPlayServiceGetRoom(id);
    setState({
      roomId: id,
      roomCode: data.code ?? null,
      members: [],
      connected: false,
      isHost: true,
      queue: info.queue ?? [],
      trackIndex: info.trackIndex ?? 0,
    });
    connect(id);
    return id;
  }, [connect]);

  const joinRoom = useCallback(async (roomId: string): Promise<void> => {
    await syncPlayServiceJoinRoom(roomId, {});
    const info = await syncPlayServiceGetRoom(roomId);
    setState({
      roomId,
      roomCode: info.code ?? null,
      members: (info.members ?? []).map((m) => ({ userId: m.userId ?? "", username: m.username ?? "" })),
      connected: false,
      isHost: false,
      queue: info.queue ?? [],
      trackIndex: info.trackIndex ?? 0,
    });
    connect(roomId);
  }, [connect]);

  const joinByCode = useCallback(async (code: string): Promise<{ roomId: string; mediaId: string; mediaType: string }> => {
    const data = await syncPlayServiceJoinByCode({ code });
    const roomId = data.roomId ?? "";
    const mediaId = data.mediaId ?? "";
    const mediaType = data.mediaType ?? "video";
    // Pull room info to grab queue if any
    let queue: string[] = [];
    let trackIndex = 0;
    try {
      const info = await syncPlayServiceGetRoom(roomId);
      queue = info.queue ?? [];
      trackIndex = info.trackIndex ?? 0;
    } catch {
      // ignore
    }
    setState({ roomId, roomCode: null, members: [], connected: false, isHost: false, queue, trackIndex });
    connect(roomId);
    return { roomId, mediaId, mediaType };
  }, [connect]);

  const leaveRoom = useCallback(async (): Promise<void> => {
    leftVoluntarilyRef.current = true;
    if (reconnectTimerRef.current) {
      clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = null;
    }
    const roomId = state.roomId;
    wsRef.current?.close();
    wsRef.current = null;
    setState({ roomId: null, roomCode: null, members: [], connected: false, isHost: false, queue: [], trackIndex: 0 });
    if (roomId) {
      await syncPlayServiceLeaveRoom(roomId).catch((err) => console.warn("syncplay leave failed", err));
    }
  }, [state.roomId]);

  const send = useCallback((msg: SyncMessage) => {
    const ws = wsRef.current;
    if (!ws || ws.readyState !== WebSocket.OPEN) {
      if (ws && ws.readyState === WebSocket.CONNECTING) {
        messageQueueRef.current.push(msg);
      }
      return;
    }
    ws.send(JSON.stringify(msg));
  }, []);

  const sendPlay = useCallback((positionMs: number) => {
    send({ type: "play", positionMs });
  }, [send]);

  const sendPause = useCallback((positionMs: number) => {
    send({ type: "pause", positionMs });
  }, [send]);

  const sendSeek = useCallback((positionMs: number) => {
    send({ type: "seek", positionMs });
  }, [send]);

  const sendNext = useCallback(() => {
    send({ type: "next" });
  }, [send]);

  const sendPrev = useCallback(() => {
    send({ type: "prev" });
  }, [send]);

  const sendSetIndex = useCallback((index: number) => {
    send({ type: "setIndex", trackIndex: index });
  }, [send]);

  useEffect(() => {
    if (!accessToken) return;
    try {
      const payload = JSON.parse(atob(accessToken.split(".")[1])) as { sub?: string };
      myUserIdRef.current = payload.sub ?? null;
    } catch {
      // ignore
    }
  }, [accessToken]);

  return [state, { createRoom, createRoomFromPlaylist, joinRoom, joinByCode, leaveRoom, sendPlay, sendPause, sendSeek, sendNext, sendPrev, sendSetIndex }];
}
