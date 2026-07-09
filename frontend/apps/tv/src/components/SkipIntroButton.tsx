import { Text, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useTheme } from "../providers/ThemeProvider";
import { useMediaSegments } from "../hooks/useMediaSegments";
import { FocusableCard } from "./FocusableCard";

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
}

export function SkipIntroButton({ mediaId, currentTime, onSkip }: SkipIntroButtonProps) {
  const { t } = useTranslation();
  const theme = useTheme();

  const { data: segments } = useMediaSegments(mediaId, [...SKIPPABLE]);

  const candidates: ResolvedSegment[] = (segments ?? [])
    .filter((s) => (SKIPPABLE as readonly string[]).includes(s.type))
    .map((s) => ({ type: s.type as SkippableType, startSec: s.startMs / 1000, endSec: s.endMs / 1000 }));
  const resolved = active(candidates, currentTime);

  if (!resolved) return null;

  const labelKey = LABEL_KEY[resolved.type];

  return (
    <FocusableCard
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
    fontSize: 18,
    fontWeight: "600",
  },
});
