import { View, Text, Modal, Pressable, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useTheme } from "../providers/ThemeProvider";
import { FocusableCard } from "./FocusableCard";
import { FONT_SIZE } from "@evey/theme";

interface TrackOptionsMenuProps {
  visible: boolean;
  onClose: () => void;
  // Audio and subtitles are optional because the player now gives them their
  // own buttons in the control bar: a caller that surfaces them there passes
  // no callback here, and the row simply does not appear.
  hasAudioOptions?: boolean;
  hasSubtitleOptions?: boolean;
  onOpenAudio?: () => void;
  onOpenSubtitles?: () => void;
  // Subtitle appearance (colour, size, background, position). Offered
  // whenever the caller can show subtitles at all - the choice is the
  // viewer's, not the track's.
  onOpenSubtitleStyle?: () => void;
  hasChapters?: boolean;
  onOpenChapters?: () => void;
  // Title of the chapter the playhead is in. Shown - and spoken - on the
  // chapters row, so "where am I" no longer requires opening the picker, the
  // only place the current chapter was ever stated.
  currentChapterTitle?: string;
  onOpenSleepTimer?: () => void;
  onOpenVolume?: () => void;
  hasStreamInfo?: boolean;
  onOpenStreamInfo?: () => void;
  onOpenQuality?: () => void;
  qualityLabel?: string;
  // Speed cycles in place rather than opening a submenu - it has four values
  // and a submenu for four values is a level of nesting nobody wants on a
  // remote. The label shows what the next press will select.
  onCycleSpeed?: () => void;
  speedLabel?: string;
  onPreviousEpisode?: () => void;
  onNextEpisode?: () => void;
  // Per-profile "auto-play next episode" toggle. Mirrors onCycleSpeed's
  // shape: no submenu, the row itself flips the value and shows the result.
  autoplayNext?: boolean;
  onToggleAutoplayNext?: () => void;
}

/**
 * The player's "More" menu: everything that does not earn a permanent button
 * in the control bar.
 *
 * It used to be reachable only by long-pressing OK, alongside a set of other
 * undiscoverable bindings (up cycled speed, down opened quality, long-up and
 * long-down changed episode). Nothing on screen announced any of it, and the
 * owner could not work out how to drive the player at all. Those bindings are
 * gone; their actions live here, behind a labelled button.
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
  onOpenSubtitleStyle,
  onOpenQuality,
  qualityLabel,
  onCycleSpeed,
  speedLabel,
  onPreviousEpisode,
  onNextEpisode,
  autoplayNext,
  onToggleAutoplayNext,
  hasChapters,
  onOpenChapters,
  currentChapterTitle,
  onOpenSleepTimer,
  onOpenVolume,
  hasStreamInfo,
  onOpenStreamInfo,
}: TrackOptionsMenuProps) {
  const { t } = useTranslation();
  const theme = useTheme();

  const items: { key: string; label: string; value?: string; onPress: () => void }[] = [];
  if (hasAudioOptions && onOpenAudio) items.push({ key: "audio", label: t("player.settings.audio"), onPress: onOpenAudio });
  if (hasSubtitleOptions && onOpenSubtitles) {
    items.push({ key: "subtitles", label: t("player.settings.subtitles"), onPress: onOpenSubtitles });
  }
  if (onOpenSubtitleStyle) {
    items.push({
      key: "subtitle-style",
      label: t("player.settings.subtitleStyle", "Subtitle appearance"),
      onPress: onOpenSubtitleStyle,
    });
  }
  if (onOpenQuality) {
    items.push({ key: "quality", label: t("player.settings.quality"), value: qualityLabel, onPress: onOpenQuality });
  }
  if (onCycleSpeed) {
    items.push({ key: "speed", label: t("player.settings.speed"), value: speedLabel, onPress: onCycleSpeed });
  }
  if (onPreviousEpisode) {
    items.push({ key: "prevEpisode", label: t("player.controls.previousEpisode"), onPress: onPreviousEpisode });
  }
  if (onNextEpisode) {
    items.push({ key: "nextEpisode", label: t("player.controls.nextEpisode"), onPress: onNextEpisode });
  }
  if (onToggleAutoplayNext) {
    items.push({
      key: "autoplayNext",
      label: t("player.settings.autoplayNext"),
      value: autoplayNext ? t("common.enabled") : t("common.disabled"),
      onPress: onToggleAutoplayNext,
    });
  }
  if (hasChapters && onOpenChapters) {
    items.push({
      key: "chapters",
      label: t("player.controls.chapters", "Chapters"),
      value: currentChapterTitle,
      onPress: onOpenChapters,
    });
  }
  if (onOpenVolume) items.push({ key: "volume", label: t("player.controls.volume", "Volume"), onPress: onOpenVolume });
  if (onOpenSleepTimer) {
    items.push({ key: "sleepTimer", label: t("player.settings.sleepTimer", "Sleep timer"), onPress: onOpenSleepTimer });
  }
  if (hasStreamInfo && onOpenStreamInfo) {
    items.push({ key: "streamInfo", label: t("player.streamInfo.title", "Stream Info"), onPress: onOpenStreamInfo });
  }

  // A hidden RN Modal keeps its children mounted, so this menu re-rendered
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
            {t("player.controls.more", "More")}
          </Text>
          <View style={styles.list}>
            {items.map((item, i) => (
              <FocusableCard
                key={item.key}
                hasTVPreferredFocus={i === 0}
                onPress={item.onPress}
                // The value is the half that carries the answer - which
                // quality, which speed, which chapter you are in - and it was
                // painted on screen but never spoken: the label alone said
                // "Quality" and stopped there.
                accessibilityLabel={item.value ? `${item.label}, ${item.value}` : item.label}
                style={[styles.row, { borderBottomColor: theme.colors.border }]}
              >
                <View style={styles.rowInner}>
                  <Text style={[styles.label, { color: theme.colors.text }]}>{item.label}</Text>
                  {item.value ? (
                    <Text style={[styles.value, { color: theme.colors.textMuted }]}>{item.value}</Text>
                  ) : null}
                </View>
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
    paddingVertical: 9,
    paddingHorizontal: 16,
    borderRadius: 8,
  },
  rowInner: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    gap: 16,
  },
  label: {
    fontSize: FONT_SIZE.base,
  },
  value: {
    fontSize: FONT_SIZE.base,
  },
});
