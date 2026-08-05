import { Text, Pressable, StyleSheet } from "react-native";
import { formatTime } from "@evey/api";
import { useTranslation } from "react-i18next";
import { BottomSheet } from "./BottomSheet";
import { useTheme } from "../providers/ThemeProvider";
import type { Chapter } from "../hooks/useChapters";
import { FONT_SIZE } from "@evey/theme";

interface ChaptersPickerProps {
  visible: boolean;
  onClose: () => void;
  chapters: Chapter[];
  currentTimeMs: number;
  onSeek: (timeSec: number) => void;
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
    fontSize: FONT_SIZE.xs,
    fontVariant: ["tabular-nums"],
    width: 52,
  },
  label: {
    fontSize: FONT_SIZE.sm,
    flex: 1,
  },
  check: {
    fontSize: FONT_SIZE.lg,
    fontWeight: "600",
    marginLeft: 12,
  },
});
