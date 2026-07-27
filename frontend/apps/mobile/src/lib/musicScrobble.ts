// Pure decision rule for when a finished-track playback event should be
// recorded as a "played" scrobble - the signal the web GlobalAudioPlayer
// already posts to /api/v1/music/tracks/:id/played, which feeds the playlist
// generator's taste signal. Extracted out of MusicMiniPlayer so the guard
// against duplicate "finished" ticks (expo-audio's didJustFinish can stay
// true across more than one status update for a single playthrough) is
// unit-tested without spinning up expo-audio/expo-router.
export interface ScrobbleCheck {
  didJustFinish: boolean;
  trackId: string | null;
  lastRecordedTrackId: string | null;
}

export function shouldRecordScrobble(check: ScrobbleCheck): boolean {
  if (!check.didJustFinish || !check.trackId) return false;
  return check.lastRecordedTrackId !== check.trackId;
}
