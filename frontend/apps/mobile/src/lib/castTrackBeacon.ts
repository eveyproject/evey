// What the RECEIVER says it has, in raw form, so the track selector can be
// designed on evidence.
//
// Casting an HLS stream, the phone never learns which audio and subtitle tracks
// the receiver actually exposes: that depends on the receiver app (Default
// Media Receiver vs our custom one), on whether it parses the manifest's
// alternate renditions at all, and on the device. Building a track picker on a
// guess is how the receiver-capability table got its wrong entries before
// (2026-07-22: a real TV reported model=unknown and was silently floored).
//
// So this is step 1 of the same two-step the AC-3 work used: ship a beacon,
// read it in the admin logs from a real cast, THEN write the selector against
// what came back. It rides the existing player-event route - no server change.

export interface CastTrackLike {
  id: number;
  type: string;
  subtype?: string;
  language?: string;
  name?: string;
}

// describeCastTracks renders the receiver's track list into one log-safe line.
// Deliberately raw: names and languages are reported as the receiver spelled
// them, because the point is to discover what it actually sends. Returns "" when
// there is nothing to report, which the caller uses to skip the beacon entirely.
export function describeCastTracks(
  tracks: CastTrackLike[] | undefined,
  activeTrackIds: number[] | undefined,
): string {
  if (!tracks || tracks.length === 0) return "";
  const active = new Set(activeTrackIds ?? []);
  const parts = tracks.map((t) => {
    const kind = t.subtype ? `${t.type}/${t.subtype}` : t.type;
    const label = [t.language, t.name].filter(Boolean).join(" ");
    return `${t.id}:${kind}${label ? `:${label}` : ""}${active.has(t.id) ? ":ACTIVE" : ""}`;
  });
  return `count=${tracks.length} ${parts.join(" | ")}`;
}

// shouldReportTracks answers "is this worth one beacon?" - the status stream
// fires several times a second during playback, and the track list is a
// property of the loaded media, not of the moment. Report once per (media,
// track shape): a shape change is real news (the receiver finished parsing the
// manifest and gained renditions), the same shape again is noise.
export function shouldReportTracks(lastReported: string | null, current: string): boolean {
  return current !== "" && current !== lastReported;
}
