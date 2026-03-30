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
import { useVideoPlayer, VideoView } from "expo-video";
import * as ScreenOrientation from "expo-screen-orientation";
import { useLibraryServiceGetMediaItem } from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { usePlaySession } from "../../../src/hooks/usePlaySession";
import { useProgressReporter } from "../../../src/hooks/useProgressReporter";

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
  const { streamUrl, sessionId, isLoading, isError, stopSession } = usePlaySession(mediaId);

  const [showControls, setShowControls] = useState(true);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [isPlaying, setIsPlaying] = useState(false);
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

  // Lock to landscape on mount
  useEffect(() => {
    ScreenOrientation.lockAsync(ScreenOrientation.OrientationLock.LANDSCAPE);
    return () => {
      ScreenOrientation.lockAsync(ScreenOrientation.OrientationLock.DEFAULT);
    };
  }, []);

  // Stop session on unmount
  useEffect(() => {
    return () => stopSession();
  }, [stopSession]);

  // Auto-hide controls
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

  const getPositionMs = useCallback(() => {
    return (player?.currentTime ?? 0) * 1000;
  }, [player]);

  useProgressReporter(sessionId, getPositionMs);

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

  return (
    <View style={[styles.container, { backgroundColor: "#000" }]}>
      <StatusBar hidden />

      <Pressable onPress={toggleControls} style={styles.playerWrapper}>
        <VideoView
          player={player}
          style={styles.video}
          nativeControls={false}
          contentFit="contain"
        />

        {showControls && (
          <View style={styles.overlay}>
            {/* Top bar: back + title */}
            <View style={styles.topBar}>
              <Pressable onPress={handleBack} hitSlop={16}>
                <Text style={styles.backButton}>{"<"}</Text>
              </Pressable>
              <Text style={styles.title} numberOfLines={1}>
                {media?.title ?? ""}
              </Text>
              <View style={{ width: 40 }} />
            </View>

            {/* Center play/pause */}
            <View style={styles.centerControls}>
              <Pressable onPress={() => player?.seekBy(-10)} hitSlop={16} style={styles.seekButton}>
                <Text style={styles.seekText}>-10</Text>
              </Pressable>
              <Pressable onPress={togglePlayPause} hitSlop={16} style={styles.playButton}>
                <Text style={styles.playButtonText}>{isPlaying ? "||" : "\u25B6"}</Text>
              </Pressable>
              <Pressable onPress={() => player?.seekBy(10)} hitSlop={16} style={styles.seekButton}>
                <Text style={styles.seekText}>+10</Text>
              </Pressable>
            </View>

            {/* Bottom bar: time + seek bar */}
            <View style={styles.bottomBar}>
              <Text style={styles.timeText}>{formatTime(currentTime)}</Text>
              <Pressable
                style={styles.seekBarContainer}
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
              </Pressable>
              <Text style={styles.timeText}>{formatTime(duration)}</Text>
            </View>
          </View>
        )}
      </Pressable>
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
});
