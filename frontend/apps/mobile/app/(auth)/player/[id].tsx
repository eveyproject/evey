import { useEffect, useState, useCallback, useRef, useMemo } from "react";
import {
  View,
  Text,
  Pressable,
  ActivityIndicator,
  StatusBar,
  StyleSheet,
  useWindowDimensions,
} from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { useLocalSearchParams, router } from "expo-router";
import { useVideoPlayer, VideoView, isPictureInPictureSupported } from "expo-video";
import { Ionicons } from "@expo/vector-icons";
import * as ScreenOrientation from "expo-screen-orientation";
import * as NavigationBar from "expo-navigation-bar";
import { getCastBaseUrl } from "../../../src/lib/server-config";
import { useTranslation } from "react-i18next";
import { useLibraryServiceGetMediaItem } from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { usePlaySession } from "../../../src/hooks/usePlaySession";
import { useProgressReporter } from "../../../src/hooks/useProgressReporter";
import { usePlayerGestures } from "../../../src/hooks/usePlayerGestures";
import { useNextEpisode } from "../../../src/hooks/useNextEpisode";
import { useChapters } from "../../../src/hooks/useChapters";
import { AudioTrackPicker, type AudioStream } from "../../../src/components/AudioTrackPicker";
import { SubtitlePicker, type SubtitleStream } from "../../../src/components/SubtitlePicker";
import { QualityPicker } from "../../../src/components/QualityPicker";
import { ChaptersPicker } from "../../../src/components/ChaptersPicker";
import { StreamInfoPanel } from "../../../src/components/StreamInfoPanel";
import { SkipIntroOverlay } from "../../../src/components/SkipIntroOverlay";
import { TrickplayPreview } from "../../../src/components/TrickplayPreview";
import {
  CastContext, CastButtonNative, useCastStateHook, useRemoteMediaClientHook,
  useMediaStatusHook, CastStateEnum,
} from "../../../src/hooks/useChromecast";

const IMAGE_SUBTITLE_CODECS = new Set([
  "pgssub", "hdmv_pgs_subtitle", "dvd_subtitle", "dvbsub", "xsub",
]);

