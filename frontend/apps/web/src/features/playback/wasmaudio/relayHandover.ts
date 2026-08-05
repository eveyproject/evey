// Pure hand-over planning for the bi-source audio relay: which native (AAC)
// segments to keep and where the AC-3 tail picks up. Kept separate from
// WasmAudioEngine because it is judgement on plain data (no engine state, no
// side effects) and is exactly what needs to be readable and tested in
// isolation - the engine only applies the plan.

export interface Segment {
  url: string; // resolved, absolute
  start: number; // media start time (s)
  dur: number; // EXTINF (s)
  // native marks a segment decoded by the browser (AAC rendition via
  // decodeAudioData) instead of the wasm worker - the relay's first leg.
  native?: boolean;
  // trimSec skips the head of the segment: the relay's first AC-3 segment may
  // start before the hand-over boundary, and only the part after it may play.
  trimSec?: number;
}

// planRelayHandover computes the AAC->AC-3 hand-over as pure data: which
// native segments to keep (a prefix - live/scheduled audio is never
// un-scheduled) and the AC-3 tail, its first segment trimmed to the boundary.
// The boundary is the end of the LAST scheduled native segment, or ct plus a
// small runway when nothing is scheduled yet. Null = no hand-over possible
// (the film ends before the boundary); the native leg finishes it. Exported
// for unit testing.
export function planRelayHandover(
  nativeSegs: Segment[],
  ac3Segs: Segment[],
  scheduledIdx: Iterable<number>,
  currentTime: number,
): { segments: Segment[]; keepPrefix: number } | null {
  let boundary = currentTime + 2;
  for (const idx of scheduledIdx) {
    const s = nativeSegs[idx];
    if (s) boundary = Math.max(boundary, s.start + s.dur);
  }
  let keep = 0;
  while (keep < nativeSegs.length && nativeSegs[keep].start < boundary - 1e-3) keep++;
  if (keep > 0) {
    // Kept native segments play out in full: the boundary must sit at the last
    // one's END, or its tail would overlap the trimmed AC-3 head.
    const last = nativeSegs[keep - 1];
    boundary = Math.max(boundary, last.start + last.dur);
  }
  const k = ac3Segs.findIndex((s) => s.start + s.dur > boundary + 1e-3);
  if (k < 0) return null;
  const tail = ac3Segs.slice(k).map((s, i) =>
    i === 0 && s.start < boundary ? { ...s, trimSec: boundary - s.start } : s,
  );
  return { segments: [...nativeSegs.slice(0, keep), ...tail], keepPrefix: keep };
}
