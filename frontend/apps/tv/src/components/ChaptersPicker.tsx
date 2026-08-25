import { Text, Modal, Pressable, ScrollView, StyleSheet } from "react-native";
import { formatTime } from "@evey/api";
import { useTranslation } from "react-i18next";
import { useTheme } from "../providers/ThemeProvider";
import { FocusableCard } from "./FocusableCard";
import { findCurrentChapterIndex, type Chapter } from "../hooks/useChapters";
import { FONT_SIZE } from "@evey/theme";

interface ChaptersPickerProps {
  visible: boolean;
  onClose: () => void;
  chapters: Chapter[];
  currentTimeMs: number;
  onSeek: (timeSec: number) => void;
}

export function ChaptersPicker({ visible, onClose, chapters, currentTimeMs, onSeek }: ChaptersPickerProps) {
  const { t } = useTranslation();
  const theme = useTheme();
  const activeIndex = findCurrentChapterIndex(chapters, currentTimeMs);

  // A hidden RN Modal keeps its children mounted, so this picker re-rendered
  // on every ~2/s currentTime tick of the player while closed. Render nothing
  // until opened.
  if (!visible) return null;

  return (
    <Modal visible={visible} animationType="fade" transparent onRequestClose={onClose}>
      <Pressable style={styles.backdrop} onPress={onClose} focusable={false}>
        <Pressable
          style={[styles.sheet, { backgroundColor: theme.colors.surface }]}
          onPress={(e) => e.stopPropagation()}
          focusable={false}
        >
          <Text style={[styles.title, { color: theme.colors.text }]}>
            {t("player.controls.chapters", "Chapters")}
          </Text>
          <ScrollView style={styles.list}>
            {chapters.map((ch, i) => {
              const startSec = ch.startTimeMs / 1000;
              const active = i === activeIndex;
              const label = ch.title || t("media.chapter", { number: i + 1, defaultValue: `Chapter ${i + 1}` });
              return (
                <FocusableCard
                  key={ch.startTimeMs}
                  hasTVPreferredFocus={i === Math.max(activeIndex, 0)}
                  onPress={() => {
                    onSeek(startSec);
                    onClose();
                  }}
                  accessibilityLabel={active ? `${label}, ${t("common.selected", "selected")}` : label}
                  style={[styles.row, { borderBottomColor: theme.colors.border }]}
                >
                  <Text style={[styles.timestamp, { color: theme.colors.textMuted }]}>{formatTime(startSec)}</Text>
                  <Text style={[styles.label, { color: active ? theme.colors.accent : theme.colors.text }]} numberOfLines={1}>
                    {label}
                  </Text>
                  {active && <Text style={[styles.check, { color: theme.colors.accent }]}>{"✓"}</Text>}
                </FocusableCard>
              );
            })}
          </ScrollView>
        </Pressable>
      </Pressable>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: {
    flex: 1,
    justifyContent: "center",
    alignItems: "center",
    backgroundColor: "rgba(0,0,0,0.7)",
  },
  sheet: {
    borderRadius: 16,
    padding: 24,
    minWidth: 440,
    maxWidth: 600,
    maxHeight: 560,
  },
  title: {
    fontSize: FONT_SIZE.xl,
    fontWeight: "600",
    textAlign: "center",
    marginBottom: 16,
  },
  list: {
    gap: 4,
  },
  row: {
    flexDirection: "row",
    alignItems: "center",
    paddingVertical: 12,
    paddingHorizontal: 16,
    borderRadius: 8,
    gap: 12,
  },
  timestamp: {
    fontSize: FONT_SIZE.sm,
    fontVariant: ["tabular-nums"],
    width: 60,
  },
  label: {
    fontSize: FONT_SIZE.base,
    flex: 1,
  },
  check: {
    fontSize: FONT_SIZE.lg,
    fontWeight: "600",
    marginLeft: 12,
  },
});
