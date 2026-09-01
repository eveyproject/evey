// Sender-side observation of a receiver that stopped answering. Live
// 2026-08-31 (a film cast to a Chromecast HD): after a re-load the television
// went dark, the receiver's beacons stopped, and the phone showed a spinner
// for as long as the user cared to wait - with nothing in the server logs
// saying what the SENDER saw (was the session still CONNECTED? was the player
// state PLAYING with a frozen position, IDLE with an idleReason, or no media
// status at all?). This module decides, from the media status the Cast SDK
// hands the sender, when that is worth ONE beacon so the next incident is
// diagnosable. It decides nothing about playback: observability first, a
// recovery gesture once the numbers say which state to recover from.

/** MediaPlayerState of the Cast SDK (react-native-google-cast). */
export const PLAYER_STATE_IDLE = 1;
export const PLAYER_STATE_PLAYING = 2;

/** A PLAYING position that has not moved for this long is a stall. */
export const CAST_STALL_SEC = 20;

export interface CastStallSample {
  connected: boolean;
  /** MediaPlayerState, undefined when the SDK delivered no media status. */
  playerState?: number;
  /** MediaPlayerIdleReason when playerState is IDLE (string or enum int by SDK version). */
  idleReason?: string | number | null;
  posSec: number;
  nowMs: number;
}

export interface CastStallTracker {
  lastPos: number;
  lastMoveMs: number;
  /** The beacon already sent for the current situation, null when none. */
  reported: string | null;
}

export const initialCastStallTracker: CastStallTracker = { lastPos: -1, lastMoveMs: 0, reported: null };

/**
 * Feeds one sample; returns the next tracker and, at most once per
 * situation, the `details` line of a `cast_sender_stall` beacon.
 */
export function observeCastStall(prev: CastStallTracker, s: CastStallSample): { next: CastStallTracker; report: string | null } {
  if (!s.connected) return { next: initialCastStallTracker, report: null };

  if (s.playerState === PLAYER_STATE_IDLE && s.idleReason !== undefined && s.idleReason !== null && s.idleReason !== "") {
    const key = `idle:${String(s.idleReason)}`;
    if (prev.reported === key) return { next: prev, report: null };
    return {
      next: { ...prev, reported: key },
      report: `state=idle reason=${String(s.idleReason)} pos=${Math.round(s.posSec)}`,
    };
  }

  if (s.playerState !== PLAYER_STATE_PLAYING) {
    // Paused, buffering, loading, or no status yet: the position is not
    // expected to move, so the frozen clock restarts from here.
    return { next: { lastPos: s.posSec, lastMoveMs: s.nowMs, reported: prev.reported?.startsWith("idle:") ? null : prev.reported }, report: null };
  }

  if (s.posSec !== prev.lastPos) {
    return { next: { lastPos: s.posSec, lastMoveMs: s.nowMs, reported: null }, report: null };
  }
  const frozenSec = prev.lastMoveMs > 0 ? (s.nowMs - prev.lastMoveMs) / 1000 : 0;
  if (frozenSec >= CAST_STALL_SEC && prev.reported !== "stall") {
    return {
      next: { ...prev, reported: "stall" },
      report: `state=playing frozen=${Math.round(frozenSec)}s pos=${Math.round(s.posSec)}`,
    };
  }
  return { next: prev, report: null };
}