const CONTROLS_TIMEOUT = 3000;

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
  const { t } = useTranslation();
  const { width: screenW, height: screenH } = useWindowDimensions();
  const insets = useSafeAreaInsets();
  const mediaId = id ?? "";

  const { data: media } = useLibraryServiceGetMediaItem(mediaId);
  const { streamUrl, sessionId, streamInfo, isLoading, isError, stopSession, restartWithTracks, restartAtPosition, transcodeOffset, progress } =
    usePlaySession(mediaId);

  const { data: chapters = [] } = useChapters(mediaId);
  const videoViewRef = useRef<VideoView>(null);
  const pipSupported = useMemo(() => isPictureInPictureSupported(), []);
  const mountedRef = useRef(true);
  useEffect(() => { return () => { mountedRef.current = false; }; }, []);

  const [showControls, setShowControls] = useState(true);
  const [showAudioPicker, setShowAudioPicker] = useState(false);
  const [showSubtitlePicker, setShowSubtitlePicker] = useState(false);
  const [showQualityPicker, setShowQualityPicker] = useState(false);
  const [showChaptersPicker, setShowChaptersPicker] = useState(false);
  const [showStreamInfo, setShowStreamInfo] = useState(false);
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
  const [seekTouchX, setSeekTouchX] = useState<number | null>(null);
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
      if (!mountedRef.current) return;
      setCurrentTime(ev.currentTime + transcodeOffset);
      try { setDuration(player.duration); } catch { /* released */ }
    });
    const statusSub = player.addListener("statusChange", () => {
      if (!mountedRef.current) return;
      try { setIsPlaying(player.playing); setDuration(player.duration); } catch { /* released */ }
    });
    const playSub = player.addListener("playingChange", () => {
      if (!mountedRef.current) return;
      try { setIsPlaying(player.playing); } catch { /* released */ }
    });
    return () => {
      timeSub.remove();
      statusSub.remove();
      playSub.remove();
    };
  }, [player]);

  const castState = useCastStateHook();
  const castClient = useRemoteMediaClientHook();
  const castMediaStatus = useMediaStatusHook();
  const castConnected = castState === CastStateEnum.CONNECTED;
  const castCurrentTime = castMediaStatus?.streamPosition ?? 0;
  const castDuration = castMediaStatus?.mediaInfo?.streamDuration ?? (media?.durationMs ? Number(media.durationMs) / 1000 : 0);
  const castIsPlaying = castMediaStatus?.playerState === 2; // MediaPlayerState.PLAYING

  const muteLocalPlayer = useCallback(() => {
    if (!player) return;
    try { player.pause(); } catch { /* released */ }
    try { player.volume = 0; } catch { /* released */ }
  }, [player]);

  const unmuteLocalPlayer = useCallback(() => {
    if (!player) return;
    try { player.volume = volume; } catch { /* released */ }
  }, [player, volume]);

  const castLoadMedia = useCallback(async (client: any) => {
    if (!streamUrl || !media?.title) return;
    muteLocalPlayer();
    // Use cast base URL if configured (e.g. different network than server)
    let castUrl = streamUrl;
    try {
      const castBase = await getCastBaseUrl();
      if (castBase) {
        const parsed = new URL(streamUrl);
        castUrl = `${castBase}${parsed.pathname}${parsed.search}`;
      }
    } catch { /* use original */ }
    console.log("[Cast] loadMedia URL:", castUrl);
    const isHls = castUrl.includes("m3u8");
    try {
      await client.loadMedia({
        autoplay: true,
        mediaInfo: {
          contentId: mediaId,
          contentUrl: castUrl,
          contentType: isHls ? "application/x-mpegurl" : "video/mp4",
          streamType: "buffered",
          metadata: { type: "movie", title: media.title },
        },
        startTime: currentTime,
      });
    } catch (e) { console.warn("[CAST] loadMedia failed:", e); }
  }, [streamUrl, media?.title, mediaId, currentTime, muteLocalPlayer]);

  const handleCast = useCallback(async () => {
    if (!CastContext) return;
    if (castConnected && castClient) {
      await castLoadMedia(castClient);
    } else {
      muteLocalPlayer();
      CastContext.showCastDialog?.();
    }
  }, [castConnected, castClient, castLoadMedia, muteLocalPlayer]);

  // Auto-load when cast client becomes available after dialog
  const castLoadedForMedia = useRef<string | null>(null);
  useEffect(() => {
    if (!castClient || !castConnected || !streamUrl) return;
    if (castLoadedForMedia.current === streamUrl) return;
    castLoadedForMedia.current = streamUrl;
    castLoadMedia(castClient);
  }, [castClient, castConnected, streamUrl, castLoadMedia]);

  // Unmute local player when cast disconnects
  useEffect(() => {
    if (!castConnected) {
      castLoadedForMedia.current = null;
      unmuteLocalPlayer();
    }
  }, [castConnected, unmuteLocalPlayer]);

  useEffect(() => {
    ScreenOrientation.unlockAsync();
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

  const knownDuration = media?.durationMs ? Number(media.durationMs) / 1000 : 0;
  const effectiveDuration = knownDuration > 0 ? knownDuration : duration;

  const isTranscode = streamInfo?.playMethod === "PLAY_METHOD_TRANSCODE";

  const seekPreview = useCallback(
    (pct: number) => {
      const d = knownDuration > 0 ? knownDuration : duration;
      if (d <= 0) return;
      setCurrentTime(pct * d);
      resetHideTimer();
    },
    [duration, knownDuration, resetHideTimer],
  );

  const seekTo = useCallback(
    (pct: number) => {
      const d = knownDuration > 0 ? knownDuration : duration;
      if (d <= 0) return;
      const targetSec = pct * d;
      if (isTranscode) {
        restartAtPosition(targetSec);
      } else if (player) {
        player.currentTime = targetSec;
      }
      resetHideTimer();
    },
    [player, duration, knownDuration, resetHideTimer, isTranscode, restartAtPosition],
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
    (value: number) => {
      if (!player) return;
      player.volume = value;
      volumeBaseRef.current = value;
      setVolume(value);
    },
    [player],
  );

  const brightnessBaseRef = useRef(1);
  const handleBrightnessChange = useCallback(
    async (value: number) => {
      try {
        const Brightness = require("expo-brightness");
        await Brightness.setBrightnessAsync(value);
        brightnessBaseRef.current = value;
      } catch { /* expo-brightness not available */ }
    },
    [],
  );

  const getGestureVolume = useCallback(() => volumeBaseRef.current, []);
  const getGestureBrightness = useCallback(() => brightnessBaseRef.current, []);

  const { panResponder, seekFlash, volumeIndicator, brightnessIndicator, cleanup } = usePlayerGestures({
    onSeekBy: seekBy,
    onTogglePlayPause: togglePlayPause,
    onToggleControls: toggleControls,
    onVolumeChange: handleVolumeChange,
    onBrightnessChange: handleBrightnessChange,
    getVolume: getGestureVolume,
    getBrightness: getGestureBrightness,
  });

  useEffect(() => cleanup, [cleanup]);

  useEffect(() => {
    NavigationBar.setVisibilityAsync(showControls ? "visible" : "hidden");
  }, [showControls]);

  useEffect(() => {
    NavigationBar.setVisibilityAsync("hidden");
    return () => { NavigationBar.setVisibilityAsync("visible"); };
  }, []);

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
    duration: effectiveDuration,
    onNavigate: handleNavigateEpisode,
  });

  const getPositionMs = useCallback(() => {
    if (!mountedRef.current) return 0;
    try { return ((player?.currentTime ?? 0) + transcodeOffset) * 1000; } catch { return 0; }
  }, [player, transcodeOffset]);

  const getIsPaused = useCallback(() => {
    if (!mountedRef.current) return true;
    try { return !(player?.playing ?? false); } catch { return true; }
  }, [player]);

  const getVolume = useCallback(() => {
    if (!mountedRef.current) return 1;
    try { return player?.volume ?? 1; } catch { return 1; }
  }, [player]);

  const getIsMuted = useCallback(() => {
    if (!mountedRef.current) return false;
    try { return player?.muted ?? false; } catch { return false; }
  }, [player]);

  const remoteCallbacks = useMemo(() => ({
    onRemotePause: () => { try { player?.pause(); } catch { /* released */ } },
    onRemotePlay: () => { try { player?.play(); } catch { /* released */ } },
    onRemoteStop: () => { stopSession(); router.back(); },
  }), [player, stopSession]);

  useProgressReporter(sessionId, getPositionMs, getIsPaused, getVolume, getIsMuted, remoteCallbacks);

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
        <Text style={{ color: theme.colors.text, fontSize: 16 }}>{t("player.failedToLoadStream")}</Text>
        <Pressable onPress={handleBack} style={{ marginTop: 16 }}>
          <Text style={{ color: theme.colors.accent, fontSize: 16 }}>{t("player.goBack")}</Text>
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

  const seekProgress = effectiveDuration > 0 ? currentTime / effectiveDuration : 0;
  const volumePct = Math.round(volume * 100);

  // Casting mode: show controller instead of local player
  if (castConnected) {
    const castProgress = castDuration > 0 ? castCurrentTime / castDuration : 0;
    return (
      <View style={[styles.container, { backgroundColor: "#000" }]}>
        <StatusBar hidden />
        <View style={{ flex: 1, justifyContent: "center", alignItems: "center", padding: 40 }}>
          <Ionicons name="tv" size={64} color={theme.colors.accent} />
          <Text style={{ color: "#fff", fontSize: 20, fontWeight: "700", marginTop: 20 }}>
            {t("player.controls.castingTo", "Casting to")}
          </Text>
          <Text style={{ color: theme.colors.accent, fontSize: 16, marginTop: 4 }}>
            {media?.title ?? ""}
          </Text>

          {/* Timeline */}
          <View style={{ width: "100%", marginTop: 32, paddingHorizontal: 8 }}>
            <View style={{ height: 4, borderRadius: 2, backgroundColor: "rgba(255,255,255,0.2)", overflow: "hidden" }}>
              <View style={{ height: "100%", borderRadius: 2, backgroundColor: theme.colors.accent, width: `${Math.min(100, castProgress * 100)}%` }} />
            </View>
            <View style={{ flexDirection: "row", justifyContent: "space-between", marginTop: 6 }}>
              <Text style={{ color: "rgba(255,255,255,0.7)", fontSize: 12 }}>{formatTime(castCurrentTime)}</Text>
              <Text style={{ color: "rgba(255,255,255,0.7)", fontSize: 12 }}>{formatTime(castDuration)}</Text>
            </View>
          </View>

          {/* Controls */}
          <View style={{ flexDirection: "row", alignItems: "center", gap: 32, marginTop: 24 }}>
            <Pressable onPress={() => castClient?.seek?.({ position: Math.max(0, castCurrentTime - 10) })}>
              <Ionicons name="play-back" size={32} color="#fff" />
            </Pressable>
            <Pressable onPress={() => {
              if (castIsPlaying) castClient?.pause?.();
              else castClient?.play?.();
            }}>
              <Ionicons name={castIsPlaying ? "pause" : "play"} size={48} color="#fff" />
            </Pressable>
            <Pressable onPress={() => castClient?.seek?.({ position: castCurrentTime + 10 })}>
              <Ionicons name="play-forward" size={32} color="#fff" />
            </Pressable>
          </View>
          {CastButtonNative && (
            <View style={{ marginTop: 32 }}>
              <CastButtonNative style={{ width: 48, height: 48, tintColor: theme.colors.accent }} />
            </View>
          )}
          <Pressable onPress={handleBack} style={{ marginTop: 24 }}>
            <Text style={{ color: theme.colors.textMuted, fontSize: 14 }}>{t("player.goBack")}</Text>
          </Pressable>
        </View>
      </View>
    );
  }

  return (
    <View style={[styles.container, { backgroundColor: "#000" }]}>
      <StatusBar hidden />

      <View style={{ width: screenW, height: screenH }} {...panResponder.panHandlers}>
        <View pointerEvents="none" style={styles.video}>
          <VideoView
            ref={videoViewRef}
            player={player}
            style={styles.video}
            nativeControls={false}
            contentFit="contain"
            allowsPictureInPicture={pipSupported}
          />
        </View>

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
              <View style={[styles.volumeBarFill, { height: `${Math.round(volumeIndicator * 100)}%` }]} />
            </View>
            <Text style={styles.volumeText}>{Math.round(volumeIndicator * 100)}</Text>
          </View>
        )}

        {/* Brightness indicator */}
        {brightnessIndicator !== null && (
          <View style={styles.brightnessIndicator}>
            <Text style={styles.volumeIcon}>{"\u2600"}</Text>
            <View style={styles.volumeBarBg}>
              <View style={[styles.volumeBarFill, { height: `${Math.round(brightnessIndicator * 100)}%` }]} />
            </View>
            <Text style={styles.volumeText}>{Math.round(brightnessIndicator * 100)}</Text>
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
                ? t("player.nextEpisodeOverlay.nextIn", { seconds: Math.max(nextEp.countdown, 0) })
                : t("player.nextEpisodeOverlay.upNext")}
            </Text>
            <Text style={styles.nextEpTitle} numberOfLines={1}>
              {nextEp.nextEpisode.indexNumber != null
                ? t("player.nextEpisodeOverlay.episode", { number: nextEp.nextEpisode.indexNumber })
                : t("player.nextEpisodeOverlay.nextEpisode")}
              {nextEp.nextEpisode.title ? ` - ${nextEp.nextEpisode.title}` : ""}
            </Text>
            <View style={styles.nextEpButtons}>
              <Pressable onPress={nextEp.playNow} style={[styles.nextEpPlayBtn, { backgroundColor: theme.colors.accent }]}>
                <Text style={styles.nextEpPlayText}>{t("player.nextEpisodeOverlay.playNow")}</Text>
              </Pressable>
              <Pressable onPress={nextEp.cancel} style={styles.nextEpCancelBtn}>
                <Text style={styles.nextEpCancelText}>{t("common.cancel")}</Text>
              </Pressable>
            </View>
            {nextEp.ended && (
              <View style={styles.nextEpProgressBg}>
                <View
                  style={[
                    styles.nextEpProgressFill,
                    { width: `${((5 - Math.max(nextEp.countdown, 0)) / 5) * 100}%`, backgroundColor: theme.colors.accent },
                  ]}
                />
              </View>
            )}
          </View>
        )}

        {showControls && (
          <View style={styles.overlay} pointerEvents="box-none">
            <View style={[styles.topBar, { top: 0, paddingTop: Math.max(16, insets.top), paddingLeft: Math.max(20, insets.left), paddingRight: Math.max(20, insets.right) }]}>
              <Pressable onPress={handleBack} hitSlop={16} accessible={true} accessibilityLabel={t("common.back")} accessibilityRole="button">
                <Ionicons name="chevron-back" size={28} color="#fff" />
              </Pressable>
              <Text style={styles.title} numberOfLines={1}>
                {media?.title ?? ""}
              </Text>
              <View style={styles.topBarRight}>
                <Pressable
                  onPress={() => setShowStreamInfo(true)}
                  hitSlop={8}
                  style={styles.trackButton}
                  accessible={true}
                  accessibilityLabel={t("player.streamInfo.title", "Stream Info")}
                  accessibilityRole="button"
                >
                  <Ionicons name="information-circle-outline" size={18} color="#fff" />
                </Pressable>
                <Pressable
                  onPress={cycleSpeed}
                  hitSlop={8}
                  style={styles.trackButton}
                  accessible={true}
                  accessibilityLabel={t("player.settings.speed")}
                  accessibilityRole="button"
                >
                  <Text style={styles.speedButtonText}>{playbackSpeed}x</Text>
                </Pressable>
                <Pressable
                  onPress={() => setShowQualityPicker(true)}
                  hitSlop={8}
                  style={styles.trackButton}
                  accessible={true}
                  accessibilityLabel={t("player.settings.quality")}
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
                    accessibilityLabel={t("player.controls.chapters")}
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
                    accessibilityLabel={t("player.settings.audio")}
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
                    accessibilityLabel={t("player.settings.subtitles")}
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
                    accessibilityLabel={t("player.controls.pip")}
                    accessibilityRole="button"
                  >
                    <Ionicons name="albums-outline" size={18} color="#fff" />
                  </Pressable>
                )}
                {CastContext && (
                  <>
                    {CastButtonNative && <CastButtonNative style={{ width: 0, height: 0, opacity: 0 }} />}
                    <Pressable
                      onPress={handleCast}
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
              </View>
            </View>

            <View style={[styles.bottomControls, { paddingBottom: Math.max(20, insets.bottom), paddingLeft: Math.max(20, insets.left), paddingRight: Math.max(20, insets.right) }]}>
              {/* Seek bar */}
              <View
                style={styles.seekBarContainer}
                accessible={true}
                accessibilityRole="adjustable"
                accessibilityLabel={t("player.controls.seek")}
                onStartShouldSetResponder={() => true}
                onMoveShouldSetResponder={() => true}
                onResponderGrant={(e) => {
                  const x = e.nativeEvent.locationX;
                  setSeekTouchX(x);
                  seekPreview(Math.max(0, Math.min(1, x / (screenW - insets.left - insets.right - 40))));
                }}
                onResponderMove={(e) => {
                  const x = e.nativeEvent.locationX;
                  setSeekTouchX(x);
                  seekPreview(Math.max(0, Math.min(1, x / (screenW - insets.left - insets.right - 40))));
                }}
                onResponderRelease={(e) => {
                  const x = e.nativeEvent.locationX;
                  seekTo(Math.max(0, Math.min(1, x / (screenW - insets.left - insets.right - 40))));
                  setSeekTouchX(null);
                }}
              >
                {seekTouchX != null && (
                  <View style={{ position: "absolute", bottom: "50%", left: seekTouchX, transform: [{ translateX: -80 }] }}>
                    <TrickplayPreview mediaId={mediaId} currentTime={currentTime} visible={showControls && effectiveDuration > 0} />
                  </View>
                )}
                <View style={styles.seekBarTrack}>
                  <View style={[styles.seekBarFill, { flex: seekProgress, backgroundColor: theme.colors.accent }]} />
                  <View style={{ flex: 1 - seekProgress }} />
                </View>
                <View style={[styles.seekThumb, { left: `${seekProgress * 100}%`, backgroundColor: theme.colors.accent }]} />
                {effectiveDuration > 0 && chapters.map((ch, i) => {
                  const pct = (ch.startTimeMs / 1000) / effectiveDuration;
                  if (pct <= 0 || pct >= 1) return null;
                  return (
                    <View
                      key={i}
                      style={[styles.chapterMarker, { left: `${pct * 100}%` }]}
                    />
                  );
                })}
              </View>

              {/* Controls row */}
              <View style={[styles.controlsRow, { marginTop: 4 }]}>
                <Pressable onPress={() => seekBy(-10)} hitSlop={12} style={styles.controlBtn} accessible={true} accessibilityLabel={t("player.controls.rewind", { seconds: 10 })} accessibilityRole="button">
                  <Ionicons name="play-back" size={20} color="#fff" />
                </Pressable>
                <Pressable onPress={togglePlayPause} hitSlop={12} style={styles.controlBtn} accessible={true} accessibilityLabel={isPlaying ? t("player.controls.pause") : t("player.controls.play")} accessibilityRole="button">
                  <Ionicons name={isPlaying ? "pause" : "play"} size={24} color="#fff" />
                </Pressable>
                <Pressable onPress={() => seekBy(10)} hitSlop={12} style={styles.controlBtn} accessible={true} accessibilityLabel={t("player.controls.forward", { seconds: 10 })} accessibilityRole="button">
                  <Ionicons name="play-forward" size={20} color="#fff" />
                </Pressable>
                <Text style={styles.timeText}>{formatTime(currentTime)} / {formatTime(effectiveDuration)}</Text>
                <View style={{ flex: 1 }} />
              </View>
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

      <StreamInfoPanel
        visible={showStreamInfo}
        onClose={() => setShowStreamInfo(false)}
        streamInfo={streamInfo}
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
    flex: 1,
  },
  video: {
    width: "100%",
    height: "100%",
  },
  overlay: {
    position: "absolute",
    top: 0,
    left: 0,
    right: 0,
    bottom: 0,
    backgroundColor: "rgba(0,0,0,0.4)",
  },
  topBar: {
    position: "absolute",
    left: 0,
    right: 0,
    flexDirection: "row",
    alignItems: "center",
    paddingTop: 16,
    paddingHorizontal: 20,
    zIndex: 2,
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
  bottomControls: {
    position: "absolute",
    left: 0,
    right: 0,
    bottom: 0,
    paddingHorizontal: 20,
    zIndex: 2,
  },
  controlsRow: {
    flexDirection: "row",
    alignItems: "center",
    gap: 16,
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
  seekBarContainer: {
    flex: 1,
    height: 32,
    justifyContent: "center",
    overflow: "visible",
  },
  seekBarTrack: {
    height: 4,
    borderRadius: 2,
    backgroundColor: "rgba(255,255,255,0.3)",
    flexDirection: "row",
    overflow: "hidden",
  },
  seekBarFill: {
  },
  seekThumb: {
    position: "absolute",
    top: 10,
    width: 14,
    height: 14,
    borderRadius: 7,
    marginLeft: -7,
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
  brightnessIndicator: {
    position: "absolute",
    left: 40,
    top: "25%",
    height: "50%",
    width: 36,
    alignItems: "center",
    justifyContent: "center",
    backgroundColor: "rgba(0,0,0,0.6)",
    borderRadius: 18,
    paddingVertical: 8,
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
