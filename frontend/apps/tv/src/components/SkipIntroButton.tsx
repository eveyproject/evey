import { useEffect, useRef } from "react";
import { Text, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useTheme } from "../providers/ThemeProvider";
import { useMediaSegments } from "../hooks/useMediaSegments";
import { FocusableCard } from "./FocusableCard";
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
  for (const c of candidates) {
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
}

export function SkipIntroButton({ mediaId, currentTime, onSkip, onVisibleChange }: SkipIntroButtonProps) {
  const { t } = useTranslation();
  const theme = useTheme();

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
  }, [autoTarget]);

  // Hook must run before the early return below (rules of hooks). While an
  // auto-skip is pending the button is on its way out, so it must not claim
  // TV focus - otherwise OK would land on a control about to disappear.
  const visible = resolved !== null && autoTarget === null;
  useEffect(() => {
    onVisibleChange?.(visible);
    return () => onVisibleChange?.(false);
  }, [visible, onVisibleChange]);

  if (!resolved || autoTarget) return null;

  const labelKey = LABEL_KEY[resolved.type];

  return (
    <FocusableCard
      hasTVPreferredFocus
      onPress={() => onSkip(resolved.endSec)}
      accessibilityLabel={t(labelKey)}
      style={[styles.button, { borderColor: theme.colors.border, backgroundColor: theme.colors.overlay }]}
    >
      <Text style={[styles.text, { color: theme.colors.text }]}>{t(labelKey)}</Text>
    </FocusableCard>
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
  text: {
    fontSize: FONT_SIZE.lg,
    fontWeight: "600",
  },
});
