import { View, Text, Modal, Pressable, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useTheme } from "../providers/ThemeProvider";
import { FocusableCard } from "./FocusableCard";
import { FONT_SIZE } from "@evey/theme";

interface TrackOptionsMenuProps {
  visible: boolean;
  onClose: () => void;
  hasAudioOptions: boolean;
  hasSubtitleOptions: boolean;
  onOpenAudio: () => void;
  onOpenSubtitles: () => void;
  // All optional/additive: every existing call site (just Audio + Subtitles)
  // keeps working unchanged.
  hasChapters?: boolean;
  onOpenChapters?: () => void;
  onOpenSleepTimer?: () => void;
  onOpenVolume?: () => void;
  hasStreamInfo?: boolean;
  onOpenStreamInfo?: () => void;
}

/**
 * Entry point opened via a long-press of the D-pad select button. The main
 * player screen already claims every plain D-pad direction (seek/speed/
 * quality) plus their long-press variants (accelerated seek, prev/next
 * episode), so every other option lives one level down behind this menu
 * instead of competing for a free key combination.
 *
 * Rows are built as a single filtered list so exactly one gets
 * hasTVPreferredFocus, regardless of which rows a given call site opts into -
 * two concurrent hasTVPreferredFocus on conditionally-rendered rows is the bug
 * this shape avoids.
 */
export function TrackOptionsMenu({
  visible,
  onClose,
  hasAudioOptions,
  hasSubtitleOptions,
  onOpenAudio,
  onOpenSubtitles,
  hasChapters,
  onOpenChapters,
  onOpenSleepTimer,
  onOpenVolume,
  hasStreamInfo,
  onOpenStreamInfo,
}: TrackOptionsMenuProps) {
  const { t } = useTranslation();
  const theme = useTheme();

  const items: { key: string; label: string; onPress: () => void }[] = [];
  if (hasAudioOptions) items.push({ key: "audio", label: t("player.settings.audio"), onPress: onOpenAudio });
  if (hasSubtitleOptions) items.push({ key: "subtitles", label: t("player.settings.subtitles"), onPress: onOpenSubtitles });
  if (hasChapters && onOpenChapters) {
    items.push({ key: "chapters", label: t("player.controls.chapters", "Chapters"), onPress: onOpenChapters });
  }
  if (onOpenVolume) items.push({ key: "volume", label: t("player.controls.volume", "Volume"), onPress: onOpenVolume });
  if (onOpenSleepTimer) {
    items.push({ key: "sleepTimer", label: t("player.settings.sleepTimer", "Sleep timer"), onPress: onOpenSleepTimer });
  }
  if (hasStreamInfo && onOpenStreamInfo) {
    items.push({ key: "streamInfo", label: t("player.streamInfo.title", "Stream Info"), onPress: onOpenStreamInfo });
  }

  return (
    <Modal visible={visible} animationType="fade" transparent onRequestClose={onClose}>
      <Pressable style={styles.backdrop} onPress={onClose}>
        <Pressable style={[styles.sheet, { backgroundColor: theme.colors.surface }]} onPress={(e) => e.stopPropagation()}>
          <Text style={[styles.title, { color: theme.colors.text }]}>
            {t("player.settings.trackOptions", "Audio & Subtitles")}
          </Text>
          <View style={styles.list}>
            {items.map((item, i) => (
              <FocusableCard
                key={item.key}
                hasTVPreferredFocus={i === 0}
                onPress={item.onPress}
                accessibilityLabel={item.label}
                style={[styles.row, { borderBottomColor: theme.colors.border }]}
              >
                <Text style={[styles.label, { color: theme.colors.text }]}>{item.label}</Text>
              </FocusableCard>
            ))}
          </View>
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
    minWidth: 360,
    maxWidth: 480,
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
    paddingVertical: 12,
    paddingHorizontal: 16,
    borderRadius: 8,
  },
  label: {
    fontSize: FONT_SIZE.base,
  },
});
