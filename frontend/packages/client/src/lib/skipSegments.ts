// Which segment the "skip" button is offering right now, and what to call it.
//
// The rules the phone, the browser and both televisions share: a segment has
// to be long enough to be worth a button, and when a recap and an intro claim
// the same seconds the earliest one wins (the server's array order used to
// decide, so a recap listed first shadowed an intro that started sooner).

export const SKIPPABLE_SEGMENTS = ["intro", "recap", "end_credits", "preview"] as const;
export type SkippableType = (typeof SKIPPABLE_SEGMENTS)[number];

export const SKIP_LABEL_KEY: Record<SkippableType, string> = {
  intro: "player.skipIntro",
  recap: "player.skipRecap",
  end_credits: "player.skipCredits",
  preview: "player.skipPreview",
};

export const MIN_SKIPPABLE_SEC = 5;

// With auto-play on, the next episode starts this long after the credits
// begin unless the viewer asks to watch them. Measured on the playhead, not
// the clock: a pause freezes it, and it can never outlive the segment.
export const CREDITS_COUNTDOWN_SEC = 15;

export interface ResolvedSegment {
  type: SkippableType;
  startSec: number;
  endSec: number;
}

/** The segment the playhead is inside, or null. */
export function activeSkippable(candidates: readonly ResolvedSegment[], currentTime: number): ResolvedSegment | null {
  const sorted = [...candidates].sort((a, b) => a.startSec - b.startSec);
  for (const c of sorted) {
    if (c.endSec <= c.startSec) continue;
    if (c.endSec - c.startSec < MIN_SKIPPABLE_SEC) continue;
    if (currentTime >= c.startSec && currentTime < c.endSec) return c;
  }
  return null;
}

/** The server's segments, kept to the skippable ones and put in seconds. */
export function skippableCandidates(segments: readonly { type?: string; startMs?: number; endMs?: number }[] | undefined): ResolvedSegment[] {
  return (segments ?? [])
    .filter((s) => (SKIPPABLE_SEGMENTS as readonly string[]).includes(s.type ?? ""))
    .map((s) => ({ type: s.type as SkippableType, startSec: (s.startMs ?? 0) / 1000, endSec: (s.endMs ?? 0) / 1000 }));
}

// MKV chapter title to segment type. Mirrors the server-side mapping in
// intro_detection_service.go, so the client fallback (used while the server
// has not run detection yet) recognises the same chapters.
const CHAPTER_PATTERNS: { type: SkippableType; re: RegExp }[] = [
  { type: "intro", re: /^(intro|introduction|opening|op$|op\s)/i },
  { type: "recap", re: /(recap|previously)/i },
  { type: "end_credits", re: /(credit|ending|outro|^ed$|^ed\s)/i },
];

/** The file's own chapters that name a skippable part, put in seconds. */
export function skippableFromChapters(chapters: readonly { title?: string; startMs?: string | number; endMs?: string | number }[] | undefined): ResolvedSegment[] {
  const out: ResolvedSegment[] = [];
  for (const ch of chapters ?? []) {
    const title = ch.title?.trim();
    if (!title) continue;
    const match = CHAPTER_PATTERNS.find((p) => p.re.test(title));
    if (match) out.push({ type: match.type, startSec: Number(ch.startMs ?? 0) / 1000, endSec: Number(ch.endMs ?? 0) / 1000 });
  }
  return out;
}
