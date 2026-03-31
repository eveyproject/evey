import { useEffect, useState, useCallback, useRef, useMemo } from "react";
import { View, Text, Pressable, ActivityIndicator, Dimensions, StyleSheet, Platform } from "react-native";
import { useLocalSearchParams, router } from "expo-router";
import { useVideoPlayer, VideoView } from "expo-video";
import { useLibraryServiceGetMediaItem } from "@evey/api";

// react-native-tvos replaces react-native at runtime and exports useTVEventHandler
// eslint-disable-next-line @typescript-eslint/no-var-requires
const { useTVEventHandler } = require("react-native") as {
  useTVEventHandler: (handler: (evt: { eventType: string }) => void) => void;
};
import { useTheme } from "../../../src/providers/ThemeProvider";
import { usePlaySession } from "../../../src/hooks/usePlaySession";
import { useProgressReporter } from "../../../src/hooks/useProgressReporter";

const CONTROLS_TIMEOUT = 4000;
const SEEK_STEP = 10;
const { width: SCREEN_WIDTH, height: SCREEN_HEIGHT } = Dimensions.get("screen");

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
  const { streamUrl, sessionId, streamInfo, isLoading, isError, stopSession } =
    usePlaySession(mediaId);

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

  const togglePlayPause = useCallback(() => {
    if (!player) return;
    if (player.playing) player.pause();
    else player.play();
    resetHideTimer();
  }, [player, resetHideTimer]);

  const seekBy = useCallback(
    (seconds: number) => {
      if (!player) return;
      player.seekBy(seconds);
      resetHideTimer();
    },
    [player, resetHideTimer],
  );

  const handleBack = useCallback(() => {
    stopSession();
    router.back();
  }, [stopSession]);

  // D-pad remote handler
  useTVEventHandler((evt) => {
    if (!evt) return;
    const type = evt.eventType;

    if (type === "select" || type === "playPause") {
      togglePlayPause();
    } else if (type === "left") {
      seekBy(-SEEK_STEP);
    } else if (type === "right") {
      seekBy(SEEK_STEP);
    } else if (type === "up" || type === "down") {
      resetHideTimer();
    } else if (type === "menu" || type === "back") {
      if (showControls) handleBack();
      else resetHideTimer();
    }
  });

  const getPositionMs = useCallback(() => {
    return (player?.currentTime ?? 0) * 1000;
  }, [player]);

  useProgressReporter(sessionId, getPositionMs);

  if (isError) {
    return (
      <View style={[styles.container, { backgroundColor: "#000" }]}>
        <Text style={{ color: "#fff", fontSize: 24 }}>Failed to load stream</Text>
        <Pressable onPress={handleBack} style={{ marginTop: 24 }}>
          <Text style={{ color: theme.colors.accent, fontSize: 22 }}>Go back</Text>
        </Pressable>
      </View>
    );
  }

  if (isLoading || !streamUrl) {
    return (
      <View style={[styles.container, { backgroundColor: "#000" }]}>
        <ActivityIndicator size="large" color={theme.colors.accent} />
      </View>
    );
  }

  const progress = duration > 0 ? currentTime / duration : 0;

  return (
    <View style={[styles.container, { backgroundColor: "#000" }]}>
      <VideoView
        player={player}
        style={styles.video}
        nativeControls={false}
        contentFit="contain"
      />

      {showControls && (
        <View style={styles.overlay} pointerEvents="box-none">
          {/* Top bar */}
          <View style={styles.topBar}>
            <Text style={styles.mediaTitle} numberOfLines={1}>
              {media?.title ?? ""}
            </Text>
          </View>

          {/* Center play/pause indicator */}
          <View style={styles.centerControls}>
            <View style={styles.seekHint}>
              <Text style={styles.seekHintText}>-{SEEK_STEP}s</Text>
            </View>
            <View style={styles.playIndicator}>
              <Text style={styles.playIcon}>{isPlaying ? "||" : "\u25B6"}</Text>
            </View>
            <View style={styles.seekHint}>
              <Text style={styles.seekHintText}>+{SEEK_STEP}s</Text>
            </View>
          </View>

          {/* Bottom bar */}
          <View style={styles.bottomBar}>
            <Text style={styles.timeText}>{formatTime(currentTime)}</Text>
            <View style={styles.seekBarTrack}>
              <View style={[styles.seekBarFill, { flex: progress }]} />
              <View style={{ flex: 1 - progress }} />
            </View>
            <Text style={styles.timeText}>{formatTime(duration)}</Text>
          </View>
        </View>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    justifyContent: "center",
    alignItems: "center",
  },
  video: {
    width: SCREEN_WIDTH,
    height: SCREEN_HEIGHT,
  },
  overlay: {
    ...StyleSheet.absoluteFillObject,
    backgroundColor: "rgba(0,0,0,0.4)",
    justifyContent: "space-between",
  },
  topBar: {
    paddingTop: 24,
    paddingHorizontal: 40,
  },
  mediaTitle: {
    color: "#fff",
    fontSize: 24,
    fontWeight: "600",
  },
  centerControls: {
    flexDirection: "row",
    justifyContent: "center",
    alignItems: "center",
    gap: 80,
  },
  seekHint: {
    width: 64,
    height: 64,
    borderRadius: 32,
    backgroundColor: "rgba(255,255,255,0.15)",
    justifyContent: "center",
    alignItems: "center",
  },
  seekHintText: {
    color: "#fff",
    fontSize: 16,
    fontWeight: "600",
  },
  playIndicator: {
    width: 80,
    height: 80,
    borderRadius: 40,
    backgroundColor: "rgba(255,255,255,0.25)",
    justifyContent: "center",
    alignItems: "center",
  },
  playIcon: {
    color: "#fff",
    fontSize: 28,
    fontWeight: "bold",
  },
  bottomBar: {
    flexDirection: "row",
    alignItems: "center",
    paddingBottom: 32,
    paddingHorizontal: 40,
    gap: 16,
  },
  timeText: {
    color: "#fff",
    fontSize: 18,
    fontVariant: ["tabular-nums"],
    minWidth: 70,
    textAlign: "center",
  },
  seekBarTrack: {
    flex: 1,
    height: 6,
    borderRadius: 3,
    backgroundColor: "rgba(255,255,255,0.3)",
    flexDirection: "row",
    overflow: "hidden",
  },
  seekBarFill: {
    backgroundColor: "#fff",
  },
});
