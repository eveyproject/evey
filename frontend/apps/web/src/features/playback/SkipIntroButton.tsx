import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { mKVServiceListChapters } from "@evey/api";
import { toast } from "@evey/store";
import {
  CREDITS_COUNTDOWN_SEC,
  SKIPPABLE_SEGMENTS,
  SKIP_LABEL_KEY,
  activeSkippable,
  skippableCandidates,
  skippableFromChapters,
  type ResolvedSegment,
} from "@evey/client";
import { useMediaSegments } from "@/hooks/useMediaSegments";

// Segment types the button can skip, with their i18n label. The label is
// chosen from whichever segment the playhead is currently inside, so the
// same button reads "Skip Intro", "Skip Recap" or "Skip Credits" as
// playback moves through the episode.
// "preview" (next-episode preview reel) is admin-gated server-side: it is
// only present in the API response when the admin enables it, so listing it
// here is safe - no button appears unless the server actually returns one.
// With auto-play on, the next episode starts this long after the credits
// begin unless the viewer asks to watch them (@evey/client skipSegments).
export { CREDITS_COUNTDOWN_SEC } from "@evey/client";

interface SkipIntroButtonProps {
  mediaId?: string; // segments API (priority)
  filePath?: string; // MKV chapters fallback
  currentTime: number;
  onSkip: (toSec: number) => void;
  /** Jump straight to the next episode. When given, the end-credits segment
   *  offers it next to "Skip credits": skipping the credits only lands on the
   *  end of the episode, most viewers want the next one. */
  onNextEpisode?: () => void;
  /** Render nothing - the next-episode card is on screen at the same spot
   *  and already offers "Play now"; two overlapping prompts help nobody. */
  hidden?: boolean;
  /** The profile's auto-play choice. On, the next-episode button counts down
   *  through the credits and fires on its own; "Watch credits" cancels it. */
  autoAdvance?: boolean;
}

const BUTTON_CLASS =
  "px-5 py-2.5 text-sm font-semibold text-white rounded-md transition-all duration-200 hover:scale-105 bg-[var(--overlay-btn-bg)] border border-[var(--overlay-dot-inactive)] backdrop-blur-[8px]";

export function SkipIntroButton({ mediaId, filePath, currentTime, onSkip, onNextEpisode, hidden, autoAdvance = false }: SkipIntroButtonProps) {
  const { t } = useTranslation();

  const { data: segmentsData } = useMediaSegments(mediaId, [...SKIPPABLE_SEGMENTS]);
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
    resolved = activeSkippable(skippableCandidates(segments), currentTime);
  } else if (chaptersData?.chapters) {
    resolved = activeSkippable(skippableFromChapters(chaptersData.chapters), currentTime);
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

  // Credits countdown. Cancelled per segment (keyed on its start, like the
  // auto-skip): "Watch credits" is a decision for this episode's credits, a
  // seek back into them must not restart the clock on the viewer.
  const withNext = resolved?.type === "end_credits" && !!onNextEpisode;
  const [cancelledCredits, setCancelledCredits] = useState<Set<number>>(() => new Set());
  const [lastCancelMedia, setLastCancelMedia] = useState(mediaId);
  if (lastCancelMedia !== mediaId) {
    setLastCancelMedia(mediaId);
    setCancelledCredits(new Set());
  }
  const counting =
    withNext && autoAdvance && resolved !== null && !cancelledCredits.has(resolved.startSec);
  const remaining = counting && resolved
    ? Math.max(0, Math.ceil(Math.min(resolved.startSec + CREDITS_COUNTDOWN_SEC, resolved.endSec) - currentTime))
    : null;
  const onNextEpisodeRef = useRef(onNextEpisode);
  onNextEpisodeRef.current = onNextEpisode;
  const firedRef = useRef<number | null>(null);
  useEffect(() => {
    if (remaining !== 0 || !resolved || firedRef.current === resolved.startSec) return;
    firedRef.current = resolved.startSec;
    onNextEpisodeRef.current?.();
  }, [remaining, resolved]);

  if (!resolved || hidden) return null;

  const labelKey = SKIP_LABEL_KEY[resolved.type];

  return (
    <div className="absolute bottom-28 right-4 z-float flex gap-2 animate-fade-in">
      {counting ? (
        <button
          onClick={() => setCancelledCredits((prev) => new Set(prev).add(resolved.startSec))}
          className={BUTTON_CLASS}
          aria-label={t("player.watchCredits")}
        >
          {t("player.watchCredits")}
        </button>
      ) : (
        <button
          onClick={() => onSkip(resolved.endSec)}
          className={BUTTON_CLASS}
          aria-label={t(labelKey)}
        >
          {t(labelKey)}
        </button>
      )}
      {withNext && (
        <button
          onClick={onNextEpisode}
          className={BUTTON_CLASS}
          aria-label={t("player.nextEpisodeOverlay.nextEpisode")}
        >
          {counting
            ? t("player.nextEpisodeOverlay.nextIn", { seconds: remaining })
            : t("player.nextEpisodeOverlay.nextEpisode")}
        </button>
      )}
    </div>
  );
}
