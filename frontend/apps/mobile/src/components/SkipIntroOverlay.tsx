import { useEffect, useRef, useState } from "react";
import { AccessibilityInfo, Pressable, Text, View, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import { useTheme } from "../providers/ThemeProvider";
import { useMediaSegments } from "../hooks/useMediaSegments";
import { FONT_SIZE } from "@evey/theme";
import {
  CREDITS_COUNTDOWN_SEC,
  SKIPPABLE_SEGMENTS,
  SKIP_LABEL_KEY,
  activeSkippable,
  skippableCandidates,
  useChapterSkippables,
  type ResolvedSegment,
  type SkippableType,
} from "@evey/client";

// Segment types the overlay can skip, with their i18n label. The label is
// chosen from whichever segment the playhead is currently inside, so the
// same button reads "Skip Intro" / "Skip Recap" / "Skip Credits".
// "preview" (next-episode preview reel) is admin-gated server-side: it is
// only present in the API response when the admin enables it, so listing it
// here is safe - no button appears unless the server actually returns one.
// Types, labels, minimum length, the active-segment rule and the MKV-chapter
// fallback are @evey/client's (skipSegments), shared with the other players.
export { CREDITS_COUNTDOWN_SEC } from "@evey/client";

interface SkipIntroOverlayProps {
  mediaId?: string; // segments API (priority)
  filePath?: string; // MKV chapters fallback
  currentTime: number;
  onSkip: (toSec: number) => void;
  // Distance from the screen bottom, in px. Defaults to the old hardcoded
  // 100 - callers with a taller bottom control bar pass a bigger value so
  // the button doesn't sit on top of the seek bar.
  bottomOffset?: number;
  // Render nothing while true. Used while the next-episode card is on
  // screen so the two overlays never stack.
  hidden?: boolean;
  // When set and the active segment is end_credits, a second "Next episode"
  // button renders next to the skip button.
  onNextEpisode?: () => void;
  /** The profile's auto-play choice. On, the next-episode button counts down
   *  through the credits and fires on its own; "Watch credits" cancels it. */
  autoAdvance?: boolean;
}

export function SkipIntroOverlay({
  mediaId,
  filePath,
  currentTime,
  onSkip,
  bottomOffset = 100,
  hidden = false,
  onNextEpisode,
  autoAdvance = false,
}: SkipIntroOverlayProps) {
  const { t } = useTranslation();
  const theme = useTheme();

  const { data: segmentsData } = useMediaSegments(mediaId, [...SKIPPABLE_SEGMENTS]);
  const segments = segmentsData?.segments;
  const autoSkip = segmentsData?.introAutoSkip === true;

  // API segments take priority over the file's own MKV chapters.
  const hasSegments = !!segments && segments.length > 0;
  const chapterCandidates = useChapterSkippables(filePath, !hasSegments);
  const resolved: ResolvedSegment | null = activeSkippable(hasSegments ? skippableCandidates(segments) : chapterCandidates, currentTime);

  // Auto-skip (admin toggle): jump the intro instead of waiting for the tap.
  // Intro type only - a recap or the credits stay a manual choice. One jump
  // per segment per media: a viewer who seeks back into the intro is
  // choosing to watch it, and gets the button, not another jump.
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

  // The button appears on its own, a few seconds into a segment, and vanishes
  // on its own at the end of it. Sighted viewers see it arrive; nobody else
  // did, and a chance to skip that is never announced is a chance that does
  // not exist. Announced once per segment - re-announcing on every tick would
  // talk over the film.
  const announcedRef = useRef<string | null>(null);
  const announceKey = resolved ? `${resolved.type}:${resolved.startSec}` : null;
  useEffect(() => {
    if (!announceKey || announcedRef.current === announceKey) return;
    announcedRef.current = announceKey;
    const type = announceKey.slice(0, announceKey.indexOf(":")) as SkippableType;
    AccessibilityInfo.announceForAccessibility(t(SKIP_LABEL_KEY[type]));
  }, [announceKey, t]);

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
  // Skipping the credits only lands on the end of the episode; most viewers
  // want the next one, so the credits button comes with a second one.
  const showNextEpisode = withNext;
  const buttonStyle = [styles.button, { borderColor: theme.colors.border, backgroundColor: theme.colors.overlay }];

  return (
    <View style={[styles.row, { bottom: bottomOffset }]}>
      {counting ? (
        <Pressable
          style={buttonStyle}
          onPress={() => setCancelledCredits((prev) => new Set(prev).add(resolved.startSec))}
          accessible={true}
          accessibilityLabel={t("player.watchCredits")}
          accessibilityRole="button"
        >
          <Text style={[styles.text, { color: theme.colors.text }]}>{t("player.watchCredits")}</Text>
        </Pressable>
      ) : (
        <Pressable
          style={buttonStyle}
          onPress={() => onSkip(resolved.endSec)}
          accessible={true}
          accessibilityLabel={t(labelKey)}
          accessibilityRole="button"
        >
          <Text style={[styles.text, { color: theme.colors.text }]}>{t(labelKey)}</Text>
        </Pressable>
      )}
      {showNextEpisode && (
        <Pressable
          style={buttonStyle}
          onPress={onNextEpisode}
          accessible={true}
          accessibilityLabel={t("player.nextEpisodeOverlay.nextEpisode")}
          accessibilityRole="button"
        >
          <Text style={[styles.text, { color: theme.colors.text }]}>
            {counting ? t("player.nextEpisodeOverlay.nextIn", { seconds: remaining }) : t("player.nextEpisodeOverlay.nextEpisode")}
          </Text>
        </Pressable>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  row: {
    position: "absolute",
    right: 20,
    flexDirection: "row",
    gap: 8,
  },
  button: {
    borderWidth: 1,
    borderRadius: 8,
    paddingHorizontal: 20,
    paddingVertical: 10,
  },
  text: {
    fontSize: FONT_SIZE.sm,
    fontWeight: "600",
  },
});
