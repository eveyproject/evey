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
}

interface SyncMessage {
  type: string;
  positionMs?: number;
  userId?: string;
  username?: string;
  playing?: boolean;
}

interface UseSyncPlayOptions {
  onPlay?: (positionMs: number) => void;
  onPause?: (positionMs: number) => void;
  onSeek?: (positionMs: number) => void;
  onMemberJoined?: (userId: string, username: string) => void;
  onMemberLeft?: (userId: string) => void;
}

export function useSyncPlay(opts: UseSyncPlayOptions = {}) {
  const serverUrl = useServerUrl();
  const [state, setState] = useState<SyncPlayState>({ roomId: null, roomCode: null, members: [], connected: false, isHost: false });
  const wsRef = useRef<WebSocket | null>(null);
  const optsRef = useRef(opts);
  optsRef.current = opts;
  const leftVoluntarilyRef = useRef(false);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const reconnectAttemptRef = useRef(0);
  const messageQueueRef = useRef<string[]>([]);

  const connect = useCallback((roomId: string) => {
    leftVoluntarilyRef.current = false;
    reconnectAttemptRef.current = 0;
    if (wsRef.current) { wsRef.current.close(); wsRef.current = null; }
    const token = useAuthStore.getState().accessToken;
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

  const send = useCallback((type: string, positionMs: number) => {
    const payload = JSON.stringify({ type, positionMs });
    const ws = wsRef.current;
    if (ws?.readyState === WebSocket.OPEN) {
      ws.send(payload);
    } else if (ws?.readyState === WebSocket.CONNECTING) {
      messageQueueRef.current.push(payload);
    }
  }, []);

  const createRoom = useCallback(async (mediaId: string): Promise<string> => {
    const data = await eveyFetch<{ id: string; code: string }>({
      url: "/api/v1/syncplay/rooms",
      method: "POST",
      data: { mediaId, name: "SyncPlay" },
    });
    setState((prev) => ({ ...prev, roomId: data.id, roomCode: data.code, isHost: true }));
    connect(data.id);
    return data.id;
  }, [connect]);

  const joinByCode = useCallback(async (code: string): Promise<{ roomId: string; mediaId: string }> => {
    const data = await eveyFetch<{ id: string; mediaId: string; code: string }>({
      url: `/api/v1/syncplay/rooms/code/${code}`,
      method: "GET",
    });
    setState((prev) => ({ ...prev, roomId: data.id, roomCode: data.code, isHost: false }));
    connect(data.id);
    return { roomId: data.id, mediaId: data.mediaId };
  }, [connect]);

  const leaveRoom = useCallback(async () => {
    leftVoluntarilyRef.current = true;
    if (reconnectTimerRef.current) {
      clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = null;
    }
    if (wsRef.current) { wsRef.current.close(); wsRef.current = null; }
    const roomId = state.roomId;
    setState({ roomId: null, roomCode: null, members: [], connected: false, isHost: false });
    if (roomId) {
      try {
        await eveyFetch<void>({ url: `/api/v1/syncplay/rooms/${roomId}/leave`, method: "POST" });
      } catch {}
    }
  }, [state.roomId]);

  return {
    state,
    createRoom,
    joinByCode,
    leaveRoom,
    sendPlay: (ms: number) => send("play", ms),
    sendPause: (ms: number) => send("pause", ms),
    sendSeek: (ms: number) => send("seek", ms),
  };
}
