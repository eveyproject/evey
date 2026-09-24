import { useCallback } from "react";
import {
  syncPlayServiceCreateRoom,
  syncPlayServiceJoinRoom,
  syncPlayServiceGetRoom,
  syncPlayServiceLeaveRoom,
  syncPlayServiceJoinByCode,
} from "@evey/api";
import { useSyncPlaySocket, type SyncPlayState } from "@evey/client";

// The room websocket is @evey/client's (useSyncPlaySocket); the REST calls
// that create, join and leave a room are this app's.
export type { SyncPlayMember } from "@evey/client";

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
  const { state, setState, connect, disconnect, send } = useSyncPlaySocket({
    ...opts,
    serverUrl: window.location.origin,
  });

  const createRoom = useCallback(async (mediaId: string, mediaType?: string): Promise<string> => {
    const data = await syncPlayServiceCreateRoom({ mediaId, name: mediaType === "music" ? "Listen Together" : "Watch Together", mediaType });
    const id = data.id ?? "";
    setState({ roomId: id, roomCode: data.code ?? null, members: [], connected: false, isHost: true, queue: [], trackIndex: 0 });
    connect(id);
    return id;
  }, [connect, setState]);

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
  }, [connect, setState]);

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
  }, [connect, setState]);

  const joinRoom = useCallback(async (roomId: string): Promise<void> => {
    // A guest has no JWT - the REST join/get-room RPCs would 401. The ws-guest
    // upgrade path adds the guest as a member on connect, so a guest just opens
    // the socket directly; member/state come from the ws broadcasts.
    if (opts.guestToken) {
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
  }, [connect, setState, opts.guestToken]);

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
  }, [connect, setState]);

  const leaveRoom = useCallback(async (): Promise<void> => {
    const roomId = disconnect();
    if (roomId) {
      await syncPlayServiceLeaveRoom(roomId).catch((err) => console.warn("syncplay leave failed", err));
    }
  }, [disconnect]);

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

  return [state, { createRoom, createRoomFromPlaylist, createRoomFromQueue, joinRoom, joinByCode, leaveRoom, sendPlay, sendPause, sendSeek, sendNext, sendPrev, sendSetIndex, sendResync, sendCatchUp }];
}
