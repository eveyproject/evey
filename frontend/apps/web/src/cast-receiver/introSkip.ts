// introSkip.ts - auto-skip of detected intros on the cast receiver.
//
// The web player offers a "skip intro" button; a Chromecast has no input, so
// the TV-native equivalent is the auto-skip (with an on-screen notice, so the
// jump reads as a feature, not a glitch). Only "intro" segments are skipped:
// a recap is content a viewer may want, and end credits belong to the
// next-episode chain (castNext.ts). Pure logic, main.ts owns the timers.

export interface SkippableSegment {
  type: string;
  startSec: number;
  endSec: number;
}

// Bounds on what a plausible intro is: an auto-jump on TV cannot be undone
// from the couch, so degenerate detector output must never fire one.
const MIN_INTRO_SEC = 5;
const MAX_INTRO_SEC = 180;

// buildSegmentsUrl derives the segments endpoint from the playing master's
// URL - token-only query, the caps params mean nothing to it.
export function buildSegmentsUrl(streamUrl: string): string | null {
  try {
    const u = new URL(streamUrl);
    const m = /^(.*\/api\/v1\/media\/[^/]+)\//.exec(u.pathname + "/");
    const token = u.searchParams.get("token");
    if (!m || !token) return null;
    return `${u.origin}${m[1]}/segments?types=intro&token=${encodeURIComponent(token)}`;
  } catch {
    return null;
  }
}

// parseSegments validates the payload into second-based skippable segments,
// dropping anything out of the plausible-intro bounds.
export function parseSegments(data: unknown): SkippableSegment[] {
  if (typeof data !== "object" || data === null) return [];
  const raw = (data as { segments?: unknown }).segments;
  if (!Array.isArray(raw)) return [];
  const out: SkippableSegment[] = [];
  for (const s of raw) {
    if (typeof s !== "object" || s === null) continue;
    const seg = s as Record<string, unknown>;
    if (seg.type !== "intro") continue;
    const startMs = typeof seg.startMs === "number" ? seg.startMs : NaN;
    const endMs = typeof seg.endMs === "number" ? seg.endMs : NaN;
    if (!isFinite(startMs) || !isFinite(endMs)) continue;
    const lenSec = (endMs - startMs) / 1000;
    if (lenSec < MIN_INTRO_SEC || lenSec > MAX_INTRO_SEC) continue;
    out.push({ type: "intro", startSec: startMs / 1000, endSec: endMs / 1000 });
  }
  return out;
}

// planIntroSkip decides whether the playhead sits inside a segment that
// should be jumped. One skip per segment per load (the caller records
// skipped starts): a viewer who deliberately seeks back into the intro is
// choosing to watch it, bouncing them out again would fight the remote.
// The 1s tail guard avoids a pointless jump when the intro is ending anyway.
export function planIntroSkip(
  segments: SkippableSegment[],
  posSec: number,
  skippedStarts: ReadonlySet<number>,
): SkippableSegment | null {
  if (!isFinite(posSec)) return null;
  for (const seg of segments) {
    if (skippedStarts.has(seg.startSec)) continue;
    if (posSec >= seg.startSec && posSec < seg.endSec - 1) return seg;
  }
  return null;
}

// introSkippedLabel is the 3s on-screen notice, in the device language.
export function introSkippedLabel(lang: string): string {
  return lang.toLowerCase().startsWith("fr") ? "Intro passée" : "Intro skipped";
}
