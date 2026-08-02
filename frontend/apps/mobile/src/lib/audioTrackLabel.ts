// The audio track menu entry, worded the same way the web player words it.
//
// `channels` was already carried into the mobile panel's stream type and then
// never rendered - a dead field. Web shows "VF - 5.1", mobile showed "VF", so
// two tracks that differ only by layout were indistinguishable in the menu.
//
// ⚠️ This logic exists in three places (web AudioTrackSelector, TV
// AudioTrackPicker, here) because the monorepo has no shared pure-utility
// package yet. Consolidating means adding one, which is a workspace change -
// worth doing, not worth smuggling into a display fix.

const CHANNEL_LAYOUTS: Record<number, string> = {
  1: "Mono",
  2: "Stereo",
  6: "5.1",
  8: "7.1",
};

export function formatChannelLayout(channels: number | undefined): string {
  if (!channels || channels <= 0) return "";
  return CHANNEL_LAYOUTS[channels] ?? `${channels}ch`;
}

/**
 * `fallback` is what to show when the stream has neither title nor language;
 * the caller localises it.
 */
export function formatAudioTrackLabel(
  s: { title?: string; language?: string; channels?: number },
  fallback: string,
): string {
  const name = s.title ?? s.language ?? fallback;
  const layout = formatChannelLayout(s.channels);
  return layout ? `${name} - ${layout}` : name;
}
