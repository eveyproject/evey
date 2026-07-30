// Which playback state to REPORT while a cast session owns the playback.
//
// During a cast the local player is deliberately parked: useCastSession's
// muteLocalPlayer pauses it and sets volume 0 so the phone stays silent while
// the TV plays. Reporting the local player's state therefore tells the server
// the exact opposite of the truth - "paused, muted, volume 0" for a session
// that is playing out loud on a television. That reached the admin now-playing
// panel and the presence feed as a paused, muted session.
//
// getPositionMs already had this fix (it reads the receiver's streamPosition);
// isPaused/volume/isMuted did not. The receiver's own MediaStatus carries all
// three, so the same rule applies to all four: while casting, the receiver is
// the only source of truth about playback.

export interface LocalPlaybackState {
  isPaused: boolean;
  volume: number;
  isMuted: boolean;
}

export interface CastPlaybackState {
  connected: boolean;
  isPlaying: boolean;
  // From MediaStatus: the receiver's stream volume (0.0-1.0) and mute flag.
  // Undefined until the first status arrives, which is why each falls back to
  // the neutral value rather than to the local player's parked state.
  volume: number | undefined;
  isMuted: boolean | undefined;
}

// resumePositionAfterCast decides where the LOCAL player should pick up once a
// cast session ends, in seconds, or null to leave it alone.
//
// The local player was parked at whatever position the cast started from, so
// after forty minutes on the television, disconnecting rewound the viewer to
// where they had been forty minutes earlier. The receiver's last known position
// is the one that reflects what was actually watched.
//
// Returns null when there is nothing trustworthy to seek to: no cast happened
// in this session, or the receiver never reported a position (it was still
// loading, or it failed). Seeking to 0 in those cases would invent a rewind
// rather than fix one.
export function resumePositionAfterCast(lastCastSec: number | null): number | null {
  if (lastCastSec === null || !Number.isFinite(lastCastSec) || lastCastSec <= 0) return null;
  return lastCastSec;
}

// reportedPlaybackState picks the source that is actually playing. Falls back
// to the local state whenever no cast session is connected.
export function reportedPlaybackState(
  local: LocalPlaybackState,
  cast: CastPlaybackState,
): LocalPlaybackState {
  if (!cast.connected) return local;
  return {
    isPaused: !cast.isPlaying,
    // A receiver that has not reported a status yet is assumed to be at full
    // volume and unmuted - the neutral assumption. Falling back to the local
    // player here would report volume 0 and muted, which is precisely the bug.
    volume: cast.volume ?? 1,
    isMuted: cast.isMuted ?? false,
  };
}
