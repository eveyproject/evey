import { useEffect, useState, useCallback, useRef, useMemo } from "react";
import { View, Text, Pressable, ActivityIndicator, StyleSheet, useWindowDimensions, useTVEventHandler } from "react-native";
import { useLocalSearchParams, router } from "expo-router";
import { useVideoPlayer, VideoView } from "expo-video";
import {
  useLibraryServiceGetMediaItem,
  usePlaybackServiceMarkAsWatched,
  usePlaybackServiceGetWatchProgress,
} from "@evey/api";
import { useAuthStore } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { usePlaySession } from "../../../src/hooks/usePlaySession";
import { useProgressReporter } from "../../../src/hooks/useProgressReporter";
import { usePlaybackSettings } from "../../../src/hooks/usePlaybackSettings";
import { useNextEpisode } from "../../../src/hooks/useNextEpisode";
import { QualityPicker } from "../../../src/components/QualityPicker";
import { AudioTrackPicker, type AudioStream } from "../../../src/components/AudioTrackPicker";
import { SubtitlePicker, type SubtitleStream } from "../../../src/components/SubtitlePicker";
import { TrackOptionsMenu } from "../../../src/components/TrackOptionsMenu";
import { SkipIntroButton } from "../../../src/components/SkipIntroButton";
import { NextEpisodeOverlay } from "../../../src/components/NextEpisodeOverlay";
import { useParentalGate } from "../../../src/hooks/useParentalGate";
import { ParentalPINModal } from "../../../src/components/ParentalPINModal";

const IMAGE_SUBTITLE_CODECS = new Set([
  "pgssub", "hdmv_pgs_subtitle", "dvd_subtitle", "dvbsub", "xsub",
]);

