import { Text, Pressable, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { BottomSheet } from "./BottomSheet";
import { useTheme } from "../providers/ThemeProvider";
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
  const { t } = useTranslation();
  const theme = useTheme();

  return (
    <BottomSheet visible={visible} onClose={onClose} title={t("player.controls.chapters")}>
      {chapters.map((ch, i) => {
        const startSec = ch.startTimeMs / 1000;
        const isActive =
          currentTimeMs >= ch.startTimeMs &&
          (i === chapters.length - 1 || currentTimeMs < chapters[i + 1].startTimeMs);
        return (
          <Pressable
            key={i}
            style={[styles.row, { borderBottomColor: theme.colors.border }]}
            onPress={() => {
              onSeek(startSec);
              onClose();
            }}
            accessible={true}
            accessibilityLabel={ch.title || `${t("media.chapter")} ${i + 1}`}
            accessibilityRole="button"
          >
            <Text style={[styles.timestamp, { color: theme.colors.textMuted }]}>{formatTime(startSec)}</Text>
            <Text
              style={[styles.label, { color: theme.colors.text }, isActive && { color: theme.colors.accent }]}
              numberOfLines={1}
            >
              {ch.title || `${t("media.chapter")} ${i + 1}`}
            </Text>
            {isActive && <Text style={[styles.check, { color: theme.colors.accent }]}>{"\u2713"}</Text>}
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
    gap: 10,
  },
  timestamp: {
    fontSize: 12,
    fontVariant: ["tabular-nums"],
    width: 52,
  },
  label: {
    fontSize: 15,
    flex: 1,
  },
  check: {
    fontSize: 18,
    fontWeight: "600",
    marginLeft: 12,
  },
});
