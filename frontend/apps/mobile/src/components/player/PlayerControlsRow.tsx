import { View, Text, Pressable, StyleSheet } from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { useTranslation } from "react-i18next";
import { formatTime } from "../../lib/formatTime";
import { FONT_SIZE, ICON_SIZE } from "@evey/theme";

// "more" opens PlayerMoreSheet (chapters/quality/speed/sleep timer/stream
// info/PiP - see the 2026-07-25 declutter pass). "audio"/"subtitle"/"syncPlay"
// stay direct entry points; cast has no sheet, it lives in the top-right cluster.
export type SheetName = "audio" | "subtitle" | "quality" | "chapters" | "streamInfo" | "sleepTimer" | "syncPlay" | "more";

interface PlayerControlsRowProps {
  isPlaying: boolean;
  currentTime: number;
  effectiveDuration: number;
  skipFwd: number;
  skipBwd: number;
  isFullscreen: boolean;
  audioTrackCount: number;
  subtitleTrackCount: number;
  onSeekBy: (seconds: number) => void;
  onTogglePlayPause: () => void;
  onToggleFullscreen: () => void;
  onOpenSheet: (sheet: SheetName) => void;
}

// Bottom transport bar: play/pause, seek +/-, the timeline (rendered above by
// PlayerSeekBar), audio/subtitle tracks, fullscreen, and a single "more" button
// for everything secondary. Cast and "watch together" moved to the top-right
// cluster; chapters/quality/speed/sleep timer/stream info/PiP moved into the
// "more" sheet - see PlayerMoreSheet. This trims the row from ~13 buttons to 6.
export function PlayerControlsRow({
  isPlaying,
  currentTime,
  effectiveDuration,
  skipFwd,
  skipBwd,
  isFullscreen,
  audioTrackCount,
  subtitleTrackCount,
  onSeekBy,
  onTogglePlayPause,
  onToggleFullscreen,
  onOpenSheet,
}: PlayerControlsRowProps) {
  const { t } = useTranslation();
  return (
    <View style={[styles.controlsRow, { marginTop: 4 }]}>
      <Pressable onPress={() => onSeekBy(-skipBwd)} hitSlop={12} style={styles.controlBtn} accessible={true} accessibilityLabel={t("player.controls.rewind", { seconds: skipBwd })} accessibilityRole="button">
        <Ionicons name="play-back" size={ICON_SIZE.lg} color="#fff" />
      </Pressable>
      <Pressable onPress={onTogglePlayPause} hitSlop={12} style={styles.controlBtn} accessible={true} accessibilityLabel={isPlaying ? t("player.controls.pause") : t("player.controls.play")} accessibilityRole="button">
        <Ionicons name={isPlaying ? "pause" : "play"} size={ICON_SIZE.xl} color="#fff" />
      </Pressable>
      <Pressable onPress={() => onSeekBy(skipFwd)} hitSlop={12} style={styles.controlBtn} accessible={true} accessibilityLabel={t("player.controls.forward", { seconds: skipFwd })} accessibilityRole="button">
        <Ionicons name="play-forward" size={ICON_SIZE.lg} color="#fff" />
      </Pressable>
      <Text style={styles.timeText}>{formatTime(currentTime)} / {formatTime(effectiveDuration)}</Text>
      <View style={{ flex: 1 }} />
      {audioTrackCount > 1 && (
        <Pressable
          onPress={() => onOpenSheet("audio")}
          hitSlop={8}
          style={styles.trackButton}
          accessible={true}
          accessibilityLabel={t("player.settings.audio")}
          accessibilityRole="button"
        >
          <Ionicons name="volume-high" size={ICON_SIZE.lg} color="#fff" />
        </Pressable>
      )}
      {subtitleTrackCount > 0 && (
        <Pressable
          onPress={() => onOpenSheet("subtitle")}
          hitSlop={8}
          style={styles.trackButton}
          accessible={true}
          accessibilityLabel={t("player.settings.subtitles")}
          accessibilityRole="button"
        >
          <Ionicons name="chatbubble-ellipses" size={ICON_SIZE.lg} color="#fff" />
        </Pressable>
      )}
      <Pressable
        onPress={() => onOpenSheet("more")}
        hitSlop={8}
        style={styles.trackButton}
        accessible={true}
        accessibilityLabel={t("player.settings.more", "More options")}
        accessibilityRole="button"
      >
        <Ionicons name="ellipsis-horizontal" size={ICON_SIZE.lg} color="#fff" />
      </Pressable>
      <Pressable
        onPress={onToggleFullscreen}
        hitSlop={8}
        style={styles.trackButton}
        accessible={true}
        accessibilityLabel={isFullscreen ? t("player.controls.exitFullscreen", "Exit fullscreen") : t("player.controls.fullscreen", "Fullscreen")}
        accessibilityRole="button"
      >
        <Ionicons name={isFullscreen ? "contract-outline" : "expand-outline"} size={ICON_SIZE.lg} color="#fff" />
      </Pressable>
    </View>
  );
}

const styles = StyleSheet.create({
  controlsRow: {
    flexDirection: "row",
    alignItems: "center",
    flexWrap: "wrap",
    rowGap: 8,
    columnGap: 12,
  },
  controlBtn: {
    padding: 4,
  },
  timeText: {
    color: "#fff",
    fontSize: FONT_SIZE.xs,
    fontVariant: ["tabular-nums"],
    minWidth: 50,
    textAlign: "center",
  },
  trackButton: {
    width: 36,
    height: 36,
    borderRadius: 18,
    backgroundColor: "rgba(255,255,255,0.15)",
    justifyContent: "center",
    alignItems: "center",
  },
});
