import { useCallback, useEffect, useRef, useState } from "react";
import { useAuthStore } from "@evey/store";

export interface SyncPlayMember {
  userId: string;
  username: string;
}

export interface SyncPlayState {
  roomId: string | null;
  members: SyncPlayMember[];
  connected: boolean;
}

export interface SyncPlayControls {
  createRoom: (mediaId: string) => Promise<string>;
  joinRoom: (roomId: string) => Promise<void>;
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
  const [state, setState] = useState<SyncPlayState>({ roomId: null, members: [], connected: false });
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
    // Pass token as subprotocol — browsers cannot set custom headers on WebSocket connections.
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
          // Initial state on join — caller can handle via onPlay/onSeek
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
    const res = await fetch("/api/v1/syncplay/rooms", {
      method: "POST",
      headers: { "Content-Type": "application/json", Authorization: `Bearer ${accessToken ?? ""}` },
      body: JSON.stringify({ mediaId, name: "Watch Together" }),
    });
    if (!res.ok) throw new Error("Failed to create room");
    const data = (await res.json()) as { id: string };
    const roomId = data.id;
    setState({ roomId, members: [], connected: false });
    connect(roomId);
    return roomId;
  }, [accessToken, connect]);

  const joinRoom = useCallback(async (roomId: string): Promise<void> => {
    const joinRes = await fetch(`/api/v1/syncplay/rooms/${roomId}/join`, {
      method: "POST",
      headers: { Authorization: `Bearer ${accessToken ?? ""}` },
    });
    if (!joinRes.ok) throw new Error("Failed to join room");

    const infoRes = await fetch(`/api/v1/syncplay/rooms/${roomId}`, {
      headers: { Authorization: `Bearer ${accessToken ?? ""}` },
    });
    const info = infoRes.ok ? ((await infoRes.json()) as { members?: SyncPlayMember[] }) : null;
    setState({ roomId, members: info?.members ?? [], connected: false });
    connect(roomId);
  }, [accessToken, connect]);

  const leaveRoom = useCallback(async (): Promise<void> => {
    const roomId = state.roomId;
    wsRef.current?.close();
    wsRef.current = null;
    setState({ roomId: null, members: [], connected: false });
    if (roomId) {
      await fetch(`/api/v1/syncplay/rooms/${roomId}/leave`, {
        method: "DELETE",
        headers: { Authorization: `Bearer ${accessToken ?? ""}` },
      }).catch(() => {});
    }
  }, [accessToken, state.roomId]);

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

  // Store userId for echo suppression (set from token if available)
  useEffect(() => {
    if (!accessToken) return;
    try {
      const payload = JSON.parse(atob(accessToken.split(".")[1])) as { sub?: string };
      myUserIdRef.current = payload.sub ?? null;
    } catch {
      // ignore
    }
  }, [accessToken]);

  return [state, { createRoom, joinRoom, leaveRoom, sendPlay, sendPause, sendSeek }];
}
