import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useAuthStore } from "@evey/store";
import { eveyFetch } from "@evey/api";
import { reconcilePosition } from "../lib/syncplayReconcile";

// TV is a SyncPlay JOIN-only client (see the join screen for why room
// creation is deliberately not offered here). The websocket wire protocol
// itself is unchanged: it follows the exact message shapes and reconnect
// behaviour of the mobile client (frontend/apps/mobile/src/hooks/
// useSyncPlay.ts) - same URL, same Bearer.<token> subprotocol, same
// exponential backoff - so the server side needs no TV-specific branch.

export interface SyncplayJoinResponse {
  roomId: string;
  mediaId: string;
}

// Plain REST helper, deliberately NOT part of the hook: the join screen only
// needs the room lookup (the server's JoinByCode RPC already adds the caller
// as a member - see syncplay_handler.go's h.hub.JoinRoom call - independent
// of any websocket), then it hands off to the player screen via a
// `?roomId=` navigation param. The player's OWN useSyncplay instance opens
// the actual websocket with connect() below once it mounts with that param.
// Doing it this way (REST join now, connect() later on the screen that
// actually has a live player) avoids opening a websocket on the join screen
// just to tear it down a navigation later.
export async function joinSyncplayRoomByCode(code: string): Promise<SyncplayJoinResponse> {
  return eveyFetch<SyncplayJoinResponse>({
    url: "/api/v1/syncplay/join",
    method: "POST",
    data: { code },
  });
}

export interface SyncplayMember {
  userId: string;
  username: string;
}

export interface SyncplayState {
  roomId: string | null;
  connected: boolean;
  isHost: boolean;
  members: SyncplayMember[];
}

interface SyncMessage {
  type: string;
  positionMs?: number;
  userId?: string;
  username?: string;
  playing?: boolean;
}

export interface UseSyncplayOptions {
  /** The server the room lives on: each client resolves its own. */
  serverUrl: string;
  // Reads the player's current position at the instant an incoming message
  // is handled. A callback rather than a plain value so the message handler
  // (captured once per connect()) always reads the LIVE position, never a
  // stale one from the render that created the websocket.
  getLocalPositionMs: () => number;
  // shouldSeek is decided by reconcilePosition (../lib/syncplayReconcile.ts):
  // false means only the play/pause transition should apply, the playhead
  // must not jump.
  onPlay?: (positionMs: number, shouldSeek: boolean) => void;
  onPause?: (positionMs: number, shouldSeek: boolean) => void;
  // A host's explicit seek is deliberate intent, not a resync - always
  // applied verbatim, same as web/mobile.
  onSeek?: (positionMs: number) => void;
}

