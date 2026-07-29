export type AudioStream = {
  index: number;      // absolute stream index from proto
  trackIndex: number; // 0-based among audio streams (for ffmpeg -map 0:a:N)
  language?: string;
  title?: string;
  isDefault?: boolean;
  codec?: string;     // source codec (ac3/eac3/aac/...) - drives the wasm-audio path
  channels?: number;  // channel count, rendered as a 5.1/7.1 style layout
};

// Speaker-layout notation for a channel count. Deliberately not translated:
// "5.1" reads the same in every locale we ship, and it is the notation the
// rest of the product (media info, settings) already uses. Unknown counts
// degrade to "<n>ch" rather than guessing a layout.
const CHANNEL_LAYOUTS: Record<number, string> = {
  1: "1.0",
  2: "2.0",
  6: "5.1",
  8: "7.1",
};

export function formatChannelLayout(channels: number | undefined): string {
  if (!channels || channels <= 0) return "";
  return CHANNEL_LAYOUTS[channels] ?? `${channels}ch`;
}

/**
 * Label for one entry of the audio track menu. Mirrors the TV picker's shape
 * (name, then a dash, then the detail) so the three clients read alike.
 * `fallback` is what the caller wants shown when the stream carries neither a
 * title nor a language - it is already localised by the caller.
 */
export function formatAudioTrackLabel(s: AudioStream, fallback: string): string {
  const name = s.title ?? s.language ?? fallback;
  const layout = formatChannelLayout(s.channels);
  return layout ? `${name} - ${layout}` : name;
}
