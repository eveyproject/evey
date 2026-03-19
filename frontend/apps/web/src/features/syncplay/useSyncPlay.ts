import { useCallback, useEffect, useRef, useState } from "react";
import { useAuthStore } from "@evey/store";
import { eveyFetch } from "@evey/api";

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

export interface SyncPlayControls {
  createRoom: (mediaId: string) => Promise<string>;
  joinRoom: (roomId: string) => Promise<void>;
  joinByCode: (code: string) => Promise<{ roomId: string; mediaId: string }>;
  leaveRoom: () => Promise<void>;
  sendPlay: (positionMs: number) => void;
  sendPause: (positionMs: number) => void;
  sendSeek: (positionMs: number) => void;
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

export function useSyncPlay(opts: UseSyncPlayOptions = {}): [SyncPlayState, SyncPlayControls] {
  const accessToken = useAuthStore((s) => s.accessToken);
  const [state, setState] = useState<SyncPlayState>({ roomId: null, roomCode: null, members: [], connected: false, isHost: false });
  const wsRef = useRef<WebSocket | null>(null);
  const myUserIdRef = useRef<string | null>(null);
  const optsRef = useRef(opts);
  optsRef.current = opts;

  const connect = useCallback((roomId: string) => {
    if (wsRef.current) {
      wsRef.current.close();
      wsRef.current = null;
    }
    const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    const url = `${proto}//${window.location.host}/api/v1/syncplay/rooms/${roomId}/ws`;
    const ws = new WebSocket(url, [`Bearer.${accessToken ?? ""}`]);
    wsRef.current = ws;

    ws.onopen = () => {
      setState((prev) => ({ ...prev, connected: true }));
    };

    ws.onclose = () => {
      setState((prev) => ({ ...prev, connected: false }));
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
  }, [accessToken]);

  useEffect(() => {
    return () => {
      wsRef.current?.close();
      wsRef.current = null;
    };
  }, []);

  const createRoom = useCallback(async (mediaId: string): Promise<string> => {
    const data = await eveyFetch<{ id: string; code: string }>({
      url: "/api/v1/syncplay/rooms",
      method: "POST",
      headers: { "Content-Type": "application/json" },
      data: { mediaId, name: "Watch Together" },
    });
    setState({ roomId: data.id, roomCode: data.code, members: [], connected: false, isHost: true });
    connect(data.id);
    return data.id;
  }, [connect]);

  const joinRoom = useCallback(async (roomId: string): Promise<void> => {
    await eveyFetch<void>({
      url: `/api/v1/syncplay/rooms/${roomId}/join`,
      method: "POST",
    });
    const info = await eveyFetch<{ members?: SyncPlayMember[]; code?: string }>({
      url: `/api/v1/syncplay/rooms/${roomId}`,
      method: "GET",
    });
    setState({ roomId, roomCode: info.code ?? null, members: info.members ?? [], connected: false, isHost: false });
    connect(roomId);
  }, [connect]);

  const joinByCode = useCallback(async (code: string): Promise<{ roomId: string; mediaId: string }> => {
    const data = await eveyFetch<{ roomId: string; mediaId: string; wsUrl: string }>({
      url: "/api/v1/syncplay/join",
      method: "POST",
      headers: { "Content-Type": "application/json" },
      data: { code },
    });
    setState({ roomId: data.roomId, roomCode: null, members: [], connected: false, isHost: false });
    connect(data.roomId);
    return { roomId: data.roomId, mediaId: data.mediaId };
  }, [connect]);

  const leaveRoom = useCallback(async (): Promise<void> => {
    const roomId = state.roomId;
    wsRef.current?.close();
    wsRef.current = null;
    setState({ roomId: null, roomCode: null, members: [], connected: false, isHost: false });
    if (roomId) {
      await eveyFetch<void>({
        url: `/api/v1/syncplay/rooms/${roomId}/leave`,
        method: "DELETE",
      }).catch(() => {});
    }
  }, [state.roomId]);

  const send = useCallback((msg: SyncMessage) => {
    const ws = wsRef.current;
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
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

  useEffect(() => {
    if (!accessToken) return;
    try {
      const payload = JSON.parse(atob(accessToken.split(".")[1])) as { sub?: string };
      myUserIdRef.current = payload.sub ?? null;
    } catch {
      // ignore
    }
  }, [accessToken]);

  return [state, { createRoom, joinRoom, joinByCode, leaveRoom, sendPlay, sendPause, sendSeek }];
}
