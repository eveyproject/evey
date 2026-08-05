import { useState, useEffect } from "react";
import { View, Text, Pressable, ActivityIndicator, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import MaterialIcons from "@expo/vector-icons/MaterialIcons";
import Svg, { Path, Text as SvgText } from "react-native-svg";
import { useTranslation } from "react-i18next";
import { formatTime } from "@evey/api";
import { FONT_SIZE, ICON_SIZE } from "@evey/theme";
import { PlayerVolumeControl } from "./PlayerVolumeControl";

export interface EpisodeNeighbor {
  id: string;
  title?: string;
  indexNumber?: number;
}

interface PlayerControlsRowProps {
  isPlaying: boolean;
  isBuffering: boolean;
  currentTime: number;
  effectiveDuration: number;
  skipFwd: number;
  skipBwd: number;
  isFullscreen: boolean;
  volume: number;
  muted: boolean;
  prevEpisode: EpisodeNeighbor | null;
  nextEpisode: EpisodeNeighbor | null;
  hasPrevChapter: boolean;
  hasNextChapter: boolean;
  hasChapters: boolean;
  settingsOpen: boolean;
  sleepTimerActive: boolean;
  pipSupported: boolean;
  onSeekBy: (seconds: number) => void;
  onTogglePlayPause: () => void;
  onToggleMute: () => void;
  onVolumeChange: (value: number) => void;
  onEpisodeChange: (id: string) => void;
  onPrevChapter: () => void;
  onNextChapter: () => void;
  onOpenChapters: () => void;
  onToggleSettings: () => void;
  onOpenStreamInfo: () => void;
  onOpenSleepTimer: () => void;
  onPip: () => void;
  onToggleFullscreen: () => void;
}

// Same glyphs as the web player: circular arrow with the number of seconds
// embedded (the web draws them as inline SVG, we reuse the paths).
function SkipBackIcon({ seconds }: { seconds: number }) {
  return (
    <Svg width={22} height={22} viewBox="0 0 24 24">
      <Path fill="#fff" d="M11.99 5V1l-5 5 5 5V7c3.31 0 6 2.69 6 6s-2.69 6-6 6-6-2.69-6-6h-2c0 4.42 3.58 8 8 8s8-3.58 8-8-3.58-8-8-8z" />
      <SvgText x="12" y="15.5" textAnchor="middle" fontSize="7" fontWeight="bold" fill="#fff">{seconds}</SvgText>
    </Svg>
  );
}

function SkipForwardIcon({ seconds }: { seconds: number }) {
  return (
    <Svg width={22} height={22} viewBox="0 0 24 24">
      <Path fill="#fff" d="M12.01 5V1l5 5-5 5V7c-3.31 0-6 2.69-6 6s2.69 6 6 6 6-2.69 6-6h2c0 4.42-3.58 8-8 8s-8-3.58-8-8 3.58-8 8-8z" />
      <SvgText x="12" y="15.5" textAnchor="middle" fontSize="7" fontWeight="bold" fill="#fff">{seconds}</SvgText>
    </Svg>
  );
}

// Transport row, copy of the web player's bar: previous episode/chapter,
// -Ns, play/pause, +Ns, next chapter/episode, time, volume, then a "..."
// button that reveals the floating secondary row (chapters, settings,
// stream info, timer, PiP, fullscreen) - exactly the same grouping as
// the web under the sm breakpoint.
export function PlayerControlsRow({
  isPlaying,
  isBuffering,
  currentTime,
  effectiveDuration,
  skipFwd,
  skipBwd,
  isFullscreen,
  volume,
  muted,
  prevEpisode,
  nextEpisode,
  hasPrevChapter,
  hasNextChapter,
  hasChapters,
  settingsOpen,
  sleepTimerActive,
  pipSupported,
  onSeekBy,
  onTogglePlayPause,
  onToggleMute,
  onVolumeChange,
  onEpisodeChange,
  onPrevChapter,
  onNextChapter,
  onOpenChapters,
  onToggleSettings,
  onOpenStreamInfo,
  onOpenSleepTimer,
  onPip,
  onToggleFullscreen,
}: PlayerControlsRowProps) {
  const { t } = useTranslation();
  const [overflowOpen, setOverflowOpen] = useState(false);

  // Like the web: the settings panel lives in the secondary row, so it
  // stays open as long as the panel is; otherwise it closes when the
  // controls hide (the parent unmounts this row).
  useEffect(() => {
    if (!settingsOpen) return;
    setOverflowOpen(true);
  }, [settingsOpen]);

  return (
    <View style={styles.controlsRow}>
      {prevEpisode && (
        <Pressable
          onPress={() => onEpisodeChange(prevEpisode.id)}
          hitSlop={8}
          style={styles.controlBtn}
          accessible={true}
          accessibilityLabel={t("player.controls.previousEpisode")}
          accessibilityRole="button"
        >
          <Ionicons name="play-skip-back" size={ICON_SIZE.lg} color="#fff" />
        </Pressable>
      )}
      {hasPrevChapter && (
        <Pressable
          onPress={onPrevChapter}
          hitSlop={8}
          style={styles.controlBtn}
          accessible={true}
          accessibilityLabel={t("player.controls.chapters")}
          accessibilityRole="button"
        >
          <MaterialIcons name="first-page" size={ICON_SIZE.xl} color="#fff" />
        </Pressable>
      )}
      <Pressable
        onPress={() => onSeekBy(-skipBwd)}
        hitSlop={10}
        style={styles.controlBtn}
        accessible={true}
        accessibilityLabel={t("player.controls.rewind", { seconds: skipBwd })}
        accessibilityRole="button"
      >
        <SkipBackIcon seconds={skipBwd} />
      </Pressable>
      <Pressable
        onPress={onTogglePlayPause}
        hitSlop={10}
        style={styles.controlBtn}
        accessible={true}
        accessibilityLabel={isPlaying ? t("player.controls.pause") : t("player.controls.play")}
        accessibilityRole="button"
      >
        <Ionicons name={isPlaying ? "pause" : "play"} size={ICON_SIZE["2xl"]} color="#fff" />
      </Pressable>
      <Pressable
        onPress={() => onSeekBy(skipFwd)}
        hitSlop={10}
        style={styles.controlBtn}
        accessible={true}
        accessibilityLabel={t("player.controls.forward", { seconds: skipFwd })}
        accessibilityRole="button"
      >
        <SkipForwardIcon seconds={skipFwd} />
      </Pressable>
      {hasNextChapter && (
        <Pressable
          onPress={onNextChapter}
          hitSlop={8}
          style={styles.controlBtn}
          accessible={true}
          accessibilityLabel={t("player.controls.chapters")}
          accessibilityRole="button"
        >
          <MaterialIcons name="last-page" size={ICON_SIZE.xl} color="#fff" />
        </Pressable>
      )}
      {nextEpisode && (
        <Pressable
          onPress={() => onEpisodeChange(nextEpisode.id)}
          hitSlop={8}
          style={styles.controlBtn}
          accessible={true}
          accessibilityLabel={t("player.controls.nextEpisode")}
          accessibilityRole="button"
        >
          <Ionicons name="play-skip-forward" size={ICON_SIZE.lg} color="#fff" />
        </Pressable>
      )}

      <Text style={styles.timeText}>{formatTime(currentTime)} / {formatTime(effectiveDuration)}</Text>

      <PlayerVolumeControl
        volume={volume}
        muted={muted}
        onToggleMute={onToggleMute}
        onVolumeChange={onVolumeChange}
      />

      {isBuffering && isPlaying && (
        <ActivityIndicator size="small" color="rgba(255,255,255,0.7)" />
      )}

      <View style={{ flex: 1 }} />

      <Pressable
        onPress={() => setOverflowOpen((v) => !v)}
        hitSlop={8}
        style={styles.controlBtn}
        accessible={true}
        accessibilityLabel={t("player.controls.more")}
        accessibilityRole="button"
      >
        <Ionicons name="ellipsis-horizontal" size={ICON_SIZE.lg} color="#fff" />
      </Pressable>

      {overflowOpen && (
        <View style={styles.overflowRow}>
          {hasChapters && (
            <Pressable
              onPress={onOpenChapters}
              hitSlop={8}
              style={styles.controlBtn}
              accessible={true}
              accessibilityLabel={t("player.controls.chapters")}
              accessibilityRole="button"
            >
              <Ionicons name="book-outline" size={ICON_SIZE.lg} color="#fff" />
            </Pressable>
          )}
          <Pressable
            onPress={onToggleSettings}
            hitSlop={8}
            style={styles.controlBtn}
            accessible={true}
            accessibilityLabel={t("player.controls.settings")}
            accessibilityRole="button"
          >
            <Ionicons name="settings-sharp" size={ICON_SIZE.lg} color="#fff" />
          </Pressable>
          <Pressable
            onPress={onOpenStreamInfo}
            hitSlop={8}
            style={styles.controlBtn}
            accessible={true}
            accessibilityLabel={t("player.streamInfo.title", "Stream Info")}
            accessibilityRole="button"
          >
            <Ionicons name="information-circle-outline" size={ICON_SIZE.lg} color="#fff" />
          </Pressable>
          <Pressable
            onPress={onOpenSleepTimer}
            hitSlop={8}
            style={styles.controlBtn}
            accessible={true}
            accessibilityLabel={t("player.settings.sleepTimer", "Sleep timer")}
            accessibilityRole="button"
          >
            <Ionicons name={sleepTimerActive ? "moon" : "moon-outline"} size={ICON_SIZE.lg} color="#fff" />
          </Pressable>
          {pipSupported && (
            <Pressable
              onPress={onPip}
              hitSlop={8}
              style={styles.controlBtn}
              accessible={true}
              accessibilityLabel={t("player.controls.pip")}
              accessibilityRole="button"
            >
              <Ionicons name="albums-outline" size={ICON_SIZE.lg} color="#fff" />
            </Pressable>
          )}
          <Pressable
            onPress={onToggleFullscreen}
            hitSlop={8}
            style={styles.controlBtn}
            accessible={true}
            accessibilityLabel={isFullscreen ? t("player.controls.exitFullscreen") : t("player.controls.fullscreen")}
            accessibilityRole="button"
          >
            <Ionicons name={isFullscreen ? "contract-outline" : "expand-outline"} size={ICON_SIZE.lg} color="#fff" />
          </Pressable>
        </View>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  controlsRow: {
    flexDirection: "row",
    alignItems: "center",
    columnGap: 10,
    marginTop: 4,
  },
  controlBtn: {
    padding: 4,
  },
  timeText: {
    color: "#fff",
    fontSize: FONT_SIZE.xs,
    fontVariant: ["tabular-nums"],
    textAlign: "center",
  },
  // Floating secondary row above the bar, right-aligned - same placement
  // as the web's "overflow" row under sm.
  overflowRow: {
    position: "absolute",
    bottom: "100%",
    right: 0,
    marginBottom: 8,
    flexDirection: "row",
    alignItems: "center",
    columnGap: 10,
    paddingHorizontal: 12,
    paddingVertical: 8,
    borderRadius: 8,
    backgroundColor: "rgba(0,0,0,0.85)",
  },
});