const CONTROLS_TIMEOUT = 4000;

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
  const { width: screenWidth, height: screenHeight } = useWindowDimensions();

  const { data: media } = useLibraryServiceGetMediaItem(mediaId);
  const { streamUrl, sessionId, streamInfo, isLoading, isError, stopSession, restartWithTracks, restartAtPosition, transcodeOffset, progress } =
    usePlaySession(mediaId);

  const [showControls, setShowControls] = useState(true);
  const [showQualityPicker, setShowQualityPicker] = useState(false);
  const [showTrackMenu, setShowTrackMenu] = useState(false);
  const [showAudioPicker, setShowAudioPicker] = useState(false);
  const [showSubtitlePicker, setShowSubtitlePicker] = useState(false);
  const [selectedQuality, setSelectedQuality] = useState(0);
  const [selectedAudioIndex, setSelectedAudioIndex] = useState<number>(0);
  const [selectedSubtitleIndex, setSelectedSubtitleIndex] = useState<number | null>(null);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [isPlaying, setIsPlaying] = useState(false);
  const [playbackSpeed, setPlaybackSpeed] = useState(1);
  const hideTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

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

  // Set initial selected audio index from the default stream.
  useEffect(() => {
    const def = audioStreams.find((s) => s.isDefault);
    if (def) setSelectedAudioIndex(def.index);
    else if (audioStreams.length > 0) setSelectedAudioIndex(audioStreams[0].index);
  }, [audioStreams]);

  const videoSource = useMemo(() => {
    if (!streamUrl) return null;
    const isHls = streamUrl.includes("m3u8");
    return {
      uri: streamUrl,
      ...(isHls ? { contentType: "hls" as const } : {}),
    };
  }, [streamUrl]);

  const resumePositionSec = useMemo(() => {
    if (!progress?.positionMs || progress.played) return 0;
    const ms = Number(progress.positionMs);
    return ms > 0 ? ms / 1000 : 0;
  }, [progress]);

  const player = useVideoPlayer(videoSource, (p) => {
    p.timeUpdateEventInterval = 0.5;
    if (resumePositionSec > 0) {
      p.currentTime = resumePositionSec;
    }
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

  const { data: playbackSettings } = usePlaybackSettings();
  const skipFwd = playbackSettings?.skipForwardSeconds ?? 30;
  const skipBwd = playbackSettings?.skipBackwardSeconds ?? 30;

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
      restartWithTracks(selectedAudioIndex, selectedSubtitleIndex ?? undefined, quality || undefined);
    },
    [restartWithTracks, selectedAudioIndex, selectedSubtitleIndex],
  );

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
      restartWithTracks(selectedAudioIndex, index ?? undefined, selectedQuality || undefined);
    },
    [restartWithTracks, selectedAudioIndex, selectedQuality],
  );

  const handleSkipIntro = useCallback(
    (toSec: number) => {
      if (player) player.currentTime = toSec;
    },
    [player],
  );

  const handleBack = useCallback(() => {
    stopSession();
    router.back();
  }, [stopSession]);

  const handleNavigateEpisode = useCallback(
    (episodeId: string) => {
      stopSession();
      router.replace(`/(auth)/player/${episodeId}` as never);
    },
    [stopSession],
  );

  // The next-episode auto-advance (and its "play now" button) jump straight
  // into another player instance, bypassing the media detail screen where
  // the parental PIN gate normally lives. useNextEpisode calls onNavigate
  // both on its own countdown and from playNow, so gating that single
  // callback covers both paths. onNavigate is wired through a ref so the
  // gate (which needs nextEp.nextEpisode's rating, only known once
  // Entry gate: the player is reachable WITHOUT going through the media detail
  // screen - the home hero, a deep link, and the remote "loadAndPlay" device
  // command all land straight here. Gating the screen itself (rather than each
  // caller) is the invariant that holds for any future entry point. The server
  // refuses the stream regardless; this turns a dead error into a PIN prompt.
  const { needsPin: entryNeedsPin, profileId: entryGateProfileId } = useParentalGate(media?.officialRating);
  const [entryUnlocked, setEntryUnlocked] = useState(false);
  const entryGated = !!media && !entryUnlocked && entryNeedsPin();

  useEffect(() => {
    if (entryGated) player?.pause();
  }, [entryGated, player]);

  const handleEntryPinVerified = useCallback(() => {
    setEntryUnlocked(true);
    player?.play();
  }, [player]);

  // useNextEpisode itself has resolved it) can be defined after the hook
  // call without creating a circular dependency.
  const nextEpNavigateRef = useRef<(episodeId: string) => void>(() => {});
  const forwardNextEpNavigate = useCallback((episodeId: string) => {
    nextEpNavigateRef.current(episodeId);
  }, []);

  const nextEp = useNextEpisode({
    media,
    currentTime,
    duration,
    onNavigate: forwardNextEpNavigate,
  });

  const { needsPin: nextEpNeedsPin, profileId: nextEpGateProfileId } = useParentalGate(
    nextEp.nextEpisode?.officialRating,
  );
  const [showNextEpPinModal, setShowNextEpPinModal] = useState(false);
  const [pendingNextEpisodeId, setPendingNextEpisodeId] = useState<string | null>(null);

  const gatedNavigateEpisode = useCallback(
    (episodeId: string) => {
      if (nextEpNeedsPin()) {
        setPendingNextEpisodeId(episodeId);
        setShowNextEpPinModal(true);
        return;
      }
      handleNavigateEpisode(episodeId);
    },
    [nextEpNeedsPin, handleNavigateEpisode],
  );
  nextEpNavigateRef.current = gatedNavigateEpisode;

  const handleNextEpPinVerified = useCallback(() => {
    setShowNextEpPinModal(false);
    if (pendingNextEpisodeId) {
      handleNavigateEpisode(pendingNextEpisodeId);
      setPendingNextEpisodeId(null);
    }
  }, [pendingNextEpisodeId, handleNavigateEpisode]);

  const anyOverlayOpen =
    showQualityPicker || showTrackMenu || showAudioPicker || showSubtitlePicker || nextEp.visible;

  // D-pad remote handler. The callback param is annotated explicitly: the
  // react-native-tvos alias resolves per-app, and the hoisted classic RN
  // types (shared workspace) leave HWEvent unresolvable in some installs -
  // a structural annotation keeps tsc deterministic everywhere.
  useTVEventHandler((evt: { eventType: string } | undefined) => {
    if (!evt) return;
    const type = evt.eventType;

    // While any overlay (picker/menu/next-episode card) is showing, its own
    // FocusableCard rows own the D-pad via the native TV focus engine - if we
    // kept interpreting the same key here (e.g. "select") it would double-fire
    // (native onPress AND togglePlayPause()).
    if (anyOverlayOpen) return;
    if (type === "select" || type === "playPause") {
      togglePlayPause();
    } else if (type === "longSelect") {
      if (showControls && (audioStreams.length > 1 || subtitleStreams.length > 0)) {
        setShowTrackMenu(true);
      }
    } else if (type === "left") {
      seekBy(-skipBwd);
    } else if (type === "right") {
      seekBy(skipFwd);
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

  const getIsPaused = useCallback(() => {
    try {
      return !(player?.playing ?? false);
    } catch {
      return true;
    }
  }, [player]);

  const remoteCallbacks = useMemo(
    () => ({
      onRemotePause: () => {
        try {
          player?.pause();
        } catch (err) {
          console.warn("remote pause failed:", err);
        }
      },
      onRemotePlay: () => {
        try {
          player?.play();
        } catch (err) {
          console.warn("remote play failed:", err);
        }
      },
      onRemoteStop: () => {
        stopSession();
        router.back();
      },
    }),
    [player, stopSession],
  );

  useProgressReporter(sessionId, getPositionMs, getIsPaused, undefined, undefined, remoteCallbacks);

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

  const seekProgress = duration > 0 ? currentTime / duration : 0;

  return (
    <View style={[styles.container, { backgroundColor: "#000" }]}>
      <VideoView
        player={player}
        style={[styles.video, { width: screenWidth, height: screenHeight }]}
        nativeControls={false}
        contentFit="contain"
      />

      <QualityPicker
        visible={showQualityPicker}
        currentQuality={selectedQuality}
        onSelect={handleQualitySelect}
        onClose={() => setShowQualityPicker(false)}
      />

      <TrackOptionsMenu
        visible={showTrackMenu}
        onClose={() => setShowTrackMenu(false)}
        hasAudioOptions={audioStreams.length > 1}
        hasSubtitleOptions={subtitleStreams.length > 0}
        onOpenAudio={() => {
          setShowTrackMenu(false);
          setShowAudioPicker(true);
        }}
        onOpenSubtitles={() => {
          setShowTrackMenu(false);
          setShowSubtitlePicker(true);
        }}
      />

      <AudioTrackPicker
        visible={showAudioPicker}
        streams={audioStreams}
        currentIndex={selectedAudioIndex}
        onSelect={handleAudioSelect}
        onClose={() => setShowAudioPicker(false)}
      />

      <SubtitlePicker
        visible={showSubtitlePicker}
        streams={subtitleStreams}
        currentIndex={selectedSubtitleIndex}
        onSelect={handleSubtitleSelect}
        onClose={() => setShowSubtitlePicker(false)}
      />

      <SkipIntroButton mediaId={mediaId} currentTime={currentTime} onSkip={handleSkipIntro} />

      <NextEpisodeOverlay
        visible={nextEp.visible}
        ended={nextEp.ended}
        countdown={nextEp.countdown}
        nextEpisode={nextEp.nextEpisode}
        onPlayNow={nextEp.playNow}
        onCancel={nextEp.cancel}
      />

      {entryGateProfileId ? (
        <ParentalPINModal
          visible={entryGated}
          onClose={() => router.back()}
          onVerified={handleEntryPinVerified}
          profileId={entryGateProfileId}
        />
      ) : null}

      {nextEpGateProfileId ? (
        <ParentalPINModal
          visible={showNextEpPinModal}
          onClose={() => { setShowNextEpPinModal(false); setPendingNextEpisodeId(null); }}
          onVerified={handleNextEpPinVerified}
          profileId={nextEpGateProfileId}
        />
      ) : null}

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
              <Text style={styles.seekHintText}>-{skipBwd}s</Text>
            </View>
            <View style={styles.playIndicator}>
              <Text style={styles.playIcon}>{isPlaying ? "||" : "\u25B6"}</Text>
            </View>
            <View style={styles.seekHint}>
              <Text style={styles.seekHintText}>+{skipFwd}s</Text>
            </View>
          </View>

          {/* Bottom bar */}
          <View style={styles.bottomBar}>
            <Text style={styles.timeText}>{formatTime(currentTime)}</Text>
            <View style={styles.seekBarTrack}>
              <View style={[styles.seekBarFill, { flex: seekProgress }]} />
              <View style={{ flex: 1 - seekProgress }} />
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
  video: {},
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
