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

// TFDT_TRUST_SEC bounds how far a segment's own decode time may sit from its
// playlist slot and still be believed. The AC-3 copy cuts on the codec's 32 ms
// frames, so its real start lands up to a frame after the slot; a timeline
// with its own origin (an ffmpeg copy starting at 1.4 s) is not that, and
// keeps the playlist's placement.
const TFDT_TRUST_SEC = 0.25;

// pcmPlacement says where a decoded PCM segment goes on the media timeline:
// from which media time it plays (startMedia), at which offset into the buffer
// (bufferOffset), and for how long (playSec). contentStartSec is the media
// time of the buffer's first sample, from the segment's tfdt.
//
// Placing the buffer at its slot instead (n x 6.006 s) played the sound up to
// 30 ms early, 15 ms on average, and every boundary either repeated 10 ms or
// left a 22 ms hole: the 188- or 187-frame segments last 6.016 or 5.984 s,
// not 6.006 (prod, The Terminal List S02E02, 2026-10-22). At their own start
// the buffers are contiguous, so each one plays out in full.
export function pcmPlacement(
  seg: Segment,
  bufferSec: number,
  contentStartSec: number | undefined,
): { startMedia: number; bufferOffset: number; playSec: number } {
  const trim = seg.trimSec ?? 0;
  const slotStart = seg.start + trim;
  if (contentStartSec === undefined || Math.abs(contentStartSec - seg.start) > TFDT_TRUST_SEC) {
    return { startMedia: slotStart, bufferOffset: trim, playSec: seg.dur - trim };
  }
  const startMedia = Math.max(slotStart, contentStartSec);
  const bufferOffset = startMedia - contentStartSec;
  return { startMedia, bufferOffset, playSec: bufferSec - bufferOffset };
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
