import { useEffect, useState, useCallback, useRef, useMemo } from "react";
import {
  View,
  Text,
  Pressable,
  ActivityIndicator,
  StatusBar,
  StyleSheet,
  useWindowDimensions,
  BackHandler,
} from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { useLocalSearchParams, router } from "expo-router";
import { useVideoPlayer, VideoView, isPictureInPictureSupported } from "expo-video";
import { Ionicons } from "@expo/vector-icons";
import * as ScreenOrientation from "expo-screen-orientation";
import * as NavigationBar from "expo-navigation-bar";
import { setAudioBoost, releaseAudioBoost } from "../../../src/lib/audioBoost";
import { getDownloadPath } from "../../../src/lib/downloads";
import { useTranslation } from "react-i18next";
import { toast, useAuthStore } from "@evey/store";
import {
  useLibraryServiceGetMediaItem,
  useLibraryServiceListMediaItems,
  usePlaybackServiceMarkAsWatched,
} from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { usePlaySession } from "../../../src/hooks/usePlaySession";
import { useProgressReporter } from "../../../src/hooks/useProgressReporter";
import { usePlayerGestures } from "../../../src/hooks/usePlayerGestures";
import { usePlaybackSettings } from "../../../src/hooks/usePlaybackSettings";
import { useNextEpisode } from "../../../src/hooks/useNextEpisode";
import { useChapters } from "../../../src/hooks/useChapters";
import { useSleepTimer } from "../../../src/hooks/useSleepTimer";
import { SleepTimerPicker } from "../../../src/components/SleepTimerPicker";
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
import { useSyncPlay } from "../../../src/hooks/useSyncPlay";
import { SyncPlayBar } from "../../../src/components/SyncPlayBar";
import { SyncPlayModal } from "../../../src/components/SyncPlayModal";

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

  // Offline playback: if this media was downloaded, prefer the file already
  // on disk over the network stream. NOTE this only short-circuits the video
  // SOURCE - usePlaySession below still opens a play session against the
  // server for progress reporting/watched-state, so this doesn't give a fully
  // network-free experience (no network still means that session request
  // hangs/fails in the background); it does mean the video itself plays from
  // disk and isn't blocked on the network being reachable.
  const [localFilePath, setLocalFilePath] = useState<string | null>(null);
  useEffect(() => {
    let cancelled = false;
    if (!mediaId) { setLocalFilePath(null); return; }
    getDownloadPath(mediaId).then((p) => { if (!cancelled) setLocalFilePath(p); });
    return () => { cancelled = true; };
  }, [mediaId]);

  const { data: media } = useLibraryServiceGetMediaItem(mediaId);
  // Series and seasons have no underlying file, so the streaming endpoints 404.
  // Resolve to a playable child (season → episode → episode) so the home hero
  // and any "Play" button on a series detail Just Work. Two redirects max:
  //   /player/<seriesId>  → /player/<season1Id>
  //   /player/<season1Id> → /player/<episode1Id>
  const isSeriesOrSeason =
    media?.type === "MEDIA_ITEM_TYPE_SERIES" || media?.type === "MEDIA_ITEM_TYPE_SEASON";
  const { data: childrenData } = useLibraryServiceListMediaItems(
    { parentId: mediaId, "pagination.pageSize": 500 } as never,
    { query: { enabled: !!mediaId && isSeriesOrSeason } } as never,
  );
  useEffect(() => {
    if (!isSeriesOrSeason) return;
    const items = childrenData?.items;
    if (!items || items.length === 0) return;
    const sorted = [...items].sort((a, b) => (a.indexNumber ?? 0) - (b.indexNumber ?? 0));
    const target = sorted[0];
    if (target?.id && target.id !== mediaId) {
      router.replace(`/player/${target.id}` as never);
    }
  }, [isSeriesOrSeason, childrenData?.items, mediaId]);

  const isEpisode = media?.type === "MEDIA_ITEM_TYPE_EPISODE";
  const { data: parentItem } = useLibraryServiceGetMediaItem(
    media?.parentId ?? "",
    { query: { enabled: !!media?.parentId && isEpisode } } as never,
  );
  const episodeLabel = useMemo(() => {
    if (!isEpisode) return null;
    const s = parentItem?.indexNumber ?? 0;
    const e = media?.indexNumber ?? 0;
    if (s > 0 && e > 0) return `S${String(s).padStart(2, "0")}E${String(e).padStart(2, "0")}`;
    if (e > 0) return `E${String(e).padStart(2, "0")}`;
    return null;
  }, [isEpisode, parentItem?.indexNumber, media?.indexNumber]);
  const playerTitle = episodeLabel ? `${episodeLabel} - ${media?.title ?? ""}` : (media?.title ?? "");
  // Don't open a play session for a series/season - the redirect above will
  // remount us on a real episode in a tick. Passing "" makes usePlaySession
  // hold its initial loading state instead of hitting /transcode and 404'ing.
  const sessionMediaId = isSeriesOrSeason ? "" : mediaId;
  const { streamUrl, streamToken, sessionId, streamInfo, isLoading, isError, stopSession, restartWithTracks, restartAtPosition, transcodeOffset, progress } =
    usePlaySession(sessionMediaId);

  const { data: chapters = [] } = useChapters(mediaId);
  const videoViewRef = useRef<VideoView>(null);
  const pipSupported = useMemo(() => isPictureInPictureSupported(), []);
  const mountedRef = useRef(true);
  useEffect(() => { return () => { mountedRef.current = false; void releaseAudioBoost(); }; }, []);

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
  // While the user scrubs the timeline, freeze the live timeUpdate: otherwise it
  // overwrites the scrubbed position ~4×/s and the thumb jumps between the finger
  // and the real playback position (the "wobbly" seek).
  const seekingRef = useRef(false);
  useEffect(() => {
    seekingRef.current = seekTouchX != null;
  }, [seekTouchX]);
  // Measured seek-bar width (the bar is flex:1 between the time labels, so it is
  // NOT screenW-minus-insets) and the last previewed fraction. onResponderRelease's
  // locationX is unreliable on Android (it can report a coordinate relative to a
  // nested absolute child like the trickplay preview → a near-zero x → the seek
  // jumps to ~0:00). So we commit the fraction tracked during the reliable move
  // events, divided by the real measured width.
  const seekBarWidthRef = useRef(0);
  const seekPctRef = useRef(0);
  const pctFromX = useCallback((x: number) => {
    const w = seekBarWidthRef.current;
    if (w <= 0) return 0;
    return Math.max(0, Math.min(1, x / w));
  }, []);
  const volumeBaseRef = useRef(1);
  const hideTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [showSyncPlay, setShowSyncPlay] = useState(false);
  const [syncCreating, setSyncCreating] = useState(false);
  const [syncJoining, setSyncJoining] = useState(false);
  const [showSleepTimer, setShowSleepTimer] = useState(false);
  const sleepTimer = useSleepTimer({
    onMinutesFire: () => {
      try { player?.pause(); } catch { /* released */ }
      toast.info(t("player.sleepTimer.fired", "Sleep timer ended"));
    },
  });
  const syncPlayRef = useRef<{ player: typeof player } | null>(null);

  const syncPlay = useSyncPlay({
    onPlay: (posMs) => { if (syncPlayRef.current?.player) { syncPlayRef.current.player.currentTime = posMs / 1000; syncPlayRef.current.player.play(); } },
    onPause: (posMs) => { if (syncPlayRef.current?.player) { syncPlayRef.current.player.currentTime = posMs / 1000; syncPlayRef.current.player.pause(); } },
    onSeek: (posMs) => { if (syncPlayRef.current?.player) { syncPlayRef.current.player.currentTime = posMs / 1000; } },
  });

  const videoSource = useMemo(() => {
    // Downloaded copy on disk takes priority over the network stream - no
    // reason to re-download bytes the device already has, and it plays even
    // if the play-session request above is stuck/failing offline.
    if (localFilePath) {
      return { uri: localFilePath.startsWith("file://") ? localFilePath : `file://${localFilePath}` };
    }
    if (!streamUrl) return null;
    const isHls = streamUrl.includes("m3u8");
    return {
      uri: streamUrl,
      ...(isHls ? { contentType: "hls" as const } : {}),
      // Also send the opaque stream token as a Bearer header: the server accepts
      // cookie/Bearer/query, so if the native player forwards the source headers
      // to its segment requests they authenticate without the token in the URL.
      // The ?token= stays in the URL as the reliable fallback (some engines drop
      // headers on the manifest's relative segment/redirect fetches).
      ...(streamToken ? { headers: { Authorization: `Bearer ${streamToken}` } } : {}),
    };
  }, [streamUrl, streamToken, localFilePath]);

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

  useEffect(() => { syncPlayRef.current = { player }; }, [player]);

  useEffect(() => {
    if (!player) return;
    const timeSub = player.addListener("timeUpdate", (ev) => {
      if (!mountedRef.current) return;
      if (seekingRef.current) return; // frozen while scrubbing - see seekingRef
      setCurrentTime(ev.currentTime + transcodeOffset);
      try { setDuration(player.duration); } catch (err) { console.warn("released", err); }
    });
    const statusSub = player.addListener("statusChange", () => {
      if (!mountedRef.current) return;
      try { setIsPlaying(player.playing); setDuration(player.duration); } catch (err) { console.warn("released", err); }
    });
    const playSub = player.addListener("playingChange", () => {
      if (!mountedRef.current) return;
      try { setIsPlaying(player.playing); } catch (err) { console.warn("released", err); }
    });
    return () => {
      timeSub.remove();
      statusSub.remove();
      playSub.remove();
    };
    // transcodeOffset MUST be a dep: after a transcode seek it changes, and a
    // stale closure here would keep adding the old offset (0) - the position
    // snaps back to the start even though the server restarted at the seek point.
  }, [player, transcodeOffset]);

  const castState = useCastStateHook();
  const castClient = useRemoteMediaClientHook();
  const castMediaStatus = useMediaStatusHook();
  const castConnected = castState === CastStateEnum.CONNECTED;
  const castCurrentTime = castMediaStatus?.streamPosition ?? 0;
  const castDuration = castMediaStatus?.mediaInfo?.streamDuration ?? (media?.durationMs ? Number(media.durationMs) / 1000 : 0);
  const castIsPlaying = castMediaStatus?.playerState === 2; // MediaPlayerState.PLAYING

  const muteLocalPlayer = useCallback(() => {
    if (!player) return;
    try { player.pause(); } catch (err) { console.warn("released", err); }
    try { player.volume = 0; } catch (err) { console.warn("released", err); }
    void setAudioBoost(1); // drop the amplification while muted for casting
  }, [player]);

  const unmuteLocalPlayer = useCallback(() => {
    if (!player) return;
    // volume is the 0-2 level: restore the native part and re-arm the boost.
    try {
      player.volume = Math.min(1, volume);
      void setAudioBoost(volume > 1 ? volume : 1);
    } catch (err) { console.warn("released", err); }
  }, [player, volume]);

  const castLoadMedia = useCallback(async (client: any) => {
    if (!streamUrl || !media?.title) return;
    muteLocalPlayer();
    const castUrl = streamUrl;
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

  // Open in whatever orientation the user is currently holding the device in.
  // The user toggles fullscreen (== lock to landscape) explicitly via the
  // bottom bar button. Restore the OS default lock when leaving the screen.
  useEffect(() => {
    ScreenOrientation.unlockAsync();
    return () => {
      ScreenOrientation.lockAsync(ScreenOrientation.OrientationLock.DEFAULT);
    };
  }, []);

  const [isFullscreen, setIsFullscreen] = useState(false);
  const toggleFullscreen = useCallback(() => {
    setIsFullscreen((prev) => {
      const next = !prev;
      if (next) {
        ScreenOrientation.lockAsync(ScreenOrientation.OrientationLock.LANDSCAPE);
      } else {
        ScreenOrientation.lockAsync(ScreenOrientation.OrientationLock.PORTRAIT_UP);
      }
      return next;
    });
  }, []);

  useEffect(() => {
    const sub = BackHandler.addEventListener("hardwareBackPress", () => {
      router.back();
      return true;
    });
    return () => sub.remove();
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
      if (syncPlay.state.connected) syncPlay.sendPause(player.currentTime * 1000);
    } else {
      player.play();
      if (syncPlay.state.connected) syncPlay.sendPlay(player.currentTime * 1000);
    }
    resetHideTimer();
  }, [player, resetHideTimer, syncPlay]);

  const knownDuration = media?.durationMs ? Number(media.durationMs) / 1000 : 0;
  const effectiveDuration = knownDuration > 0 ? knownDuration : duration;

  // Mirror Jellyfin's behaviour: mark the item as watched once playback
  // crosses the MaxResumePct threshold (90%). The server only marks
  // Completed=true on explicit MarkAsWatched calls - without this the user
  // had to manually toggle "watched" after every episode.
  const profileId = useAuthStore((s) => s.profileId) ?? "";
  const markAsWatchedMut = usePlaybackServiceMarkAsWatched();
  const autoMarkedRef = useRef(false);
  useEffect(() => { autoMarkedRef.current = false; }, [mediaId]);
  useEffect(() => {
    if (autoMarkedRef.current) return;
    if (progress?.played) { autoMarkedRef.current = true; return; }
    if (!profileId || !mediaId) return;
    if (effectiveDuration <= 0 || currentTime <= 0) return;
    if (currentTime / effectiveDuration >= 0.9) {
      autoMarkedRef.current = true;
      markAsWatchedMut.mutate({ mediaItemId: mediaId, params: { profileId } });
    }
  }, [currentTime, effectiveDuration, progress?.played, profileId, mediaId, markAsWatchedMut]);

  const isTranscode = streamInfo?.playMethod === "PLAY_METHOD_TRANSCODE";

  const { data: playbackSettings } = usePlaybackSettings();
  const skipFwd = playbackSettings?.skipForwardSeconds ?? 30;
  const skipBwd = playbackSettings?.skipBackwardSeconds ?? 30;

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
      // Optimistic: show the target immediately so the release doesn't flash the
      // pre-seek position before the first post-seek timeUpdate lands.
      setCurrentTime(targetSec);
      // Native seek on the absolute HLS timeline - works for direct-play AND
      // transcode (the event playlist is 0-based; the server rafales any not-yet-
      // produced segment). Reloading the stream instead replayed it from the start.
      if (player) {
        player.currentTime = targetSec;
      }
      if (syncPlay.state.connected) syncPlay.sendSeek(targetSec * 1000);
      resetHideTimer();
    },
    [player, duration, knownDuration, resetHideTimer, isTranscode, restartAtPosition, syncPlay],
  );

  const seekBy = useCallback(
    (seconds: number) => {
      // Native relative seek on the 0-based HLS timeline (transcode included).
      if (player) {
        player.seekBy(seconds);
      }
      resetHideTimer();
    },
    [player, resetHideTimer],
  );

  const handleVolumeChange = useCallback(
    (value: number) => {
      if (!player) return;
      // 0-2 split: 0-1 rides the native element volume, 1-2 amplifies through the
      // native LoudnessEnhancer (Android; no-op elsewhere until its module lands).
      const v = Math.max(0, Math.min(2, value));
      player.volume = Math.min(1, v);
      void setAudioBoost(v > 1 ? v : 1);
      volumeBaseRef.current = v;
      setVolume(v);
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
      } catch (err) { console.warn("expo-brightness not available", err); }
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
    skipForwardSeconds: skipFwd,
    skipBackwardSeconds: skipBwd,
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

  // End-of-episode sleep timer: when the user picked "stop after this episode",
  // intercept videoEnded BEFORE useNextEpisode auto-navigates. Pause the player
  // and clear the timer so the user lands on the next-episode countdown but with
  // the auto-advance cancelled.
  useEffect(() => {
    if (sleepTimer.state?.kind !== "endOfEpisode") return;
    if (!nextEp.ended) return;
    try { player?.pause(); } catch { /* released */ }
    nextEp.cancel();
    sleepTimer.cancel();
    toast.info(t("player.sleepTimer.fired", "Sleep timer ended"));
  }, [sleepTimer, nextEp, player, t]);

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
    onRemotePause: () => { try { player?.pause(); } catch (err) { console.warn("released", err); } },
    onRemotePlay: () => { try { player?.play(); } catch (err) { console.warn("released", err); } },
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
      // timeSec is absolute; on transcode it must restart the session (see seekBy).
      if (isTranscode) {
        restartAtPosition(timeSec);
      } else if (player) {
        player.currentTime = timeSec;
      }
      resetHideTimer();
    },
    [player, isTranscode, restartAtPosition, resetHideTimer],
  );

  const handleSkipIntro = useCallback(
    (toSec: number) => {
      if (player) {
        player.currentTime = toSec;
      }
    },
    [player],
  );

  const handlePip = useCallback(async () => {
    if (!videoViewRef.current) return;
    try {
      await videoViewRef.current.startPictureInPicture();
    } catch (err) { console.warn("PiP unavailable", err); }
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

  // A local download plays regardless of the play-session network state -
  // don't gate on isError/isLoading/streamUrl when we already have bytes on
  // disk (see the videoSource useMemo above for the same rationale).
  if (isError && !localFilePath) {
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

  if (!localFilePath && (isLoading || !streamUrl)) {
    return (
      <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
        <StatusBar hidden />
        <ActivityIndicator size="large" color={theme.colors.accent} />
      </View>
    );
  }

  const seekProgress = effectiveDuration > 0 ? currentTime / effectiveDuration : 0;

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
            {playerTitle}
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
              {/* Bar is full at 100%; the >100% boost is shown by the amber fill + the % text. */}
              <View style={[
                styles.volumeBarFill,
                { height: `${Math.min(100, Math.round(volumeIndicator * 100))}%` },
                volumeIndicator > 1 && { backgroundColor: "#f59e0b" },
              ]} />
            </View>
            <Text style={[styles.volumeText, volumeIndicator > 1 && { color: "#f59e0b" }]}>{Math.round(volumeIndicator * 100)}</Text>
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
          mediaId={mediaId}
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
                {playerTitle}
              </Text>
            </View>

            <View style={[styles.bottomControls, { paddingBottom: Math.max(20, insets.bottom), paddingLeft: Math.max(20, insets.left), paddingRight: Math.max(20, insets.right) }]}>
              {/* Seek bar */}
              <View
                style={styles.seekBarContainer}
                accessible={true}
                accessibilityRole="adjustable"
                accessibilityLabel={t("player.controls.seek")}
                onLayout={(e) => { seekBarWidthRef.current = e.nativeEvent.layout.width; }}
                onStartShouldSetResponder={() => true}
                onMoveShouldSetResponder={() => true}
                onResponderGrant={(e) => {
                  const x = e.nativeEvent.locationX;
                  setSeekTouchX(x);
                  seekPctRef.current = pctFromX(x);
                  seekPreview(seekPctRef.current);
                }}
                onResponderMove={(e) => {
                  const x = e.nativeEvent.locationX;
                  setSeekTouchX(x);
                  seekPctRef.current = pctFromX(x);
                  seekPreview(seekPctRef.current);
                }}
                onResponderRelease={() => {
                  // Commit the fraction tracked during move - NOT the release
                  // event's locationX, which is unreliable on Android.
                  seekTo(seekPctRef.current);
                  setSeekTouchX(null);
                }}
              >
                {seekTouchX != null && (
                  <View style={{ position: "absolute", bottom: "50%", left: seekTouchX, transform: [{ translateX: -80 }] }}>
                    <TrickplayPreview mediaId={mediaId} currentTime={currentTime} visible={effectiveDuration > 0} />
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

              {/* Controls row - playback transport on the left, settings/track
                  pickers/cast on the right (matches the web player layout). */}
              <View style={[styles.controlsRow, { marginTop: 4 }]}>
                <Pressable onPress={() => seekBy(-skipBwd)} hitSlop={12} style={styles.controlBtn} accessible={true} accessibilityLabel={t("player.controls.rewind", { seconds: skipBwd })} accessibilityRole="button">
                  <Ionicons name="play-back" size={20} color="#fff" />
                </Pressable>
                <Pressable onPress={togglePlayPause} hitSlop={12} style={styles.controlBtn} accessible={true} accessibilityLabel={isPlaying ? t("player.controls.pause") : t("player.controls.play")} accessibilityRole="button">
                  <Ionicons name={isPlaying ? "pause" : "play"} size={24} color="#fff" />
                </Pressable>
                <Pressable onPress={() => seekBy(skipFwd)} hitSlop={12} style={styles.controlBtn} accessible={true} accessibilityLabel={t("player.controls.forward", { seconds: skipFwd })} accessibilityRole="button">
                  <Ionicons name="play-forward" size={20} color="#fff" />
                </Pressable>
                <Text style={styles.timeText}>{formatTime(currentTime)} / {formatTime(effectiveDuration)}</Text>
                <View style={{ flex: 1 }} />
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
                  onPress={() => setShowSleepTimer(true)}
                  hitSlop={8}
                  style={styles.trackButton}
                  accessible={true}
                  accessibilityLabel={t("player.settings.sleepTimer", "Sleep timer")}
                  accessibilityRole="button"
                >
                  <Ionicons
                    name={sleepTimer.state ? "moon" : "moon-outline"}
                    size={18}
                    color={sleepTimer.state ? theme.colors.accent : "#fff"}
                  />
                  {sleepTimer.state?.kind === "minutes" && sleepTimer.state.minutesLeft !== null && (
                    <Text style={[styles.speedButtonText, { color: theme.colors.accent, marginLeft: 4 }]}>
                      {sleepTimer.state.minutesLeft}
                    </Text>
                  )}
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
                <Pressable
                  onPress={() => syncPlay.state.connected ? syncPlay.leaveRoom() : setShowSyncPlay(true)}
                  hitSlop={8}
                  style={styles.trackButton}
                  accessible={true}
                  accessibilityLabel={syncPlay.state.connected ? t("media.syncplay.leave", "Leave") : t("media.syncplay.title", "Watch Together")}
                  accessibilityRole="button"
                >
                  <Ionicons name="people" size={18} color={syncPlay.state.connected ? theme.colors.accent : "#fff"} />
                </Pressable>
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
                <Pressable
                  onPress={toggleFullscreen}
                  hitSlop={8}
                  style={styles.trackButton}
                  accessible={true}
                  accessibilityLabel={isFullscreen ? t("player.controls.exitFullscreen", "Exit fullscreen") : t("player.controls.fullscreen", "Fullscreen")}
                  accessibilityRole="button"
                >
                  <Ionicons name={isFullscreen ? "contract-outline" : "expand-outline"} size={18} color="#fff" />
                </Pressable>
              </View>
            </View>
          </View>
        )}

        {syncPlay.state.connected && (
          <View style={{ position: "absolute", bottom: 0, left: 0, right: 0 }}>
            <SyncPlayBar
              roomCode={syncPlay.state.roomCode}
              isHost={syncPlay.state.isHost}
              members={syncPlay.state.members}
              connected={syncPlay.state.connected}
              onLeave={syncPlay.leaveRoom}
              queue={syncPlay.state.queue}
              trackIndex={syncPlay.state.trackIndex}
              onNext={syncPlay.sendNext}
              onPrev={syncPlay.sendPrev}
            />
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

      <SleepTimerPicker
        visible={showSleepTimer}
        onClose={() => setShowSleepTimer(false)}
        state={sleepTimer.state}
        onPickMinutes={(m) => sleepTimer.setMinutes(m)}
        onPickEndOfEpisode={() => sleepTimer.setEndOfEpisode()}
        onCancel={() => sleepTimer.cancel()}
      />

      <SyncPlayModal
        visible={showSyncPlay}
        onClose={() => setShowSyncPlay(false)}
        creating={syncCreating}
        joining={syncJoining}
        onCreateRoom={async () => {
          setSyncCreating(true);
          try {
            await syncPlay.createRoom(mediaId);
            setShowSyncPlay(false);
          } catch {
            toast.error(t("media.syncplay.createFailed", "Failed to create room"));
          } finally { setSyncCreating(false); }
        }}
        onJoinRoom={async (code) => {
          setSyncJoining(true);
          try {
            await syncPlay.joinByCode(code);
            setShowSyncPlay(false);
          } catch {
            toast.error(t("media.syncplay.joinFailed", "Failed to join room"));
          } finally { setSyncJoining(false); }
        }}
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
