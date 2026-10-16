import { useEffect, useRef, type MutableRefObject } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import {
  SKIPPABLE_SEGMENTS,
  SKIP_LABEL_KEY,
  activeSkippable,
  skippableCandidates,
  useChapterSkippables,
  useCreditsCountdown,
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
   *  through the credits and fires on its own; "Keep watching" cancels it. */
  autoAdvance?: boolean;
  /** Armed with the cancel while the countdown runs, null otherwise, so the
   *  keyboard can make Escape "keep watching" before it means "leave". */
  cancelCountdownRef?: MutableRefObject<(() => void) | null>;
}

const BUTTON_CLASS =
  "px-5 py-2.5 text-sm font-semibold text-white rounded-md transition-all duration-200 hover:scale-105 bg-[var(--overlay-btn-bg)] border border-[var(--overlay-dot-inactive)] backdrop-blur-[8px]";

export function SkipIntroButton({ mediaId, filePath, currentTime, onSkip, onNextEpisode, hidden, autoAdvance = false, cancelCountdownRef }: SkipIntroButtonProps) {
  const { t } = useTranslation();

  const { data: segmentsData } = useMediaSegments(mediaId, [...SKIPPABLE_SEGMENTS]);
  const segments = segmentsData?.segments;
  const autoSkip = segmentsData?.introAutoSkip === true;

  // API segments take priority over the file's own MKV chapters.
  const hasSegments = !!segments && segments.length > 0;
  const chapterCandidates = useChapterSkippables(filePath, !hasSegments);
  const resolved: ResolvedSegment | null = activeSkippable(hasSegments ? skippableCandidates(segments) : chapterCandidates, currentTime);

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

  // Credits countdown: @evey/client's, shared with the phone and the
  // televisions. It starts where the viewer entered the credits and "Watch
  // credits" cancels it for these credits.
  const withNext = resolved?.type === "end_credits" && !!onNextEpisode;
  const { counting, remaining, watchCredits } = useCreditsCountdown({
    segment: resolved,
    currentTime,
    mediaId,
    enabled: autoAdvance,
    onNextEpisode,
  });
  useEffect(() => {
    if (!cancelCountdownRef) return;
    cancelCountdownRef.current = counting ? watchCredits : null;
    return () => {
      cancelCountdownRef.current = null;
    };
  }, [cancelCountdownRef, counting, watchCredits]);

  if (!resolved || hidden) return null;

  const labelKey = SKIP_LABEL_KEY[resolved.type];

  return (
    <div className="absolute bottom-28 right-4 z-float flex gap-2 animate-fade-in">
      {counting ? (
        <button
          onClick={watchCredits}
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
