// Speaker-layout notation for a channel count, and the audio-track menu entry
// built on it.
//
// This lived in SIX places (web AudioTrackSelector + StreamInfoPanel, mobile
// audioTrackLabel + StreamInfoPanel, TV AudioTrackPicker + StreamInfoPanel)
// and had already drifted - inside a single app: web's track menu said "2.0"
// while web's own stream-info panel said "Stereo" for that same track, on the
// same screen. Five of the six also hardcoded English words into a UI that
// ships in six languages.
//
// The numeric family wins: it is already the notation everyone agreed on for
// 6 and 8 channels, it reads the same in every locale, and it is what the rest
// of the product (media info, settings) uses. Unknown counts degrade to
// "<n>ch" rather than guessing a layout.
const CHANNEL_LAYOUTS: Record<number, string> = {
  1: "1.0",
  2: "2.0",
  6: "5.1",
  8: "7.1",
};

/** Empty string for an absent or nonsensical count, so callers can skip the row. */
export function formatChannelLayout(channels: number | undefined): string {
  if (!channels || channels <= 0) return "";
  return CHANNEL_LAYOUTS[channels] ?? `${channels}ch`;
}

/**
 * Label for one entry of the audio track menu.
 *
 * `fallback` is what to show when the stream carries neither a title nor a
 * language; the caller localises it, because the i18n `t` signature differs
 * between the web and React Native apps.
 */
export function formatAudioTrackLabel(
  s: { title?: string; language?: string; channels?: number },
  fallback: string,
): string {
  const name = s.title ?? s.language ?? fallback;
  const layout = formatChannelLayout(s.channels);
  return layout ? `${name} - ${layout}` : name;
}
