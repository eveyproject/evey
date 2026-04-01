import { useEffect, useState, useCallback, useRef, useMemo } from "react";
import {
  View,
  Text,
  Pressable,
  ActivityIndicator,
  Dimensions,
  StatusBar,
  StyleSheet,
} from "react-native";
import { useLocalSearchParams, router } from "expo-router";
import { useVideoPlayer, VideoView, isPictureInPictureSupported } from "expo-video";
import { Ionicons } from "@expo/vector-icons";
import * as ScreenOrientation from "expo-screen-orientation";
import { useLibraryServiceGetMediaItem } from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { usePlaySession } from "../../../src/hooks/usePlaySession";
import { useProgressReporter } from "../../../src/hooks/useProgressReporter";
import { usePlayerGestures } from "../../../src/hooks/usePlayerGestures";
import { useNextEpisode } from "../../../src/hooks/useNextEpisode";
import { useChapters } from "../../../src/hooks/useChapters";
import { AudioTrackPicker, type AudioStream } from "../../../src/components/AudioTrackPicker";
import { SubtitlePicker, isImageSubtitle, type SubtitleStream } from "../../../src/components/SubtitlePicker";
import { QualityPicker } from "../../../src/components/QualityPicker";
import { ChaptersPicker } from "../../../src/components/ChaptersPicker";
import { SkipIntroOverlay } from "../../../src/components/SkipIntroOverlay";

const IMAGE_SUBTITLE_CODECS = new Set([
  "pgssub", "hdmv_pgs_subtitle", "dvd_subtitle", "dvbsub", "xsub",
]);

const CONTROLS_TIMEOUT = 3000;
const { width: SCREEN_WIDTH, height: SCREEN_HEIGHT } = Dimensions.get("screen");
const PLAYER_WIDTH = Math.max(SCREEN_WIDTH, SCREEN_HEIGHT);
const PLAYER_HEIGHT = Math.min(SCREEN_WIDTH, SCREEN_HEIGHT);

function formatTime(seconds: number): string {
  const s = Math.max(0, Math.floor(seconds));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = s % 60;
  if (h > 0) return `${h}:${String(m).padStart(2, "0")}:${String(sec).padStart(2, "0")}`;
  return `${m}:${String(sec).padStart(2, "0")}`;
}

