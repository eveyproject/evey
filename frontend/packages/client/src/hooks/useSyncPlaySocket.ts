import { useCallback, useEffect, useRef, useState, type Dispatch, type SetStateAction } from "react";
import { useAuthStore } from "@evey/store";
import { reconcilePosition } from "../lib/syncplayReconcile";

// The SyncPlay room websocket of every client. The web, the phone and the two
// televisions each kept their own until 2026-09-24, and they had drifted: only
// the web kept its reconnect backoff (the others reset the attempt counter on
// every retry, so a down server was retried every second forever), only the
// web obeyed "resync" and "host_changed", and the televisions dropped what was
// sent while the socket was still opening. REST calls stay with each app.

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

export const EMPTY_SYNCPLAY_STATE: SyncPlayState = {
  roomId: null,
  roomCode: null,
  members: [],
  connected: false,
  isHost: false,
  queue: [],
  trackIndex: 0,
};

export interface SyncMessage {
  type: string;
  positionMs?: number;
  userId?: string;
  username?: string;
  playing?: boolean;
  trackIndex?: number;
  mediaId?: string;
}

export interface SyncPlaySocketOptions {
  // The server the room lives on (the web passes its own origin).
  serverUrl: string;
  // An unauthenticated "watch together" guest authenticates the ws with its
  // media-scoped stream token instead of the account JWT.
  guestToken?: string;
  // With it, play/pause/state go through reconcilePosition and the handlers
  // learn whether to move the playhead (shouldSeek): a sub-second drift only
  // applies the play/pause transition. Without it they always seek.
  getLocalPositionMs?: () => number;
  // The player goes to positionMs, then plays or pauses.
  onPlay?: (positionMs: number, shouldSeek: boolean) => void;
  onPause?: (positionMs: number, shouldSeek: boolean) => void;
  // A seek is deliberate intent, always applied verbatim.
  onSeek?: (positionMs: number) => void;
  onMemberJoined?: (userId: string, username: string) => void;
  onMemberLeft?: (userId: string) => void;
  onTrackChange?: (trackIndex: number, mediaId: string) => void;
}

const MAX_RECONNECT_ATTEMPTS = 5;

function tokenSubject(token: string | null | undefined): string | null {
  const payload = token?.split(".")[1];
  if (!payload) return null;
  try {
    return (JSON.parse(atob(payload.replace(/-/g, "+").replace(/_/g, "/"))) as { sub?: string }).sub ?? null;
  } catch {
    return null;
  }
}

export interface SyncPlaySocket {
  state: SyncPlayState;
  setState: Dispatch<SetStateAction<SyncPlayState>>;
  connect: (roomId: string) => void;
  // Closes the socket for good (no reconnect) and resets the state; returns
  // the room that was open, for the caller's REST leave.
  disconnect: () => string | null;
  send: (msg: SyncMessage) => void;
}

