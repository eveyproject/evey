// A track chosen on the media page BEFORE playback, carried to the player as
// route params. `audio` and `subtitle` are ABSOLUTE media-stream indices (the
// numbering MediaStreams exposes), `subtitle` may also be "off". Absent means
// the player resolves the track itself (profile preference, file default).
export interface TrackPick {
  audio?: number;
  subtitle?: number | "off";
}

const SUBTITLE_OFF = "off";

function parseIndex(raw: unknown): number | undefined {
  if (typeof raw === "number") return Number.isInteger(raw) && raw >= 0 ? raw : undefined;
  if (typeof raw === "string" && /^\d+$/.test(raw)) return Number(raw);
  return undefined;
}

/** Reads a pick back from route params (strings from a URL, numbers from a typed router). */
export function parseTrackPick(raw: { audio?: unknown; subtitle?: unknown } | undefined): TrackPick {
  if (!raw) return {};
  const pick: TrackPick = {};
  const audio = parseIndex(raw.audio);
  if (audio !== undefined) pick.audio = audio;
  if (raw.subtitle === SUBTITLE_OFF) pick.subtitle = SUBTITLE_OFF;
  else {
    const subtitle = parseIndex(raw.subtitle);
    if (subtitle !== undefined) pick.subtitle = subtitle;
  }
  return pick;
}

/** The pick as route params; empty when nothing was chosen, so a plain play URL stays untouched. */
export function trackPickParams(pick: TrackPick | undefined): Record<string, string> {
  const params: Record<string, string> = {};
  if (!pick) return params;
  if (pick.audio !== undefined) params.audio = String(pick.audio);
  if (pick.subtitle !== undefined) params.subtitle = String(pick.subtitle);
  return params;
}

/** "audio=3&subtitle=off", or "" - ready to append to a path with "?". */
export function trackPickQuery(pick: TrackPick | undefined): string {
  return new URLSearchParams(trackPickParams(pick)).toString();
}
