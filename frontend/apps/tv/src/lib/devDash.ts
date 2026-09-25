// Dev-only DASH playback of the TV player, behind a flag baked into the dev
// bundle: EXPO_PUBLIC_EVEY_DEV_DASH=1 when Metro starts. The session is the
// HLS master's - the manifest.mpd request carries its query - played by
// Media3 as DASH, and the player falls back to the HLS master whenever the
// server offers no DASH for it (404) or the DASH source fails.

export function devDashEnabled(): boolean {
  return process.env.EXPO_PUBLIC_EVEY_DEV_DASH === "1";
}

// The manifest.mpd of the master a stream URL names, null for any other URL
// (direct play, a packaged playlist).
export function dashUrlFor(streamUrl: string): string | null {
  return streamUrl.includes("/master.m3u8") ? streamUrl.replace("/master.m3u8", "/manifest.mpd") : null;
}
