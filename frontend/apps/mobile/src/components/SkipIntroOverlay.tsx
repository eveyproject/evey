import { Pressable, Text, StyleSheet } from "react-native";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { mKVServiceListChapters } from "@evey/api";
import { useTheme } from "../providers/ThemeProvider";

// Matches "intro", "introduction", "opening", "op" (standalone or followed by space)
const INTRO_PATTERNS = /^(intro|introduction|opening|op$|op\s)/i;

interface SkipIntroOverlayProps {
  filePath?: string;
  currentTime: number;
  onSkip: (toSec: number) => void;
}

export function SkipIntroOverlay({ filePath, currentTime, onSkip }: SkipIntroOverlayProps) {
  const { t } = useTranslation();
  const theme = useTheme();

  const { data } = useQuery({
    queryKey: ["/api/v1/mkv/chapters", filePath],
    queryFn: ({ signal }) => mKVServiceListChapters({ path: filePath }, signal),
    enabled: !!filePath,
    staleTime: Infinity,
  });

  const introChapter = data?.chapters?.find(
    (ch) => ch.title && INTRO_PATTERNS.test(ch.title.trim()),
  );

  if (!introChapter) return null;

  const startSec = Number(introChapter.startMs ?? 0) / 1000;
  const endSec = Number(introChapter.endMs ?? 0) / 1000;

  if (endSec <= startSec) return null;
  if (endSec - startSec < 5) return null;
  if (currentTime < startSec || currentTime >= endSec) return null;

  return (
    <Pressable
      style={[styles.button, { borderColor: theme.colors.border, backgroundColor: theme.colors.overlay }]}
      onPress={() => onSkip(endSec)}
      accessible={true}
      accessibilityLabel={t("player.skipIntro")}
      accessibilityRole="button"
    >
      <Text style={[styles.text, { color: theme.colors.text }]}>{t("player.skipIntro")}</Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  button: {
    position: "absolute",
    bottom: 100,
    right: 20,
    borderWidth: 1,
    borderRadius: 8,
    paddingHorizontal: 20,
    paddingVertical: 10,
  },
  text: {
    fontSize: 14,
    fontWeight: "600",
  },
});
