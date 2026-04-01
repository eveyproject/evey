import { View, Text, Pressable, StyleSheet } from "react-native";
import { BottomSheet } from "./BottomSheet";
import type { Chapter } from "../hooks/useChapters";

interface ChaptersPickerProps {
  visible: boolean;
  onClose: () => void;
  chapters: Chapter[];
  currentTimeMs: number;
  onSeek: (timeSec: number) => void;
}

function formatTime(seconds: number): string {
  const s = Math.max(0, Math.floor(seconds));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = s % 60;
  if (h > 0) return `${h}:${String(m).padStart(2, "0")}:${String(sec).padStart(2, "0")}`;
  return `${m}:${String(sec).padStart(2, "0")}`;
}

export function ChaptersPicker({
  visible,
  onClose,
  chapters,
  currentTimeMs,
  onSeek,
}: ChaptersPickerProps) {
  return (
    <BottomSheet visible={visible} onClose={onClose} title="Chapters">
      {chapters.map((ch, i) => {
        const startSec = ch.startTimeMs / 1000;
        const isActive =
          currentTimeMs >= ch.startTimeMs &&
          (i === chapters.length - 1 || currentTimeMs < chapters[i + 1].startTimeMs);
        return (
          <Pressable
            key={i}
            style={styles.row}
            onPress={() => {
              onSeek(startSec);
              onClose();
            }}
            accessible={true}
            accessibilityLabel={ch.title || `Chapter ${i + 1}`}
            accessibilityRole="button"
          >
            <Text style={styles.timestamp}>{formatTime(startSec)}</Text>
            <Text
              style={[styles.label, isActive && styles.activeLabel]}
              numberOfLines={1}
            >
              {ch.title || `Chapter ${i + 1}`}
            </Text>
            {isActive && <Text style={styles.check}>{"\u2713"}</Text>}
          </Pressable>
        );
      })}
    </BottomSheet>
  );
}

const styles = StyleSheet.create({
  row: {
    flexDirection: "row",
    alignItems: "center",
    paddingVertical: 14,
    borderBottomWidth: StyleSheet.hairlineWidth,
    borderBottomColor: "rgba(255,255,255,0.08)",
    gap: 10,
  },
  timestamp: {
    color: "rgba(255,255,255,0.5)",
    fontSize: 12,
    fontVariant: ["tabular-nums"],
    width: 52,
  },
  label: {
    color: "#fff",
    fontSize: 15,
    flex: 1,
  },
  activeLabel: {
    color: "#60a5fa",
  },
  check: {
    color: "#60a5fa",
    fontSize: 18,
    fontWeight: "600",
    marginLeft: 12,
  },
});
