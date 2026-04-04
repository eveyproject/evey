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
  const accessToken = useAuthStore((s) => s.accessToken);
  const serverUrl = useServerUrl();
  const [state, setState] = useState<SyncPlayState>({ roomId: null, roomCode: null, members: [], connected: false, isHost: false });
  const wsRef = useRef<WebSocket | null>(null);
  const optsRef = useRef(opts);
  optsRef.current = opts;

  const connect = useCallback((roomId: string) => {
    if (wsRef.current) { wsRef.current.close(); wsRef.current = null; }
    const wsBase = serverUrl.replace(/^http/, "ws");
    const url = `${wsBase}/api/v1/syncplay/rooms/${roomId}/ws`;
    const ws = new WebSocket(url, [`Bearer.${accessToken ?? ""}`]);
    wsRef.current = ws;

    ws.onopen = () => setState((prev) => ({ ...prev, connected: true }));
    ws.onclose = () => setState((prev) => ({ ...prev, connected: false }));

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
  }, [serverUrl, accessToken]);

  useEffect(() => () => { wsRef.current?.close(); }, []);

  const send = useCallback((type: string, positionMs: number) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify({ type, positionMs }));
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
    if (wsRef.current) { wsRef.current.close(); wsRef.current = null; }
    if (state.roomId) {
      try {
        await eveyFetch<void>({ url: `/api/v1/syncplay/rooms/${state.roomId}/leave`, method: "POST" });
      } catch {}
    }
    setState({ roomId: null, roomCode: null, members: [], connected: false, isHost: false });
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
