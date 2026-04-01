import { Pressable, Text, StyleSheet } from "react-native";
import { useQuery } from "@tanstack/react-query";
import { mKVServiceListChapters } from "@evey/api";

// matches "intro", "introduction", "opening", "op" (standalone or followed by space)
const INTRO_PATTERNS = /^(intro|introduction|opening|op$|op\s)/i;

interface SkipIntroOverlayProps {
  filePath?: string;
  currentTime: number;
  onSkip: (toSec: number) => void;
}

export function SkipIntroOverlay({ filePath, currentTime, onSkip }: SkipIntroOverlayProps) {
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
  if (currentTime < startSec || currentTime >= endSec) return null;

  return (
    <Pressable
      style={styles.button}
      onPress={() => onSkip(endSec)}
      accessible={true}
      accessibilityLabel="Skip Intro"
      accessibilityRole="button"
    >
      <Text style={styles.text}>Skip Intro</Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  button: {
    position: "absolute",
    bottom: 100,
    right: 20,
    backgroundColor: "rgba(0,0,0,0.7)",
    borderWidth: 1,
    borderColor: "rgba(255,255,255,0.3)",
    borderRadius: 8,
    paddingHorizontal: 20,
    paddingVertical: 10,
  },
  text: {
    color: "#fff",
    fontSize: 14,
    fontWeight: "600",
  },
});
