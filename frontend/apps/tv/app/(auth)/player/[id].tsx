import { useEffect, useState, useCallback, useRef, useMemo } from "react";
import { View, Text, Pressable, ActivityIndicator, Dimensions, StyleSheet } from "react-native";
import { useLocalSearchParams, router } from "expo-router";
import { useVideoPlayer, VideoView } from "expo-video";
import { useLibraryServiceGetMediaItem, usePlaybackServiceMarkAsWatched, usePlaybackServiceGetWatchProgress } from "@evey/api";
import { useAuthStore } from "@evey/store";

// react-native-tvos replaces react-native at runtime and exports useTVEventHandler
// eslint-disable-next-line @typescript-eslint/no-var-requires
const { useTVEventHandler } = require("react-native") as {
  useTVEventHandler: (handler: (evt: { eventType: string }) => void) => void;
};
import { useTheme } from "../../../src/providers/ThemeProvider";
import { usePlaySession } from "../../../src/hooks/usePlaySession";
import { useProgressReporter } from "../../../src/hooks/useProgressReporter";
import { QualityPicker } from "../../../src/components/QualityPicker";

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
  const { streamUrl, sessionId, streamInfo, isLoading, isError, stopSession, restartWithTracks, restartAtPosition, transcodeOffset } =
    usePlaySession(mediaId);

  const [showControls, setShowControls] = useState(true);
  const [showQualityPicker, setShowQualityPicker] = useState(false);
  const [selectedQuality, setSelectedQuality] = useState(0);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [isPlaying, setIsPlaying] = useState(false);
  const [playbackSpeed, setPlaybackSpeed] = useState(1);
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
      setCurrentTime(ev.currentTime + transcodeOffset);
      setDuration(player.duration + transcodeOffset);
    });
    const statusSub = player.addListener("statusChange", () => {
      setIsPlaying(player.playing);
      setDuration(player.duration + transcodeOffset);
    });
    const playSub = player.addListener("playingChange", () => {
      setIsPlaying(player.playing);
    });
    return () => {
      timeSub.remove();
      statusSub.remove();
      playSub.remove();
    };
  }, [player, transcodeOffset]);

  useEffect(() => {
    return () => stopSession();
  }, [stopSession]);

  // Mirror Jellyfin's MaxResumePct=90: trigger MarkAsWatched once
  // playback crosses 90%. The server's ReportProgressBySession path
  // hardcodes Completed=false so without this fix the user has to
  // manually toggle "watched" after every episode.
  const profileId = useAuthStore((s) => s.profileId) ?? "";
  const { data: serverProgress } = usePlaybackServiceGetWatchProgress(
    mediaId,
    { profileId },
    { query: { enabled: !!mediaId && !!profileId } } as never,
  );
  const markAsWatchedMut = usePlaybackServiceMarkAsWatched();
  const autoMarkedRef = useRef(false);
  useEffect(() => { autoMarkedRef.current = false; }, [mediaId]);
  useEffect(() => {
    if (autoMarkedRef.current) return;
    if (serverProgress?.played) { autoMarkedRef.current = true; return; }
    if (!profileId || !mediaId) return;
    if (duration <= 0 || currentTime <= 0) return;
    if (currentTime / duration >= 0.9) {
      autoMarkedRef.current = true;
      markAsWatchedMut.mutate({ mediaItemId: mediaId, params: { profileId } });
    }
  }, [currentTime, duration, serverProgress?.played, profileId, mediaId, markAsWatchedMut]);

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

  const isTranscode = streamInfo?.playMethod === "PLAY_METHOD_TRANSCODE";

  const seekBy = useCallback(
    (seconds: number) => {
      if (!player) return;
      if (isTranscode) {
        const newPos = transcodeOffset + (player.currentTime ?? 0) + seconds;
        restartAtPosition(Math.max(0, newPos));
      } else {
        player.seekBy(seconds);
      }
      resetHideTimer();
    },
    [player, resetHideTimer, isTranscode, transcodeOffset, restartAtPosition],
  );

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

  const handleQualitySelect = useCallback(
    (quality: number) => {
      setSelectedQuality(quality);
      restartWithTracks(undefined, undefined, quality || undefined);
    },
    [restartWithTracks],
  );

  const handleBack = useCallback(() => {
    stopSession();
    router.back();
  }, [stopSession]);

  // D-pad remote handler
  useTVEventHandler((evt) => {
    if (!evt) return;
    const type = evt.eventType;

    if (showQualityPicker) return;
    if (type === "select" || type === "playPause") {
      togglePlayPause();
    } else if (type === "left") {
      seekBy(-SEEK_STEP);
    } else if (type === "right") {
      seekBy(SEEK_STEP);
    } else if (type === "up") {
      cycleSpeed();
    } else if (type === "down") {
      if (showControls) setShowQualityPicker(true);
      else resetHideTimer();
    } else if (type === "menu" || type === "back") {
      if (showControls) handleBack();
      else resetHideTimer();
    }
  });

  const getPositionMs = useCallback(() => {
    return (transcodeOffset + (player?.currentTime ?? 0)) * 1000;
  }, [player, transcodeOffset]);

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

      <QualityPicker
        visible={showQualityPicker}
        currentQuality={selectedQuality}
        onSelect={handleQualitySelect}
        onClose={() => setShowQualityPicker(false)}
      />

      {showControls && (
        <View style={styles.overlay} pointerEvents="box-none">
          {/* Top bar */}
          <View style={styles.topBar}>
            <Text style={styles.mediaTitle} numberOfLines={1}>
              {media?.title ?? ""}
            </Text>
            <View style={styles.badgeRow}>
              {selectedQuality > 0 && (
                <View style={styles.speedBadge}>
                  <Text style={styles.speedText}>{selectedQuality}p</Text>
                </View>
              )}
              <View style={styles.speedBadge}>
                <Text style={styles.speedText}>{playbackSpeed}x</Text>
              </View>
            </View>
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
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
  },
  mediaTitle: {
    color: "#fff",
    fontSize: 24,
    fontWeight: "600",
    flex: 1,
  },
  badgeRow: {
    flexDirection: "row",
    alignItems: "center",
    gap: 8,
    marginLeft: 16,
  },
  speedBadge: {
    backgroundColor: "rgba(255,255,255,0.2)",
    paddingHorizontal: 12,
    paddingVertical: 4,
    borderRadius: 12,
  },
  speedText: {
    color: "#fff",
    fontSize: 16,
    fontWeight: "700",
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
