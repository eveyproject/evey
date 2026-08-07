import { useEffect, useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { mKVServiceListChapters } from "@evey/api";
import { toast } from "@evey/store";
import { useMediaSegments } from "@/hooks/useMediaSegments";

// Segment types the button can skip, with their i18n label. The label is
// chosen from whichever segment the playhead is currently inside, so the
// same button reads "Skip Intro", "Skip Recap" or "Skip Credits" as
// playback moves through the episode.
// "preview" (next-episode preview reel) is admin-gated server-side: it is
// only present in the API response when the admin enables it, so listing it
// here is safe - no button appears unless the server actually returns one.
const SKIPPABLE = ["intro", "recap", "end_credits", "preview"] as const;
type SkippableType = (typeof SKIPPABLE)[number];

const LABEL_KEY: Record<SkippableType, string> = {
  intro: "player.skipIntro",
  recap: "player.skipRecap",
  end_credits: "player.skipCredits",
  preview: "player.skipPreview",
};

// MKV chapter title → segment type. Mirrors the server-side mapping in
// intro_detection_service.go so the client-only fallback (used when the
// server hasn't run detection yet) recognises the same chapters.
const CHAPTER_PATTERNS: { type: SkippableType; re: RegExp }[] = [
  { type: "intro", re: /^(intro|introduction|opening|op$|op\s)/i },
  { type: "recap", re: /(recap|previously)/i },
  { type: "end_credits", re: /(credit|ending|outro|^ed$|^ed\s)/i },
];

const MIN_SKIPPABLE_SEC = 5;

interface ResolvedSegment {
  type: SkippableType;
  startSec: number;
  endSec: number;
}

interface SkipIntroButtonProps {
  mediaId?: string; // segments API (priority)
  filePath?: string; // MKV chapters fallback
  currentTime: number;
  onSkip: (toSec: number) => void;
}

// active picks the skippable segment the playhead currently sits inside,
// long enough to be worth a button. Returns null when none match - this is
// what gates the button's visibility frame-to-frame. Picking by playhead
// (rather than "first segment") is what lets intro + recap + credits all
// work from one button without the end-credits case being shadowed by an
// earlier intro segment.
function active(
  candidates: ResolvedSegment[],
  currentTime: number,
): ResolvedSegment | null {
  for (const c of candidates) {
    if (c.endSec <= c.startSec) continue;
    if (c.endSec - c.startSec < MIN_SKIPPABLE_SEC) continue;
    if (currentTime >= c.startSec && currentTime < c.endSec) return c;
  }
  return null;
}

export function SkipIntroButton({ mediaId, filePath, currentTime, onSkip }: SkipIntroButtonProps) {
  const { t } = useTranslation();

  const { data: segmentsData } = useMediaSegments(mediaId, [...SKIPPABLE]);
  const segments = segmentsData?.segments;
  const autoSkip = segmentsData?.introAutoSkip === true;

  const { data: chaptersData } = useQuery({
    queryKey: ["/api/v1/mkv/chapters", filePath],
    queryFn: ({ signal }) => mKVServiceListChapters({ path: filePath }, signal),
    // only fetch MKV chapters when no API segments are available
    enabled: !!filePath && (!segments || segments.length === 0),
    staleTime: Infinity,
  });

  // API segments take priority over MKV chapters.
  let resolved: ResolvedSegment | null = null;

  if (segments && segments.length > 0) {
    const apiCandidates: ResolvedSegment[] = segments
      .filter((s) => (SKIPPABLE as readonly string[]).includes(s.type))
      .map((s) => ({ type: s.type as SkippableType, startSec: s.startMs / 1000, endSec: s.endMs / 1000 }));
    resolved = active(apiCandidates, currentTime);
  } else if (chaptersData?.chapters) {
    const chapterCandidates: ResolvedSegment[] = [];
    for (const ch of chaptersData.chapters) {
      const title = ch.title?.trim();
      if (!title) continue;
      const match = CHAPTER_PATTERNS.find((p) => p.re.test(title));
      if (match) {
        chapterCandidates.push({
          type: match.type,
          startSec: Number(ch.startMs ?? 0) / 1000,
          endSec: Number(ch.endMs ?? 0) / 1000,
        });
      }
    }
    resolved = active(chapterCandidates, currentTime);
  }

  // Auto-skip (admin toggle): jump the intro instead of waiting for the
  // click. Intro type only - a recap or the credits stay a manual choice.
  // One jump per segment per media: a viewer who seeks back into the intro
  // is choosing to watch it, and gets the button, not another jump.
  const autoSkippedRef = useRef<Set<number>>(new Set());
  const lastMediaRef = useRef<string | undefined>(mediaId);
  if (lastMediaRef.current !== mediaId) {
    lastMediaRef.current = mediaId;
    autoSkippedRef.current = new Set();
  }
  const autoTarget =
    autoSkip && resolved?.type === "intro" && !autoSkippedRef.current.has(resolved.startSec)
      ? resolved
      : null;
  const onSkipRef = useRef(onSkip);
  onSkipRef.current = onSkip;
  useEffect(() => {
    if (!autoTarget) return;
    autoSkippedRef.current.add(autoTarget.startSec);
    onSkipRef.current(autoTarget.endSec);
    toast.info(t("player.introSkipped"));
  }, [autoTarget, t]);

  if (!resolved) return null;

  const labelKey = LABEL_KEY[resolved.type];

  return (
    <button
      onClick={() => onSkip(resolved.endSec)}
      className="absolute bottom-28 right-4 z-float px-5 py-2.5 text-sm font-semibold text-white rounded-md transition-all duration-200 hover:scale-105 animate-fade-in bg-[var(--overlay-btn-bg)] border border-[var(--overlay-dot-inactive)] backdrop-blur-[8px]"
      aria-label={t(labelKey)}
    >
      {t(labelKey)}
    </button>
  );
}
