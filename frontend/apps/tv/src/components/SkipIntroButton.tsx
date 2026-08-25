import { useEffect, useRef, useState, useCallback } from "react";
import { Pressable, Text, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import { useTheme } from "../providers/ThemeProvider";
import { useMediaSegments } from "../hooks/useMediaSegments";
import { FONT_SIZE } from "@evey/theme";

// Segment types the button can skip, with their i18n label. The label is
// chosen from whichever segment the playhead is currently inside, so the
// same button reads "Skip Intro" / "Skip Recap" / "Skip Credits". Mirrors the
// mobile SkipIntroOverlay.
const SKIPPABLE = ["intro", "recap", "end_credits", "preview"] as const;
type SkippableType = (typeof SKIPPABLE)[number];

const LABEL_KEY: Record<SkippableType, string> = {
  intro: "player.skipIntro",
  recap: "player.skipRecap",
  end_credits: "player.skipCredits",
  preview: "player.skipPreview",
};

const MIN_SKIPPABLE_SEC = 5;

interface ResolvedSegment {
  type: SkippableType;
  startSec: number;
  endSec: number;
}

function active(candidates: ResolvedSegment[], currentTime: number): ResolvedSegment | null {
  // A recap and an intro can overlap (both can claim the same seconds near a
  // cold open). The server's own array order used to decide the winner - here
  // it is the earliest-starting segment, so a recap listed first no longer
  // shadows an intro that actually started sooner.
  const sorted = [...candidates].sort((a, b) => a.startSec - b.startSec);
  for (const c of sorted) {
    if (c.endSec <= c.startSec) continue;
    if (c.endSec - c.startSec < MIN_SKIPPABLE_SEC) continue;
    if (currentTime >= c.startSec && currentTime < c.endSec) return c;
  }
  return null;
}

interface SkipIntroButtonProps {
  mediaId?: string;
  currentTime: number;
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
}

export function SkipIntroButton({
  mediaId,
  currentTime,
  onSkip,
  onVisibleChange,
  onFocusChange,
  liftForNextEpisode,
}: SkipIntroButtonProps) {
  const { t } = useTranslation();
  const theme = useTheme();
  const [focused, setFocused] = useState(false);
  const handleFocus = useCallback(() => {
    setFocused(true);
    onFocusChange?.(true);
  }, [onFocusChange]);
  const handleBlur = useCallback(() => {
    setFocused(false);
    onFocusChange?.(false);
  }, [onFocusChange]);

  const { data } = useMediaSegments(mediaId, [...SKIPPABLE]);
  const segments = data?.segments;
  const autoSkip = data?.introAutoSkip === true;

  const candidates: ResolvedSegment[] = (segments ?? [])
    .filter((s) => (SKIPPABLE as readonly string[]).includes(s.type))
    .map((s) => ({ type: s.type as SkippableType, startSec: s.startMs / 1000, endSec: s.endMs / 1000 }));
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

  return (
    // A bare Pressable rather than FocusableCard: the player screen needs to
    // know whether THIS button specifically holds TV focus (see
    // SyncplaySessionBadge for the same reasoning), which FocusableCard does
    // not expose to its caller.
    <Pressable
      hasTVPreferredFocus
      onPress={() => onSkip(resolved.endSec)}
      onFocus={handleFocus}
      onBlur={handleBlur}
      accessible
      accessibilityRole="button"
      accessibilityLabel={t(labelKey)}
      style={[
        styles.button,
        liftForNextEpisode && styles.buttonRaised,
        {
          borderColor: focused ? theme.colors.accent : theme.colors.border,
          backgroundColor: theme.colors.overlay,
        },
      ]}
    >
      <Text style={[styles.text, { color: theme.colors.text }]}>{t(labelKey)}</Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  button: {
    position: "absolute",
    bottom: 140,
    right: 40,
    borderWidth: 1,
    borderRadius: 8,
    paddingHorizontal: 28,
    paddingVertical: 14,
  },
  // Clears the NextEpisodeOverlay card (bottom: 60, ~150px tall) instead of
  // sitting under it during the last 30s of an episode.
  buttonRaised: {
    bottom: 240,
  },
  text: {
    fontSize: FONT_SIZE.lg,
    fontWeight: "600",
  },
});