export default function PlayerScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const theme = useTheme();
  const mediaId = id ?? "";

  const { data: media } = useLibraryServiceGetMediaItem(mediaId);
  const { streamUrl, sessionId, streamInfo, isLoading, isError, stopSession, restartWithTracks } =
    usePlaySession(mediaId);

  const { data: chapters = [] } = useChapters(mediaId);
  const videoViewRef = useRef<VideoView>(null);
  const pipSupported = useMemo(() => isPictureInPictureSupported(), []);

  const [showControls, setShowControls] = useState(true);
  const [showAudioPicker, setShowAudioPicker] = useState(false);
  const [showSubtitlePicker, setShowSubtitlePicker] = useState(false);
  const [showQualityPicker, setShowQualityPicker] = useState(false);
  const [showChaptersPicker, setShowChaptersPicker] = useState(false);
  const [selectedAudioIndex, setSelectedAudioIndex] = useState<number>(0);
  const [selectedSubtitleIndex, setSelectedSubtitleIndex] = useState<number | null>(null);
  const [selectedQuality, setSelectedQuality] = useState(0);
  const [playbackSpeed, setPlaybackSpeed] = useState(1);

  const audioStreams: AudioStream[] = useMemo(
    () =>
      streamInfo?.mediaStreams
        ?.filter((s) => s.type === "MEDIA_STREAM_TYPE_AUDIO")
        .map((s, i) => ({
          index: s.index ?? 0,
          trackIndex: i,
          language: s.language,
          title: s.title,
          isDefault: s.isDefault,
          channels: s.channels,
        })) ?? [],
    [streamInfo],
  );

  const subtitleStreams: SubtitleStream[] = useMemo(
    () =>
      streamInfo?.mediaStreams
        ?.filter((s) => s.type === "MEDIA_STREAM_TYPE_SUBTITLE")
        .map((s, i) => ({
          index: s.index ?? 0,
          trackIndex: i,
          language: s.language,
          title: s.title,
          isDefault: s.isDefault,
          isForced: s.isForced,
          codec: s.codec,
          burnIn: IMAGE_SUBTITLE_CODECS.has(s.codec?.toLowerCase() ?? ""),
        })) ?? [],
    [streamInfo],
  );

  // Set initial selected audio index from default stream
  useEffect(() => {
    const def = audioStreams.find((s) => s.isDefault);
    if (def) setSelectedAudioIndex(def.index);
    else if (audioStreams.length > 0) setSelectedAudioIndex(audioStreams[0].index);
  }, [audioStreams]);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [isPlaying, setIsPlaying] = useState(false);
  const [volume, setVolume] = useState(1);
  const volumeBaseRef = useRef(1);
  const hideTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const videoSource = useMemo(() => {
    if (!streamUrl) return null;
    const isHls = streamUrl.includes("m3u8");
    return {
      uri: streamUrl,
      ...(isHls ? { contentType: "hls" as const } : {}),
    };
  }, [streamUrl]);

  const player = useVideoPlayer(videoSource, (p) => {
    p.timeUpdateEventInterval = 0.5;
    p.play();
  });

  useEffect(() => {
    if (!player) return;
    const timeSub = player.addListener("timeUpdate", (ev) => {
      setCurrentTime(ev.currentTime);
      setDuration(player.duration);
    });
    const statusSub = player.addListener("statusChange", () => {
      setIsPlaying(player.playing);
      setDuration(player.duration);
    });
    const playSub = player.addListener("playingChange", () => {
      setIsPlaying(player.playing);
    });
    return () => {
      timeSub.remove();
      statusSub.remove();
      playSub.remove();
    };
  }, [player]);

  useEffect(() => {
    ScreenOrientation.lockAsync(ScreenOrientation.OrientationLock.LANDSCAPE);
    return () => {
      ScreenOrientation.lockAsync(ScreenOrientation.OrientationLock.DEFAULT);
    };
  }, []);

  useEffect(() => {
    return () => stopSession();
  }, [stopSession]);

  const resetHideTimer = useCallback(() => {
    if (hideTimerRef.current) clearTimeout(hideTimerRef.current);
    setShowControls(true);
    hideTimerRef.current = setTimeout(() => setShowControls(false), CONTROLS_TIMEOUT);
  }, []);

  useEffect(() => {
    resetHideTimer();
    return () => {
      if (hideTimerRef.current) clearTimeout(hideTimerRef.current);
    };
  }, [resetHideTimer]);

  const toggleControls = useCallback(() => {
    if (showControls) {
      if (hideTimerRef.current) clearTimeout(hideTimerRef.current);
      setShowControls(false);
    } else {
      resetHideTimer();
    }
  }, [showControls, resetHideTimer]);

  const togglePlayPause = useCallback(() => {
    if (!player) return;
    if (player.playing) {
      player.pause();
    } else {
      player.play();
    }
    resetHideTimer();
  }, [player, resetHideTimer]);

  const seekTo = useCallback(
    (pct: number) => {
      if (!player || duration <= 0) return;
      player.currentTime = pct * duration;
      resetHideTimer();
    },
    [player, duration, resetHideTimer],
  );

  const seekBy = useCallback(
    (seconds: number) => {
      if (!player) return;
      player.seekBy(seconds);
      resetHideTimer();
    },
    [player, resetHideTimer],
  );

  const handleVolumeChange = useCallback(
    (delta: number) => {
      if (!player) return;
      const newVol = Math.max(0, Math.min(1, volumeBaseRef.current + delta));
      player.volume = newVol;
      setVolume(newVol);
    },
    [player],
  );

  const { panResponder, seekFlash, volumeIndicator, cleanup } = usePlayerGestures({
    onSeekBy: seekBy,
    onTogglePlayPause: togglePlayPause,
    onToggleControls: toggleControls,
    onVolumeChange: handleVolumeChange,
  });

  useEffect(() => cleanup, [cleanup]);

  const handleNavigateEpisode = useCallback(
    (episodeId: string) => {
      stopSession();
      router.replace(`/(auth)/player/${episodeId}`);
    },
    [stopSession],
  );

  const nextEp = useNextEpisode({
    media,
    currentTime,
    duration,
    onNavigate: handleNavigateEpisode,
  });

  const getPositionMs = useCallback(() => {
    return (player?.currentTime ?? 0) * 1000;
  }, [player]);

  useProgressReporter(sessionId, getPositionMs);

  const handleAudioSelect = useCallback(
    (index: number) => {
      setSelectedAudioIndex(index);
      restartWithTracks(index, selectedSubtitleIndex ?? undefined, selectedQuality || undefined);
    },
    [restartWithTracks, selectedSubtitleIndex, selectedQuality],
  );

  const handleSubtitleSelect = useCallback(
    (index: number | null) => {
      setSelectedSubtitleIndex(index);
      if (index === null) {
        restartWithTracks(selectedAudioIndex, undefined, selectedQuality || undefined);
      } else {
        restartWithTracks(selectedAudioIndex, index, selectedQuality || undefined);
      }
    },
    [restartWithTracks, selectedAudioIndex, selectedQuality],
  );

  const handleQualitySelect = useCallback(
    (quality: number) => {
      setSelectedQuality(quality);
      restartWithTracks(
        selectedAudioIndex,
        selectedSubtitleIndex ?? undefined,
        quality || undefined,
      );
    },
    [restartWithTracks, selectedAudioIndex, selectedSubtitleIndex],
  );

  const handleChapterSeek = useCallback(
    (timeSec: number) => {
      if (!player) return;
      player.currentTime = timeSec;
      resetHideTimer();
    },
    [player, resetHideTimer],
  );

  const handleSkipIntro = useCallback(
    (toSec: number) => {
      if (!player) return;
      player.currentTime = toSec;
    },
    [player],
  );

  const handlePip = useCallback(async () => {
    if (!videoViewRef.current) return;
    try {
      await videoViewRef.current.startPictureInPicture();
    } catch { /* PiP unavailable */ }
  }, []);

  const SPEEDS = [0.5, 0.75, 1, 1.25, 1.5, 2] as const;
  const cycleSpeed = useCallback(() => {
    setPlaybackSpeed((prev) => {
      const idx = SPEEDS.indexOf(prev as (typeof SPEEDS)[number]);
      const next = SPEEDS[(idx + 1) % SPEEDS.length];
      if (player) player.playbackRate = next;
      return next;
    });
    resetHideTimer();
  }, [player, resetHideTimer]);

  const handleBack = useCallback(() => {
    stopSession();
    router.back();
  }, [stopSession]);

  if (isError) {
    return (
      <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
        <StatusBar hidden />
        <Text style={{ color: theme.colors.text, fontSize: 16 }}>Failed to load stream</Text>
        <Pressable onPress={handleBack} style={{ marginTop: 16 }}>
          <Text style={{ color: theme.colors.accent, fontSize: 16 }}>Go back</Text>
        </Pressable>
      </View>
    );
  }

  if (isLoading || !streamUrl) {
    return (
      <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
        <StatusBar hidden />
        <ActivityIndicator size="large" color={theme.colors.accent} />
      </View>
    );
  }

  const progress = duration > 0 ? currentTime / duration : 0;
  const volumePct = Math.round(volume * 100);

  return (
    <View style={[styles.container, { backgroundColor: "#000" }]}>
      <StatusBar hidden />

      <View style={styles.playerWrapper} {...panResponder.panHandlers}>
        <VideoView
          ref={videoViewRef}
          player={player}
          style={styles.video}
          nativeControls={false}
          contentFit="contain"
          allowsPictureInPicture={pipSupported}
        />

        {/* Seek flash */}
        {seekFlash && (
          <View
            style={[
              styles.seekFlash,
              seekFlash === "left" ? styles.seekFlashLeft : styles.seekFlashRight,
            ]}
          >
            <Text style={styles.seekFlashText}>
              {seekFlash === "left" ? "\u00AB 10" : "10 \u00BB"}
            </Text>
          </View>
        )}

        {/* Volume indicator */}
        {volumeIndicator !== null && (
          <View style={styles.volumeIndicator}>
            <Text style={styles.volumeIcon}>{"\u266A"}</Text>
            <View style={styles.volumeBarBg}>
              <View style={[styles.volumeBarFill, { height: `${volumePct}%` }]} />
            </View>
            <Text style={styles.volumeText}>{volumePct}</Text>
          </View>
        )}

        {/* Skip intro */}
        <SkipIntroOverlay
          filePath={media?.filePath}
          currentTime={currentTime}
          onSkip={handleSkipIntro}
        />

        {/* Next episode overlay */}
        {nextEp.visible && nextEp.nextEpisode && (
          <View style={styles.nextEpOverlay}>
            <Text style={styles.nextEpLabel}>
              {nextEp.ended
                ? `Next in ${Math.max(nextEp.countdown, 0)}s`
                : "Up Next"}
            </Text>
            <Text style={styles.nextEpTitle} numberOfLines={1}>
              {nextEp.nextEpisode.indexNumber != null
                ? `Ep. ${nextEp.nextEpisode.indexNumber}`
                : "Next Episode"}
              {nextEp.nextEpisode.title ? ` \u2014 ${nextEp.nextEpisode.title}` : ""}
            </Text>
            <View style={styles.nextEpButtons}>
              <Pressable onPress={nextEp.playNow} style={styles.nextEpPlayBtn}>
                <Text style={styles.nextEpPlayText}>Play Now</Text>
              </Pressable>
              <Pressable onPress={nextEp.cancel} style={styles.nextEpCancelBtn}>
                <Text style={styles.nextEpCancelText}>Cancel</Text>
              </Pressable>
            </View>
            {nextEp.ended && (
              <View style={styles.nextEpProgressBg}>
                <View
                  style={[
                    styles.nextEpProgressFill,
                    { width: `${((5 - Math.max(nextEp.countdown, 0)) / 5) * 100}%` },
                  ]}
                />
              </View>
            )}
          </View>
        )}

        {showControls && (
          <View style={styles.overlay} pointerEvents="box-none">
            <View style={styles.topBar}>
              <Pressable onPress={handleBack} hitSlop={16} accessible={true} accessibilityLabel="Go back" accessibilityRole="button">
                <Ionicons name="chevron-back" size={28} color="#fff" />
              </Pressable>
              <Text style={styles.title} numberOfLines={1}>
                {media?.title ?? ""}
              </Text>
              <View style={styles.topBarRight}>
                <Pressable
                  onPress={cycleSpeed}
                  hitSlop={8}
                  style={styles.trackButton}
                  accessible={true}
                  accessibilityLabel="Playback speed"
                  accessibilityRole="button"
                >
                  <Text style={styles.speedButtonText}>{playbackSpeed}x</Text>
                </Pressable>
                <Pressable
                  onPress={() => setShowQualityPicker(true)}
                  hitSlop={8}
                  style={styles.trackButton}
                  accessible={true}
                  accessibilityLabel="Quality"
                  accessibilityRole="button"
                >
                  <Ionicons name="settings-sharp" size={18} color="#fff" />
                </Pressable>
                {chapters.length > 0 && (
                  <Pressable
                    onPress={() => setShowChaptersPicker(true)}
                    hitSlop={8}
                    style={styles.trackButton}
                    accessible={true}
                    accessibilityLabel="Chapters"
                    accessibilityRole="button"
                  >
                    <Ionicons name="book-outline" size={18} color="#fff" />
                  </Pressable>
                )}
                {audioStreams.length > 1 && (
                  <Pressable
                    onPress={() => setShowAudioPicker(true)}
                    hitSlop={8}
                    style={styles.trackButton}
                    accessible={true}
                    accessibilityLabel="Audio tracks"
                    accessibilityRole="button"
                  >
                    <Ionicons name="volume-high" size={18} color="#fff" />
                  </Pressable>
                )}
                {subtitleStreams.length > 0 && (
                  <Pressable
                    onPress={() => setShowSubtitlePicker(true)}
                    hitSlop={8}
                    style={styles.trackButton}
                    accessible={true}
                    accessibilityLabel="Subtitles"
                    accessibilityRole="button"
                  >
                    <Ionicons name="chatbubble-ellipses" size={18} color="#fff" />
                  </Pressable>
                )}
                {pipSupported && (
                  <Pressable
                    onPress={handlePip}
                    hitSlop={8}
                    style={styles.trackButton}
                    accessible={true}
                    accessibilityLabel="Picture in Picture"
                    accessibilityRole="button"
                  >
                    <Ionicons name="albums-outline" size={18} color="#fff" />
                  </Pressable>
                )}
              </View>
            </View>

            <View style={styles.centerControls}>
              <Pressable onPress={() => seekBy(-10)} hitSlop={16} style={styles.seekButton} accessible={true} accessibilityLabel="Seek backward 10 seconds" accessibilityRole="button">
                <Ionicons name="play-back" size={24} color="#fff" />
              </Pressable>
              <Pressable onPress={togglePlayPause} hitSlop={16} style={styles.playButton} accessible={true} accessibilityLabel={isPlaying ? "Pause" : "Play"} accessibilityRole="button">
                <Ionicons name={isPlaying ? "pause" : "play"} size={32} color="#fff" />
              </Pressable>
              <Pressable onPress={() => seekBy(10)} hitSlop={16} style={styles.seekButton} accessible={true} accessibilityLabel="Seek forward 10 seconds" accessibilityRole="button">
                <Ionicons name="play-forward" size={24} color="#fff" />
              </Pressable>
            </View>

            <View style={styles.bottomBar}>
              <Text style={styles.timeText}>{formatTime(currentTime)}</Text>
              <Pressable
                style={styles.seekBarContainer}
                accessible={true}
                accessibilityRole="adjustable"
                accessibilityLabel="Seek slider"
                onPress={(e) => {
                  const x = e.nativeEvent.locationX;
                  const width = PLAYER_WIDTH - 160;
                  if (width > 0) seekTo(Math.max(0, Math.min(1, x / width)));
                }}
              >
                <View style={styles.seekBarTrack}>
                  <View style={[styles.seekBarFill, { flex: progress }]} />
                  <View style={{ flex: 1 - progress }} />
                </View>
                {duration > 0 && chapters.map((ch, i) => {
                  const pct = (ch.startTimeMs / 1000) / duration;
                  if (pct <= 0 || pct >= 1) return null;
                  return (
                    <View
                      key={i}
                      style={[styles.chapterMarker, { left: `${pct * 100}%` }]}
                    />
                  );
                })}
              </Pressable>
              <Text style={styles.timeText}>{formatTime(duration)}</Text>
            </View>
          </View>
        )}
      </View>

      <AudioTrackPicker
        visible={showAudioPicker}
        onClose={() => setShowAudioPicker(false)}
        streams={audioStreams}
        currentIndex={selectedAudioIndex}
        onSelect={handleAudioSelect}
      />

      <SubtitlePicker
        visible={showSubtitlePicker}
        onClose={() => setShowSubtitlePicker(false)}
        streams={subtitleStreams}
        currentIndex={selectedSubtitleIndex}
        onSelect={handleSubtitleSelect}
      />

      <QualityPicker
        visible={showQualityPicker}
        onClose={() => setShowQualityPicker(false)}
        currentQuality={selectedQuality}
        onSelect={handleQualitySelect}
      />

      <ChaptersPicker
        visible={showChaptersPicker}
        onClose={() => setShowChaptersPicker(false)}
        chapters={chapters}
        currentTimeMs={currentTime * 1000}
        onSeek={handleChapterSeek}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    justifyContent: "center",
    alignItems: "center",
  },
  playerWrapper: {
    width: PLAYER_WIDTH,
    height: PLAYER_HEIGHT,
  },
  video: {
    width: "100%",
    height: "100%",
  },
  overlay: {
    ...StyleSheet.absoluteFillObject,
    backgroundColor: "rgba(0,0,0,0.4)",
    justifyContent: "space-between",
  },
  topBar: {
    flexDirection: "row",
    alignItems: "center",
    paddingTop: 16,
    paddingHorizontal: 20,
  },
  backButton: {
    color: "#fff",
    fontSize: 24,
    fontWeight: "bold",
    width: 40,
  },
  title: {
    flex: 1,
    color: "#fff",
    fontSize: 16,
    fontWeight: "600",
    textAlign: "center",
  },
  centerControls: {
    flexDirection: "row",
    justifyContent: "center",
    alignItems: "center",
    gap: 40,
  },
  seekButton: {
    width: 48,
    height: 48,
    borderRadius: 24,
    backgroundColor: "rgba(255,255,255,0.2)",
    justifyContent: "center",
    alignItems: "center",
  },
  seekText: {
    color: "#fff",
    fontSize: 14,
    fontWeight: "600",
  },
  playButton: {
    width: 64,
    height: 64,
    borderRadius: 32,
    backgroundColor: "rgba(255,255,255,0.25)",
    justifyContent: "center",
    alignItems: "center",
  },
  playButtonText: {
    color: "#fff",
    fontSize: 28,
  },
  bottomBar: {
    flexDirection: "row",
    alignItems: "center",
    paddingBottom: 20,
    paddingHorizontal: 20,
    gap: 12,
  },
  timeText: {
    color: "#fff",
    fontSize: 13,
    fontVariant: ["tabular-nums"],
    minWidth: 50,
    textAlign: "center",
  },
  seekBarContainer: {
    flex: 1,
    height: 32,
    justifyContent: "center",
  },
  seekBarTrack: {
    height: 4,
    borderRadius: 2,
    backgroundColor: "rgba(255,255,255,0.3)",
    flexDirection: "row",
    overflow: "hidden",
  },
  seekBarFill: {
    backgroundColor: "#fff",
  },
  chapterMarker: {
    position: "absolute",
    top: 12,
    width: 3,
    height: 8,
    borderRadius: 1,
    backgroundColor: "rgba(255,255,255,0.7)",
    marginLeft: -1,
  },
  seekFlash: {
    position: "absolute",
    top: 0,
    bottom: 0,
    width: "35%",
    justifyContent: "center",
    alignItems: "center",
    backgroundColor: "rgba(255,255,255,0.08)",
  },
  seekFlashLeft: {
    left: 0,
    borderTopRightRadius: 80,
    borderBottomRightRadius: 80,
  },
  seekFlashRight: {
    right: 0,
    borderTopLeftRadius: 80,
    borderBottomLeftRadius: 80,
  },
  seekFlashText: {
    color: "#fff",
    fontSize: 24,
    fontWeight: "700",
  },
  volumeIndicator: {
    position: "absolute",
    right: 40,
    top: "25%",
    height: "50%",
    width: 36,
    alignItems: "center",
    justifyContent: "center",
    backgroundColor: "rgba(0,0,0,0.6)",
    borderRadius: 18,
    paddingVertical: 8,
  },
  volumeIcon: {
    color: "#fff",
    fontSize: 16,
    marginBottom: 6,
  },
  volumeBarBg: {
    flex: 1,
    width: 4,
    backgroundColor: "rgba(255,255,255,0.3)",
    borderRadius: 2,
    overflow: "hidden",
    justifyContent: "flex-end",
  },
  volumeBarFill: {
    width: "100%",
    backgroundColor: "#fff",
    borderRadius: 2,
  },
  volumeText: {
    color: "#fff",
    fontSize: 11,
    marginTop: 4,
    fontWeight: "600",
  },
  nextEpOverlay: {
    position: "absolute",
    bottom: 80,
    right: 16,
    backgroundColor: "rgba(0,0,0,0.85)",
    borderRadius: 12,
    padding: 14,
    maxWidth: 260,
    borderWidth: 1,
    borderColor: "rgba(255,255,255,0.15)",
  },
  nextEpLabel: {
    color: "rgba(255,255,255,0.6)",
    fontSize: 11,
    textTransform: "uppercase",
    letterSpacing: 1,
    marginBottom: 4,
  },
  nextEpTitle: {
    color: "#fff",
    fontSize: 14,
    fontWeight: "600",
    marginBottom: 10,
  },
  nextEpButtons: {
    flexDirection: "row",
    gap: 8,
  },
  nextEpPlayBtn: {
    backgroundColor: "#fff",
    paddingHorizontal: 14,
    paddingVertical: 6,
    borderRadius: 6,
  },
  nextEpPlayText: {
    color: "#000",
    fontSize: 13,
    fontWeight: "600",
  },
  nextEpCancelBtn: {
    backgroundColor: "rgba(255,255,255,0.15)",
    paddingHorizontal: 14,
    paddingVertical: 6,
    borderRadius: 6,
  },
  nextEpCancelText: {
    color: "#fff",
    fontSize: 13,
    fontWeight: "600",
  },
  nextEpProgressBg: {
    marginTop: 8,
    height: 3,
    borderRadius: 2,
    backgroundColor: "rgba(255,255,255,0.2)",
    overflow: "hidden",
  },
  nextEpProgressFill: {
    height: "100%",
    backgroundColor: "#fff",
    borderRadius: 2,
  },
  topBarRight: {
    flexDirection: "row",
    alignItems: "center",
    gap: 8,
    minWidth: 40,
    justifyContent: "flex-end",
  },
  trackButton: {
    width: 36,
    height: 36,
    borderRadius: 18,
    backgroundColor: "rgba(255,255,255,0.15)",
    justifyContent: "center",
    alignItems: "center",
  },
  trackButtonText: {
    color: "#fff",
    fontSize: 14,
    fontWeight: "700",
  },
  speedButtonText: {
    color: "#fff",
    fontSize: 12,
    fontWeight: "700",
  },
});
