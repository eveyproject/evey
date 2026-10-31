import { useEffect, useRef, useState, useCallback, type MutableRefObject } from "react";
import { Pressable, Text, View, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import { getPlayhead, usePlayheadSelector } from "@evey/client";
import { useTheme } from "../providers/ThemeProvider";
import { useMediaSegments } from "../hooks/useMediaSegments";
import { FONT_SIZE } from "@evey/theme";

// The rules live in @evey/client (skipSegments.ts): which segments are worth
// a button, what to call them, and which one wins when two overlap.
import {
  CREDITS_COUNTDOWN_SEC,
  SKIPPABLE_SEGMENTS as SKIPPABLE,
  SKIP_LABEL_KEY as LABEL_KEY,
  activeSkippable as active,
  skippableCandidates,
  type ResolvedSegment,
  useChapterSkippables,
  useCreditsCountdown,
} from "@evey/client";

export { CREDITS_COUNTDOWN_SEC };

interface SkipIntroButtonProps {
  mediaId?: string;
  /** MKV chapters fallback while the server has no segment for this title. */
  filePath?: string;
  /** Given by a caller that drives the time itself; the playhead store otherwise. */
  currentTime?: number;
  onSkip: (toSec: number) => void;
  // Reports whether the button is currently showing, so the player screen can
  // include it in its overlay set (the button holds TV focus while visible, so
  // OK must not ALSO toggle play/pause - the double-fire the audit found).
  onVisibleChange?: (visible: boolean) => void;
  // Reports whether THIS button specifically holds TV focus, distinct from
  // onVisibleChange: the global remote handler must only swallow "select"
  // while the button is the one about to receive it, not merely on screen -
  // see [id].tsx's skipFocused.
  onFocusChange?: (focused: boolean) => void;
  // True while the NextEpisodeOverlay card is also on screen (the last 30s of
  // an episode can carry a recap/credits segment) - lifts the button clear of
  // the card instead of stacking two focusable targets in the same strip.
  liftForNextEpisode?: boolean;
  // When provided AND the active segment is the end credits, a second
  // Pressable appears alongside "Skip credits" so the viewer does not have to
  // dismiss the credits before reaching the next episode.
  onNextEpisode?: () => void;
  /** The profile's auto-play choice. On, the next-episode button counts down
   *  through the credits and fires on its own; "Keep watching" cancels it. */
  autoAdvance?: boolean;
  /** Armed with the cancel while the countdown runs, null otherwise, so the
   *  screen can make BACK "keep watching" before it means "leave". */
  cancelCountdownRef?: MutableRefObject<(() => void) | null>;
}

export function SkipIntroButton({
  mediaId,
  filePath,
  currentTime: givenTime,
  onSkip,
  onVisibleChange,
  onFocusChange,
  liftForNextEpisode,
  onNextEpisode,
  autoAdvance = false,
  cancelCountdownRef,
}: SkipIntroButtonProps) {
  const { t } = useTranslation();
  const theme = useTheme();
  // Which of the (up to two) Pressables currently holds focus. Both report
  // through the same onFocusChange - the screen only needs to know that ONE
  // of them is focused, not which.
  const [focusedButton, setFocusedButton] = useState<"skip" | "next" | null>(null);
  const reportFocus = useCallback(
    (button: "skip" | "next" | null) => {
      setFocusedButton(button);
      onFocusChange?.(button !== null);
    },
    [onFocusChange],
  );
  const handleSkipFocus = useCallback(() => reportFocus("skip"), [reportFocus]);
  const handleSkipBlur = useCallback(() => reportFocus(null), [reportFocus]);
  const handleNextFocus = useCallback(() => reportFocus("next"), [reportFocus]);
  const handleNextBlur = useCallback(() => reportFocus(null), [reportFocus]);

  const { data } = useMediaSegments(mediaId, [...SKIPPABLE]);
  const segments = data?.segments;
  const autoSkip = data?.introAutoSkip === true;

  // The server's segments first; the file's own chapters while it has none.
  const hasSegments = !!segments && segments.length > 0;
  const chapterCandidates = useChapterSkippables(filePath, !hasSegments);
  const candidates: ResolvedSegment[] = hasSegments ? skippableCandidates(segments) : chapterCandidates;
  // Follow the playhead only while inside a skippable segment, and then only to
  // the second: outside one (most of a film) the selector returns a constant so
  // the button stops re-rendering twice a second for nothing. A caller that
  // drives the time itself (givenTime) keeps that path, re-rendering on its prop.
  const segmentTick = usePlayheadSelector((sec) => {
    const at = givenTime ?? sec;
    return active(candidates, at) ? Math.floor(at) : -1;
  });
  const currentTime = givenTime ?? (segmentTick === -1 ? getPlayhead() : segmentTick);
  const resolved = active(candidates, currentTime);

  // Auto-skip (admin toggle): jump the intro instead of waiting for OK. Intro
  // type only - a recap or the credits stay a manual choice. One jump per
  // segment per media: a viewer who seeks back into the intro is choosing to
  // watch it, and gets the button, not another jump. Mirrors web and mobile,
  // which have had this since the toggle shipped; the TV player did not, so
  // the setting silently did nothing on this one player.
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
    // Silent before this fix: web and mobile have told the viewer since the
    // toggle shipped, the TV player jumped without a word.
    toast.info(t("player.introSkipped"));
  }, [autoTarget, t]);

  // Credits countdown: @evey/client's, shared with the browser and the phone.
  // It starts where the viewer entered the credits and "Keep watching"
  // cancels it for these credits.
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

  // Hook must run before the early return below (rules of hooks). While an
  // auto-skip is pending the button is on its way out, so it must not claim
  // TV focus - otherwise OK would land on a control about to disappear.
  const visible = resolved !== null && autoTarget === null;
  useEffect(() => {
    onVisibleChange?.(visible);
    // The button unmounts the instant it stops being active - its own onBlur
    // is not guaranteed to fire before that, which would leave the screen
    // thinking OK still belongs to a control that is already gone.
    if (!visible) onFocusChange?.(false);
    return () => {
      onVisibleChange?.(false);
      onFocusChange?.(false);
    };
  }, [visible, onVisibleChange, onFocusChange]);

  if (!resolved || autoTarget) return null;

  const labelKey = LABEL_KEY[resolved.type];
  const showNextEpisode = resolved.type === "end_credits" && !!onNextEpisode;
  const buttonStyle = (button: "skip" | "next") => [
    styles.button,
    {
      borderColor: focusedButton === button ? theme.colors.accent : theme.colors.border,
      backgroundColor: theme.colors.overlay,
    },
  ];

  return (
    // Bare Pressables rather than FocusableCard: the player screen needs to
    // know whether THIS control specifically holds TV focus (see
    // SyncplaySessionBadge for the same reasoning), which FocusableCard does
    // not expose to its caller. During the end credits a second button, "Next
    // episode", sits in the same row: a plain row lets the D-pad move between
    // the two natively, and only the first one claims the preferred focus.
    <View style={[styles.row, liftForNextEpisode && styles.rowRaised]}>
      <Pressable
        hasTVPreferredFocus
        onPress={() => (counting ? watchCredits() : onSkip(resolved.endSec))}
        onFocus={handleSkipFocus}
        onBlur={handleSkipBlur}
        accessible
        accessibilityRole="button"
        accessibilityLabel={counting ? t("player.watchCredits") : t(labelKey)}
        style={buttonStyle("skip")}
      >
        <Text style={[styles.text, { color: theme.colors.text }]}>
          {counting ? t("player.watchCredits") : t(labelKey)}
        </Text>
      </Pressable>
      {showNextEpisode && (
        <Pressable
          onPress={onNextEpisode}
          onFocus={handleNextFocus}
          onBlur={handleNextBlur}
          accessible
          accessibilityRole="button"
          accessibilityLabel={t("player.nextEpisodeOverlay.nextEpisode")}
          style={buttonStyle("next")}
        >
          <Text style={[styles.text, { color: theme.colors.text }]}>
            {counting
              ? t("player.nextEpisodeOverlay.nextIn", { seconds: remaining })
              : t("player.nextEpisodeOverlay.nextEpisode")}
          </Text>
        </Pressable>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  // The row anchors the corner; the buttons inside only carry their look.
  row: {
    position: "absolute",
    bottom: 140,
    right: 40,
    flexDirection: "row",
    gap: 10,
  },
  // Clears the NextEpisodeOverlay card (bottom: 60, ~150px tall) instead of
  // sitting under it during the last 30s of an episode.
  rowRaised: {
    bottom: 240,
  },
  button: {
    borderWidth: 1,
    borderRadius: 8,
    paddingHorizontal: 28,
    paddingVertical: 10,
  },
  text: {
    fontSize: FONT_SIZE.lg,
    fontWeight: "600",
  },
});
