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
  createRoomFromQueue: (trackIds: string[], name?: string) => Promise<string>;
  joinRoom: (roomId: string) => Promise<void>;
  joinByCode: (code: string) => Promise<{ roomId: string; mediaId: string; mediaType: string }>;
  leaveRoom: () => Promise<void>;
  sendPlay: (positionMs: number) => void;
  sendPause: (positionMs: number) => void;
  sendSeek: (positionMs: number) => void;
  sendNext: () => void;
  sendPrev: () => void;
  sendSetIndex: (index: number) => void;
  /** v2 "everyone sync to me": any member pulls the party to their position. */
  sendResync: (positionMs: number) => void;
  // "Put ME back with the room". The mirror image of sendResync, which moves
  // everyone: this one moves only the caller, so someone who drifted can rejoin
  // without anyone else's picture jumping. The server answers with a plain
  // state message, handled by the same path as a mid-session join.
  sendCatchUp: () => void;
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
  /** Unauthenticated guest: media-scoped stream token used for the ws instead of the JWT. */
  guestToken?: string;
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

  const connect = useCallback((roomId: string, isReconnect = false) => {
    // Cancel any reconnect already scheduled by a previous connection's onclose.
    // Without this, switching rooms (joinRoom/createRoom after an earlier room
    // dropped) lets a stale setTimeout(() => connect(oldRoomId)) fire and
    // silently reconnect to the wrong room.
    if (reconnectTimerRef.current) {
      clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = null;
    }
    leftVoluntarilyRef.current = false;
    // Only a fresh, user-initiated connect resets the attempt counter. The
    // reconnect timer below re-enters connect() too; resetting here on that path
    // would collapse the exponential backoff to a constant 1s and defeat the
    // 5-attempt give-up, producing an endless reconnect storm on a down server.
    if (!isReconnect) reconnectAttemptRef.current = 0;
    messageQueueRef.current = [];
    if (wsRef.current) {
      // Detach onclose before the deliberate close: otherwise closing the old
      // socket here re-enters the reconnect logic (the guards were just reset
      // above) and schedules a fresh reconnect to the OLD room.
      wsRef.current.onclose = null;
      wsRef.current.close();
      wsRef.current = null;
    }
    const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    const url = `${proto}//${window.location.host}/api/v1/syncplay/rooms/${roomId}/ws`;
    // Read a fresh token on each connect/reconnect. An unauthenticated
    // "watch together" guest has no JWT in the store - it passes its media-scoped
    // guest stream token instead, which the ws upgrade accepts for this room.
    const freshToken = optsRef.current.guestToken ?? useAuthStore.getState().accessToken;
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
        reconnectTimerRef.current = setTimeout(() => connect(roomId, true), delay);
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
        case "host_changed":
          // The server elected a new host (typically the old host left). Recompute
          // our own host status so the promoted member regains the host-only
          // controls; without this the whole party would lose playback control.
          setState((prev) => ({ ...prev, isHost: msg.userId === myUserIdRef.current }));
          break;
        case "resync":
          // v2 "everyone sync to me": jump to the sender's position and resume.
          if (msg.positionMs !== undefined) {
            optsRef.current.onSeek?.(msg.positionMs);
            optsRef.current.onPlay?.(msg.positionMs);
          }
          break;
        case "state":
          if (msg.positionMs !== undefined) {
            optsRef.current.onSeek?.(msg.positionMs);
            // A late joiner must also START playing when the room is already
            // playing, otherwise it seeks to the live position but stays paused
            // and desynced until the next command.
            if (msg.playing) optsRef.current.onPlay?.(msg.positionMs);
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

  const createRoomFromQueue = useCallback(async (trackIds: string[], name?: string): Promise<string> => {
    const data = await syncPlayServiceCreateRoom({ queue: trackIds, name, mediaType: "music" });
    const id = data.id ?? "";
    // Fetch the room to populate queue/trackIndex (same as the playlist path).
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
    // A guest has no JWT - the REST join/get-room RPCs would 401. The ws-guest
    // upgrade path adds the guest as a member on connect, so a guest just opens
    // the socket directly; member/state come from the ws broadcasts.
    if (optsRef.current.guestToken) {
      setState({ roomId, roomCode: null, members: [], connected: false, isHost: false, queue: [], trackIndex: 0 });
      // Defer to a microtask so the connect lands AFTER React StrictMode's
      // mount→unmount→remount churn (the unmount cleanup closes any ws opened in
      // the first pass); the authenticated path is deferred naturally by its
      // awaited REST calls.
      await Promise.resolve();
      connect(roomId);
      return;
    }
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

  const sendResync = useCallback((positionMs: number) => {
    send({ type: "resync", positionMs });
  }, [send]);

  const sendCatchUp = useCallback(() => {
    send({ type: "catch_up" });
  }, [send]);

  useEffect(() => {
    if (!accessToken) return;
    try {
      const tokenParts = accessToken.split(".");
      if (tokenParts.length < 3 || !tokenParts[1]) return;
      const payload = JSON.parse(atob(tokenParts[1])) as { sub?: string };
      myUserIdRef.current = payload.sub ?? null;
    } catch {
      // ignore
    }
  }, [accessToken]);

  return [state, { createRoom, createRoomFromPlaylist, createRoomFromQueue, joinRoom, joinByCode, leaveRoom, sendPlay, sendPause, sendSeek, sendNext, sendPrev, sendSetIndex, sendResync, sendCatchUp }];
}