export function useSyncPlaySocket(opts: SyncPlaySocketOptions): SyncPlaySocket {
  const [state, setState] = useState<SyncPlayState>(EMPTY_SYNCPLAY_STATE);
  const stateRef = useRef(state);
  stateRef.current = state;
  const optsRef = useRef(opts);
  optsRef.current = opts;
  const wsRef = useRef<WebSocket | null>(null);
  const leftVoluntarilyRef = useRef(false);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const reconnectAttemptRef = useRef(0);
  const outboxRef = useRef<string[]>([]);

  const clearReconnect = () => {
    if (reconnectTimerRef.current) {
      clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = null;
    }
  };

  const open = useCallback((roomId: string, isReconnect: boolean) => {
    // A reconnect already scheduled by a previous socket would otherwise fire
    // and silently reconnect to the OLD room after a room switch.
    clearReconnect();
    leftVoluntarilyRef.current = false;
    // Only a fresh connect resets the counter: resetting it on the retry path
    // collapsed the backoff to 1 s and defeated the give-up.
    if (!isReconnect) {
      reconnectAttemptRef.current = 0;
      outboxRef.current = [];
    }
    if (wsRef.current) {
      // Detached first: closing the old socket must not re-enter the
      // reconnect logic for the old room.
      wsRef.current.onclose = null;
      wsRef.current.close();
      wsRef.current = null;
    }
    // Read fresh at every (re)connect: the JWT rotates underneath.
    const token = optsRef.current.guestToken ?? useAuthStore.getState().accessToken;
    const wsBase = optsRef.current.serverUrl.replace(/^http/, "ws");
    const ws = new WebSocket(`${wsBase}/api/v1/syncplay/rooms/${roomId}/ws`, [`Bearer.${token ?? ""}`]);
    wsRef.current = ws;

    ws.onopen = () => {
      reconnectAttemptRef.current = 0;
      setState((prev) => ({ ...prev, connected: true }));
      for (const queued of outboxRef.current.splice(0)) ws.send(queued);
    };

    ws.onclose = () => {
      // Deliberately leaves playback alone: SyncPlay is a resync layer over a
      // player that plays fine on its own.
      setState((prev) => ({ ...prev, connected: false }));
      if (!leftVoluntarilyRef.current && reconnectAttemptRef.current < MAX_RECONNECT_ATTEMPTS) {
        const delay = Math.min(1000 * Math.pow(2, reconnectAttemptRef.current), 30000);
        reconnectAttemptRef.current += 1;
        reconnectTimerRef.current = setTimeout(() => open(roomId, true), delay);
      }
    };

    ws.onmessage = (event: MessageEvent) => {
      let msg: SyncMessage;
      try {
        msg = JSON.parse(String(event.data)) as SyncMessage;
      } catch {
        return;
      }
      const o = optsRef.current;
      const target = msg.positionMs ?? 0;
      const reconcile = () =>
        o.getLocalPositionMs
          ? reconcilePosition({ localPositionMs: o.getLocalPositionMs(), targetPositionMs: target })
          : { shouldSeek: true, positionMs: target };

      switch (msg.type) {
        case "play": {
          const r = reconcile();
          o.onPlay?.(r.positionMs, r.shouldSeek);
          break;
        }
        case "pause": {
          const r = reconcile();
          o.onPause?.(r.positionMs, r.shouldSeek);
          break;
        }
        case "seek":
          o.onSeek?.(target);
          break;
        case "state": {
          // The snapshot right after joining, and the answer to catch_up. A
          // late joiner must also START when the room plays, and stay paused
          // when it does not.
          if (msg.positionMs === undefined) break;
          const r = reconcile();
          if (msg.playing) o.onPlay?.(r.positionMs, r.shouldSeek);
          else o.onPause?.(r.positionMs, r.shouldSeek);
          break;
        }
        case "resync":
          // "Everyone sync to me": an explicit request, applied verbatim.
          if (msg.positionMs !== undefined) {
            o.onSeek?.(msg.positionMs);
            o.onPlay?.(msg.positionMs, true);
          }
          break;
        case "next":
        case "prev":
        case "setIndex":
          if (msg.trackIndex !== undefined && msg.mediaId) {
            const index = msg.trackIndex;
            setState((prev) => ({ ...prev, trackIndex: index }));
            o.onTrackChange?.(index, msg.mediaId);
          }
          break;
        case "member_joined":
          if (msg.userId && msg.username) {
            const member = { userId: msg.userId, username: msg.username };
            setState((prev) => ({ ...prev, members: [...prev.members.filter((m) => m.userId !== member.userId), member] }));
            o.onMemberJoined?.(member.userId, member.username);
          }
          break;
        case "member_left":
          if (msg.userId) {
            const gone = msg.userId;
            setState((prev) => ({ ...prev, members: prev.members.filter((m) => m.userId !== gone) }));
            o.onMemberLeft?.(gone);
          }
          break;
        case "host_changed": {
          // The server elected a new host (typically the old one left): the
          // promoted member must regain the host-only controls.
          const me = tokenSubject(useAuthStore.getState().accessToken);
          setState((prev) => ({ ...prev, isHost: !!me && msg.userId === me }));
          break;
        }
      }
    };
  }, []);

  const connect = useCallback((roomId: string) => open(roomId, false), [open]);

  const disconnect = useCallback((): string | null => {
    leftVoluntarilyRef.current = true;
    clearReconnect();
    wsRef.current?.close();
    wsRef.current = null;
    outboxRef.current = [];
    const roomId = stateRef.current.roomId;
    setState(EMPTY_SYNCPLAY_STATE);
    return roomId;
  }, []);

  const send = useCallback((msg: SyncMessage) => {
    const ws = wsRef.current;
    if (!ws) return;
    if (ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(msg));
    else if (ws.readyState === WebSocket.CONNECTING) outboxRef.current.push(JSON.stringify(msg));
  }, []);

  useEffect(() => {
    return () => {
      leftVoluntarilyRef.current = true;
      clearReconnect();
      wsRef.current?.close();
      wsRef.current = null;
    };
  }, []);

  return { state, setState, connect, disconnect, send };
}
