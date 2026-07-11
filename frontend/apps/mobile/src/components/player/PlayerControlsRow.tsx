import { View, Text, Pressable, StyleSheet } from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../providers/ThemeProvider";
import { CastButtonNative } from "../../hooks/useChromecast";
import type { SleepTimerState } from "../../hooks/useSleepTimer";
import { formatTime } from "../../lib/formatTime";

export type SheetName = "audio" | "subtitle" | "quality" | "chapters" | "streamInfo" | "sleepTimer" | "syncPlay";

interface PlayerControlsRowProps {
  isPlaying: boolean;
  currentTime: number;
  effectiveDuration: number;
  skipFwd: number;
  skipBwd: number;
  playbackSpeed: number;
  isFullscreen: boolean;
  pipSupported: boolean;
  hasChapters: boolean;
  audioTrackCount: number;
  subtitleTrackCount: number;
  sleepTimerState: SleepTimerState | null;
  syncConnected: boolean;
  castConnected: boolean;
  castAvailable: boolean;
  onSeekBy: (seconds: number) => void;
  onTogglePlayPause: () => void;
  onCycleSpeed: () => void;
  onPip: () => void;
  onToggleFullscreen: () => void;
  onCast: () => void;
  onSyncPress: () => void;
  onOpenSheet: (sheet: SheetName) => void;
}

export function PlayerControlsRow({
  isPlaying,
  currentTime,
  effectiveDuration,
  skipFwd,
  skipBwd,
  playbackSpeed,
  isFullscreen,
  pipSupported,
  hasChapters,
  audioTrackCount,
  subtitleTrackCount,
  sleepTimerState,
  syncConnected,
  castConnected,
  castAvailable,
  onSeekBy,
  onTogglePlayPause,
  onCycleSpeed,
  onPip,
  onToggleFullscreen,
  onCast,
  onSyncPress,
  onOpenSheet,
}: PlayerControlsRowProps) {
  const theme = useTheme();
  const { t } = useTranslation();
  return (
    <View style={[styles.controlsRow, { marginTop: 4 }]}>
      <Pressable onPress={() => onSeekBy(-skipBwd)} hitSlop={12} style={styles.controlBtn} accessible={true} accessibilityLabel={t("player.controls.rewind", { seconds: skipBwd })} accessibilityRole="button">
        <Ionicons name="play-back" size={20} color="#fff" />
      </Pressable>
      <Pressable onPress={onTogglePlayPause} hitSlop={12} style={styles.controlBtn} accessible={true} accessibilityLabel={isPlaying ? t("player.controls.pause") : t("player.controls.play")} accessibilityRole="button">
        <Ionicons name={isPlaying ? "pause" : "play"} size={24} color="#fff" />
      </Pressable>
      <Pressable onPress={() => onSeekBy(skipFwd)} hitSlop={12} style={styles.controlBtn} accessible={true} accessibilityLabel={t("player.controls.forward", { seconds: skipFwd })} accessibilityRole="button">
        <Ionicons name="play-forward" size={20} color="#fff" />
      </Pressable>
      <Text style={styles.timeText}>{formatTime(currentTime)} / {formatTime(effectiveDuration)}</Text>
      <View style={{ flex: 1 }} />
      {hasChapters && (
        <Pressable
          onPress={() => onOpenSheet("chapters")}
          hitSlop={8}
          style={styles.trackButton}
          accessible={true}
          accessibilityLabel={t("player.controls.chapters")}
          accessibilityRole="button"
        >
          <Ionicons name="book-outline" size={18} color="#fff" />
        </Pressable>
      )}
      {audioTrackCount > 1 && (
        <Pressable
          onPress={() => onOpenSheet("audio")}
          hitSlop={8}
          style={styles.trackButton}
          accessible={true}
          accessibilityLabel={t("player.settings.audio")}
          accessibilityRole="button"
        >
          <Ionicons name="volume-high" size={18} color="#fff" />
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
          <Ionicons name="chatbubble-ellipses" size={18} color="#fff" />
        </Pressable>
      )}
      <Pressable
        onPress={() => onOpenSheet("streamInfo")}
        hitSlop={8}
        style={styles.trackButton}
        accessible={true}
        accessibilityLabel={t("player.streamInfo.title", "Stream Info")}
        accessibilityRole="button"
      >
        <Ionicons name="information-circle-outline" size={18} color="#fff" />
      </Pressable>
      <Pressable
        onPress={onCycleSpeed}
        hitSlop={8}
        style={styles.trackButton}
        accessible={true}
        accessibilityLabel={t("player.settings.speed")}
        accessibilityRole="button"
      >
        <Text style={styles.speedButtonText}>{playbackSpeed}x</Text>
      </Pressable>
      <Pressable
        onPress={() => onOpenSheet("sleepTimer")}
        hitSlop={8}
        style={styles.trackButton}
        accessible={true}
        accessibilityLabel={t("player.settings.sleepTimer", "Sleep timer")}
        accessibilityRole="button"
      >
        <Ionicons
          name={sleepTimerState ? "moon" : "moon-outline"}
          size={18}
          color={sleepTimerState ? theme.colors.accent : "#fff"}
        />
        {sleepTimerState?.kind === "minutes" && sleepTimerState.minutesLeft !== null && (
          <Text style={[styles.speedButtonText, { color: theme.colors.accent, marginLeft: 4 }]}>
            {sleepTimerState.minutesLeft}
          </Text>
        )}
      </Pressable>
      <Pressable
        onPress={() => onOpenSheet("quality")}
        hitSlop={8}
        style={styles.trackButton}
        accessible={true}
        accessibilityLabel={t("player.settings.quality")}
        accessibilityRole="button"
      >
        <Ionicons name="settings-sharp" size={18} color="#fff" />
      </Pressable>
      {pipSupported && (
        <Pressable
          onPress={onPip}
          hitSlop={8}
          style={styles.trackButton}
          accessible={true}
          accessibilityLabel={t("player.controls.pip")}
          accessibilityRole="button"
        >
          <Ionicons name="albums-outline" size={18} color="#fff" />
        </Pressable>
      )}
      <Pressable
        onPress={onSyncPress}
        hitSlop={8}
        style={styles.trackButton}
        accessible={true}
        accessibilityLabel={syncConnected ? t("media.syncplay.leave", "Leave") : t("media.syncplay.title", "Watch Together")}
        accessibilityRole="button"
      >
        <Ionicons name="people" size={18} color={syncConnected ? theme.colors.accent : "#fff"} />
      </Pressable>
      {castAvailable && (
        <>
          {CastButtonNative && <CastButtonNative style={{ width: 0, height: 0, opacity: 0 }} />}
          <Pressable
            onPress={onCast}
            hitSlop={8}
            style={styles.trackButton}
            accessible={true}
            accessibilityLabel={castConnected ? t("player.controls.stopCast") : t("player.controls.cast")}
            accessibilityRole="button"
          >
            <Ionicons name="tv-outline" size={18} color={castConnected ? theme.colors.accent : "#fff"} />
          </Pressable>
        </>
      )}
      <Pressable
        onPress={onToggleFullscreen}
        hitSlop={8}
        style={styles.trackButton}
        accessible={true}
        accessibilityLabel={isFullscreen ? t("player.controls.exitFullscreen", "Exit fullscreen") : t("player.controls.fullscreen", "Fullscreen")}
        accessibilityRole="button"
      >
        <Ionicons name={isFullscreen ? "contract-outline" : "expand-outline"} size={18} color="#fff" />
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
    fontSize: 13,
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
  speedButtonText: {
    color: "#fff",
    fontSize: 12,
    fontWeight: "700",
  },
});