export function useSyncplay(opts: UseSyncplayOptions) {
  const serverUrl = opts.serverUrl;
  const [state, setState] = useState<SyncplayState>({ roomId: null, connected: false, isHost: false, members: [] });
  const wsRef = useRef<WebSocket | null>(null);
  const optsRef = useRef(opts);
  optsRef.current = opts;
  const leftVoluntarilyRef = useRef(false);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const reconnectAttemptRef = useRef(0);

  // Opens the room websocket. Never sets isHost=true - TV never creates a
  // room, only joins one someone else already started (see the join
  // screen's header comment).
  const connect = useCallback(
    (roomId: string) => {
      // Cancel any reconnect already scheduled by a previous connection's
      // onclose, and detach the old socket's onclose before closing it -
      // otherwise a rejoin races a stale reconnect back to the OLD room
      // (mirrors the same fix in the web/mobile hooks).
      if (reconnectTimerRef.current) {
        clearTimeout(reconnectTimerRef.current);
        reconnectTimerRef.current = null;
      }
      leftVoluntarilyRef.current = false;
      reconnectAttemptRef.current = 0;
      if (wsRef.current) {
        wsRef.current.onclose = null;
        wsRef.current.close();
        wsRef.current = null;
      }
      setState({ roomId, connected: false, isHost: false, members: [] });

      const token = useAuthStore.getState().accessToken;
      const wsBase = serverUrl.replace(/^http/, "ws");
      const url = `${wsBase}/api/v1/syncplay/rooms/${roomId}/ws`;
      const ws = new WebSocket(url, [`Bearer.${token ?? ""}`]);
      wsRef.current = ws;

      ws.onopen = () => {
        reconnectAttemptRef.current = 0;
        setState((prev) => ({ ...prev, connected: true }));
      };

      ws.onclose = () => {
        // Deliberately does not touch playback: a dropped socket must leave
        // the local player running exactly as it was. SyncPlay is a resync
        // layer on top of a player that already plays fine on its own - see
        // the player screen's D-pad wiring for the full authority rule.
        setState((prev) => ({ ...prev, connected: false }));
        if (!leftVoluntarilyRef.current && reconnectAttemptRef.current < 5) {
          const delay = Math.min(1000 * Math.pow(2, reconnectAttemptRef.current), 30000);
          reconnectAttemptRef.current += 1;
          reconnectTimerRef.current = setTimeout(() => connect(roomId), delay);
        }
      };

      ws.onmessage = (event) => {
        let msg: SyncMessage;
        try {
          msg = JSON.parse(event.data as string) as SyncMessage;
        } catch {
          return;
        }
        const targetPositionMs = msg.positionMs ?? 0;
        const localPositionMs = optsRef.current.getLocalPositionMs();
        const resync = () => reconcilePosition({ localPositionMs, targetPositionMs });

        switch (msg.type) {
          case "play": {
            const { shouldSeek, positionMs } = resync();
            optsRef.current.onPlay?.(positionMs, shouldSeek);
            break;
          }
          case "pause": {
            const { shouldSeek, positionMs } = resync();
            optsRef.current.onPause?.(positionMs, shouldSeek);
            break;
          }
          case "seek":
            optsRef.current.onSeek?.(targetPositionMs);
            break;
          case "state": {
            // Initial snapshot sent right after joining. Routed through the
            // same drift check: a freshly opened player sitting near 0
            // naturally exceeds the threshold against a room already
            // mid-playback, so it resyncs - no special-casing needed.
            const { shouldSeek, positionMs } = resync();
            if (msg.playing) optsRef.current.onPlay?.(positionMs, shouldSeek);
            else optsRef.current.onPause?.(positionMs, shouldSeek);
            break;
          }
          case "member_joined":
            if (msg.userId && msg.username) {
              setState((prev) => ({
                ...prev,
                members: [...prev.members.filter((m) => m.userId !== msg.userId), { userId: msg.userId!, username: msg.username! }],
              }));
            }
            break;
          case "member_left":
            if (msg.userId) {
              setState((prev) => ({ ...prev, members: prev.members.filter((m) => m.userId !== msg.userId) }));
            }
            break;
        }
      };
    },
    [serverUrl],
  );

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
    const ws = wsRef.current;
    if (ws?.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify(payload));
    }
  }, []);

  const leaveRoom = useCallback(async () => {
    leftVoluntarilyRef.current = true;
    if (reconnectTimerRef.current) {
      clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = null;
    }
    if (wsRef.current) {
      wsRef.current.close();
      wsRef.current = null;
    }
    const roomId = state.roomId;
    setState({ roomId: null, connected: false, isHost: false, members: [] });
    if (roomId) {
      try {
        await eveyFetch<void>({ url: `/api/v1/syncplay/rooms/${roomId}/leave`, method: "DELETE" });
      } catch {
        // Best-effort: local state is already reset above, and the
        // server-side janitor sweeps a room whose last member never sent
        // this (see hub.go StartJanitor).
      }
    }
  }, [state.roomId]);

  // Sent unconditionally on local play/pause regardless of host status - the
  // server is the sole enforcer of host-only authority (hub.go HandleCommand
  // silently drops play/pause/seek from a non-host). This mirrors the
  // web/mobile clients exactly: the local player always acts immediately on
  // the user's own remote input, and the room command is just "try to also
  // move the room", accepted or ignored server-side.
  const sendPlay = useCallback((ms: number) => send({ type: "play", positionMs: ms }), [send]);
  const sendPause = useCallback((ms: number) => send({ type: "pause", positionMs: ms }), [send]);

  // Memoised: the player screen feeds this object into the deps of its own
  // useCallback for the remote's play/pause key. A fresh object (or fresh
  // sendPlay/sendPause closures) every render would give that callback a new
  // identity every render too, re-registering the TV key handler constantly -
  // and this screen has already been bitten once by a double-firing select
  // key, so handler churn is not somewhere to be casual.
  return useMemo(
    () => ({ state, connect, leaveRoom, sendPlay, sendPause }),
    [state, connect, leaveRoom, sendPlay, sendPause],
  );
}
