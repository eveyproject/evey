import { useEffect, useState, useCallback, useRef, useMemo } from "react";
import {
  View,
  Text,
  Pressable,
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
import { FONT_SIZE, ICON_SIZE } from "@evey/theme";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { BottomSheet } from "../../../src/components/BottomSheet";
import {
  useLibraryServiceGetMediaItem,
  useLibraryServiceListMediaItems,
  usePlaybackServiceMarkAsWatched,
} from "@evey/api";
import { usePlaySession } from "../../../src/hooks/usePlaySession";
import { useProgressReporter } from "../../../src/hooks/useProgressReporter";
import { usePlayerGestures } from "../../../src/hooks/usePlayerGestures";
import { usePlaybackSettings } from "../../../src/hooks/usePlaybackSettings";
import { useNextEpisode } from "../../../src/hooks/useNextEpisode";
import { useChapters } from "../../../src/hooks/useChapters";
import { useSleepTimer } from "../../../src/hooks/useSleepTimer";
import { SleepTimerPicker } from "../../../src/components/SleepTimerPicker";
import { pickNativeAudioTrack } from "../../../src/lib/audioTrackMatch";
import { playerExit } from "../../../src/lib/playerExit";
import { ChaptersPicker } from "../../../src/components/ChaptersPicker";
import { StreamInfoPanel } from "../../../src/components/StreamInfoPanel";
import { SkipIntroOverlay } from "../../../src/components/SkipIntroOverlay";
import { CastContext } from "../../../src/hooks/useChromecast";
import { useCastSession } from "../../../src/hooks/useCastSession";
import { useSyncPlay } from "../../../src/hooks/useSyncPlay";
import { SyncPlayBar } from "../../../src/components/SyncPlayBar";
import { SyncPlayModal } from "../../../src/components/SyncPlayModal";
import { ShedScreen, ErrorScreen, LoadingScreen } from "../../../src/components/player/PlayerStatusScreens";
import { CastControllerScreen } from "../../../src/components/player/CastControllerScreen";
import { GestureIndicators } from "../../../src/components/player/GestureIndicators";
import { NextEpisodeOverlay } from "../../../src/components/player/NextEpisodeOverlay";
import { PlayerSeekBar } from "../../../src/components/player/PlayerSeekBar";
import { PlayerControlsRow } from "../../../src/components/player/PlayerControlsRow";
import { PlayerTopRightCluster } from "../../../src/components/player/PlayerTopRightCluster";
import {
  PlayerSettingsPanel,
  type AudioStream,
  type SubtitleStream,
} from "../../../src/components/player/PlayerSettingsPanel";
import { useControlsAutoHide } from "../../../src/components/player/useControlsAutoHide";

// Sheets still carried by a BottomSheet; audio / subtitles / quality / speed
// now live in the settings panel (web parity).
type SheetName = "chapters" | "streamInfo" | "sleepTimer" | "syncPlay" | "episodes";

const IMAGE_SUBTITLE_CODECS = new Set([
  "pgssub", "hdmv_pgs_subtitle", "dvd_subtitle", "dvbsub", "xsub",
]);

const WATCHED_THRESHOLD = 0.9;

export default function PlayerScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const { t } = useTranslation();
  const theme = useTheme();
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

  // Sibling episodes of the same season, for the previous/next episode
  // buttons of the transport bar (web player parity).
  const { data: siblingsData } = useLibraryServiceListMediaItems(
    { parentId: media?.parentId ?? "", "pagination.pageSize": 500 } as never,
    { query: { enabled: !!media?.parentId && isEpisode } } as never,
  );
  const { prevEpisode, nextEpisode } = useMemo(() => {
    const items = siblingsData?.items;
    if (!isEpisode || !items || items.length === 0) return { prevEpisode: null, nextEpisode: null };
    const sorted = [...items].sort((a, b) => (a.indexNumber ?? 0) - (b.indexNumber ?? 0));
    const idx = sorted.findIndex((e) => e.id === mediaId);
    return {
      prevEpisode: idx > 0 ? sorted[idx - 1] : null,
      nextEpisode: idx >= 0 && idx < sorted.length - 1 ? sorted[idx + 1] : null,
    };
  }, [siblingsData?.items, isEpisode, mediaId]);
  // Full current-season episode list for the Episodes panel (web parity's
  // EpisodesPanel). Reuses siblingsData - no extra hook. A season selector
  // would need the series' other seasons, which aren't loaded anywhere in
  // this screen; out of scope here (see dispatch note), so this panel is
  // scoped to the current season only.
  const seasonEpisodes = useMemo(() => {
    const items = siblingsData?.items;
    if (!isEpisode || !items) return [];
    return [...items].sort((a, b) => (a.indexNumber ?? 0) - (b.indexNumber ?? 0));
  }, [siblingsData?.items, isEpisode]);
  // Don't open a play session for a series/season - the redirect above will
  // remount us on a real episode in a tick. Passing "" makes usePlaySession
  // hold its initial loading state instead of hitting /transcode and 404'ing.
  const sessionMediaId = isSeriesOrSeason ? "" : mediaId;
  const { streamUrl, streamToken, sessionId, streamInfo, serverUrl, isLoading, isError, shedReason, stopSession, restartWithTracks, progress } =
    usePlaySession(sessionMediaId);

  const { data: chapters = [] } = useChapters(mediaId);
  const videoViewRef = useRef<VideoView>(null);
  const pipSupported = useMemo(() => isPictureInPictureSupported(), []);
  const mountedRef = useRef(true);
  useEffect(() => { return () => { mountedRef.current = false; void releaseAudioBoost(); }; }, []);

  const [activeSheet, setActiveSheet] = useState<null | SheetName>(null);
  const [settingsOpen, setSettingsOpen] = useState(false);
  // Read by the gestures' PanResponder (built once): a state captured in
  // its closure would be frozen, the ref always stays up to date.
  const settingsOpenRef = useRef(false);
  useEffect(() => { settingsOpenRef.current = settingsOpen; }, [settingsOpen]);
  const [selectedAudioIndex, setSelectedAudioIndex] = useState<number>(0);
  // Audio tracks the NATIVE player exposes from the HLS manifest - the source
  // of truth for a no-reload client-side switch (see handleAudioSelect).
  const [nativeAudioTracks, setNativeAudioTracks] = useState<{ id?: string; language?: string; label?: string }[]>([]);
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

  // Set initial selected audio index from default stream. Once the user has
  // manually picked a track, this must NOT run again: restartWithTracks
  // re-derives audioStreams (new reference), which re-fired this effect and
  // reset the choice back to the isDefault track - the user's language change
  // silently reverted. Mirrors the web player's userPickedAudioRef guard.
  const userPickedAudioRef = useRef(false);
  useEffect(() => {
    if (userPickedAudioRef.current) return;
    const def = audioStreams.find((s) => s.isDefault);
    if (def) setSelectedAudioIndex(def.index);
    else if (audioStreams.length > 0) setSelectedAudioIndex(audioStreams[0].index);
  }, [audioStreams]);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [isPlaying, setIsPlaying] = useState(false);
  const [isBuffering, setIsBuffering] = useState(false);
  const [volume, setVolume] = useState(1);
  const [muted, setMuted] = useState(false);
  const [brightness, setBrightness] = useState(1);
  // A fatal native failure (decoder error, 401 on a segment, broken manifest)
  // surfaces ONLY through the player's statusChange event - isError from
  // usePlaySession covers just the session-start network call. Ignoring it
  // left a black screen with live controls.
  const [nativeError, setNativeError] = useState<string | null>(null);
  // Clear a stuck native error when the media changes (auto next-episode or a
  // new navigation reuses this screen). Without this, ONE playback error latched
  // the ErrorScreen for every subsequent episode of the session, with no retry -
  // same reset discipline as autoMarkedRef / startedRef below.
  useEffect(() => {
    setNativeError(null);
    // A panel/sheet left open makes no sense on the next media
    // (auto-next-episode or a new navigation reuse this screen).
    setActiveSheet(null);
    setSettingsOpen(false);
  }, [mediaId]);
  // Whether the NEXT player instance should start playing. Changing tracks or
  // quality recreates the native player, whose setup used to call play()
  // unconditionally - a paused screen sprang back to life on every track
  // change. Handlers snapshot the playing state right before restarting;
  // defaults to true so first entry (and next-episode remounts) autoplay.
  const resumeAfterRestartRef = useRef(true);
  // While the user scrubs the timeline, freeze the live timeUpdate: otherwise it
  // overwrites the scrubbed position ~4×/s and the thumb jumps between the finger
  // and the real playback position (the "wobbly" seek).
  const seekingRef = useRef(false);
  const volumeBaseRef = useRef(1);
  const [syncCreating, setSyncCreating] = useState(false);
  const [syncJoining, setSyncJoining] = useState(false);
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
    if (resumeAfterRestartRef.current) p.play();
  });

  useEffect(() => { syncPlayRef.current = { player }; }, [player]);

  useEffect(() => {
    if (!player) return;
    const timeSub = player.addListener("timeUpdate", (ev) => {
      if (!mountedRef.current) return;
      if (seekingRef.current) return; // frozen while scrubbing - see seekingRef
      setCurrentTime(ev.currentTime);
      try { setDuration(player.duration); } catch (err) { console.warn("released", err); }
    });
    const statusSub = player.addListener("statusChange", (ev) => {
      if (!mountedRef.current) return;
      try { setIsPlaying(player.playing); setDuration(player.duration); } catch (err) { console.warn("released", err); }
      // "loading" while playing = network/segment stall: the web shows a
      // small discreet spinner in the bar, we do the same.
      setIsBuffering(ev.status === "loading");
      if (ev.status === "error") {
        console.warn("[player] native playback error:", ev.error?.message);
        setNativeError(ev.error?.message ?? "playback error");
      }
    });
    const playSub = player.addListener("playingChange", () => {
      if (!mountedRef.current) return;
      try { setIsPlaying(player.playing); } catch (err) { console.warn("released", err); }
    });
    // The native player parses the HLS audio group into availableAudioTracks
    // once the manifest loads. Switching a track is then a client-side select
    // (player.audioTrack = ...), instant and with no reload - the right
    // mechanism, decoupled from the manifest DEFAULT flag. Read the current
    // value AND subscribe to changes (it is empty until the manifest resolves).
    const readAudioTracks = () => {
      if (!mountedRef.current) return;
      try { setNativeAudioTracks(player.availableAudioTracks ?? []); } catch { /* released */ }
    };
    readAudioTracks();
    const audioTracksSub = player.addListener("availableAudioTracksChange", readAudioTracks);
    return () => {
      timeSub.remove();
      statusSub.remove();
      playSub.remove();
      audioTracksSub.remove();
    };
  }, [player]);

  const { castConnected, castClient, castCurrentTime, castDuration, castIsPlaying, handleCast } = useCastSession({
    player,
    volume,
    streamUrl,
    mediaId,
    title: media?.title,
    currentTime,
    fallbackDurationSec: media?.durationMs ? Number(media.durationMs) / 1000 : 0,
    streamInfo,
    serverUrl,
  });

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
  // Measured height of the bottom block (seekbar + row): serves as the
  // pixel anchor for the settings panel, rendered above the bar.
  const [controlsHeight, setControlsHeight] = useState(0);
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

  // Same exit contract as the web player's back button (playerUtils.playerExit):
  // pop the player's own history entry when there is one - safe because episode
  // changes REPLACE - otherwise (deep link, notification) REPLACE toward the
  // media page (an episode exits to its season) so no player entry is ever
  // left behind for a later back press to walk into.
  const handleBack = useCallback(() => {
    stopSession();
    const exit = playerExit(router.canGoBack(), mediaId, media);
    if (exit.action === "back") router.back();
    else router.replace(`/media/${exit.mediaId}` as never);
  }, [stopSession, mediaId, media]);

  useEffect(() => {
    const sub = BackHandler.addEventListener("hardwareBackPress", () => {
      handleBack();
      return true;
    });
    return () => sub.remove();
  }, [handleBack]);

  useEffect(() => {
    return () => stopSession();
  }, [stopSession]);

  // Controls stay up 4.5s after the last interaction, and never auto-hide
  // while paused or while a sheet (audio/subtitle/more/sync/...) is open -
  // see useControlsAutoHide for the timer semantics, unit-tested there.
  const { showControls, resetHideTimer, toggleControls } = useControlsAutoHide({
    isPlaying,
    hasOpenPanel: activeSheet !== null || settingsOpen,
  });

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

  // Standard resume semantics: mark the item as watched once playback
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
    if (currentTime / effectiveDuration >= WATCHED_THRESHOLD) {
      autoMarkedRef.current = true;
      markAsWatchedMut.mutate({ mediaItemId: mediaId, params: { profileId } });
    }
  }, [currentTime, effectiveDuration, progress?.played, profileId, mediaId, markAsWatchedMut]);

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
      // transcode (the event playlist is 0-based; the server produces in bursts any
      // not-yet-produced segment). Reloading the stream instead replayed it from the start.
      if (player) {
        player.currentTime = targetSec;
      }
      if (syncPlay.state.connected) syncPlay.sendSeek(targetSec * 1000);
      resetHideTimer();
    },
    [player, duration, knownDuration, resetHideTimer, syncPlay],
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
      // Dragging the bar above zero means "I want sound now": same implicit
      // unmute as the web player.
      if (v > 0 && player.muted) {
        player.muted = false;
        setMuted(false);
      }
    },
    [player],
  );

  const toggleMute = useCallback(() => {
    if (!player) return;
    try {
      player.muted = !player.muted;
      setMuted(player.muted);
    } catch (err) { console.warn("released", err); }
  }, [player]);

  const brightnessBaseRef = useRef(1);
  const handleBrightnessChange = useCallback(
    async (value: number) => {
      try {
        const Brightness = require("expo-brightness");
        await Brightness.setBrightnessAsync(value);
        brightnessBaseRef.current = value;
        setBrightness(value);
      } catch (err) { console.warn("expo-brightness not available", err); }
    },
    [],
  );
  // The settings panel's slider must start from the real system brightness,
  // not an arbitrary 100%.
  useEffect(() => {
    try {
      const Brightness = require("expo-brightness");
      Brightness.getBrightnessAsync().then((v: number) => {
        if (!mountedRef.current) return;
        brightnessBaseRef.current = v;
        setBrightness(v);
      });
    } catch (err) { console.warn("expo-brightness not available", err); }
  }, []);

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
    hasOpenPanel: () => settingsOpenRef.current,
    onCloseOverlays: () => setSettingsOpen(false),
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
    // While casting, the local player sits paused/muted (see useCastSession's
    // muteLocalPlayer) - reporting ITS position froze progress mid-cast and
    // made resume/"watched" wrong. The receiver's own position is the only
    // one moving during a cast session.
    if (castConnected) return castCurrentTime * 1000;
    try { return (player?.currentTime ?? 0) * 1000; } catch { return 0; }
  }, [player, castConnected, castCurrentTime]);

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
    onRemoteStop: () => handleBack(),
  }), [player, handleBack]);

  useProgressReporter(sessionId, getPositionMs, getIsPaused, getVolume, getIsMuted, remoteCallbacks);

  const handleAudioSelect = useCallback(
    (index: number) => {
      userPickedAudioRef.current = true;
      setSelectedAudioIndex(index);
      const stream = audioStreams.find((s) => s.index === index);
      // Prefer an instant, no-reload client-side switch on the native player -
      // the audio group is already in the manifest, so this is decoupled from
      // the DEFAULT flag and from any transcode restart. Fall back to a restart
      // only when the native tracks aren't parsed yet or don't line up.
      const nativeTrack = stream
        ? pickNativeAudioTrack(nativeAudioTracks, stream.trackIndex, stream.language)
        : null;
      if (nativeTrack && player) {
        try {
          (player as unknown as { audioTrack: unknown }).audioTrack = nativeTrack;
          return;
        } catch (err) {
          console.warn("[player] native audioTrack switch failed, restarting:", err);
        }
      }
      resumeAfterRestartRef.current = player?.playing ?? true;
      restartWithTracks(index, selectedSubtitleIndex ?? undefined, selectedQuality || undefined);
    },
    [player, nativeAudioTracks, audioStreams, restartWithTracks, selectedSubtitleIndex, selectedQuality],
  );

  const handleSubtitleSelect = useCallback(
    (index: number | null) => {
      setSelectedSubtitleIndex(index);
      resumeAfterRestartRef.current = player?.playing ?? true;
      if (index === null) {
        restartWithTracks(selectedAudioIndex, undefined, selectedQuality || undefined);
      } else {
        restartWithTracks(selectedAudioIndex, index, selectedQuality || undefined);
      }
    },
    [player, restartWithTracks, selectedAudioIndex, selectedQuality],
  );

  const handleQualitySelect = useCallback(
    (quality: number) => {
      setSelectedQuality(quality);
      resumeAfterRestartRef.current = player?.playing ?? true;
      restartWithTracks(
        selectedAudioIndex,
        selectedSubtitleIndex ?? undefined,
        quality || undefined,
      );
    },
    [player, restartWithTracks, selectedAudioIndex, selectedSubtitleIndex],
  );

  const handleChapterSeek = useCallback(
    (timeSec: number) => {
      // timeSec is absolute. Native seek on the 0-based VOD timeline, transcode
      // included - same path as seekTo/seekBy. This used to restart the whole
      // session on transcode, which replayed from 0 while the UI added an offset
      // on top, so a chapter jump reported roughly twice the real position.
      if (player) {
        player.currentTime = timeSec;
      }
      resetHideTimer();
    },
    [player, resetHideTimer],
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

  const handleSpeedSelect = useCallback((speed: number) => {
    setPlaybackSpeed(speed);
    if (player) player.playbackRate = speed;
    resetHideTimer();
  }, [player, resetHideTimer]);

  // Current chapter + previous/next chapter jumps, same logic as the web:
  // "previous" first returns to the start of the current chapter if we've
  // been in it for more than 3s, otherwise to the previous chapter.
  const currentChapterIndex = useMemo(() => {
    let idx = -1;
    const ms = currentTime * 1000;
    for (let i = 0; i < chapters.length; i++) {
      if (ms >= chapters[i].startTimeMs) idx = i;
    }
    return idx;
  }, [chapters, currentTime]);

  const skipToNextChapter = useCallback(() => {
    const nextIdx = currentChapterIndex >= 0 ? currentChapterIndex + 1 : 0;
    if (nextIdx < chapters.length) handleChapterSeek(chapters[nextIdx].startTimeMs / 1000);
  }, [chapters, currentChapterIndex, handleChapterSeek]);

  const skipToPrevChapter = useCallback(() => {
    const current = currentChapterIndex >= 0 ? chapters[currentChapterIndex] : null;
    if (!current) return;
    const elapsed = currentTime - current.startTimeMs / 1000;
    if (currentChapterIndex > 0 && elapsed <= 3) {
      handleChapterSeek(chapters[currentChapterIndex - 1].startTimeMs / 1000);
    } else {
      handleChapterSeek(current.startTimeMs / 1000);
    }
  }, [chapters, currentChapterIndex, currentTime, handleChapterSeek]);

  // Server-side load shed (HTTP 503): distinct from isError below - nothing is
  // broken, the server just isn't ready to serve THIS request yet, for one of
  // two unrelated reasons (see usePlaySession's ShedReason doc). Calm clock
  // icon (not a red error) and copy that names neither "transcoding" nor any
  // other internal term. A local download plays regardless (see below).
  if (shedReason && !localFilePath) {
    return (
      <ShedScreen
        shedReason={shedReason}
        onRetry={() => {
          // Retrying after a shed means the user wants playback back - always autoplay.
          resumeAfterRestartRef.current = true;
          restartWithTracks(selectedAudioIndex, selectedSubtitleIndex ?? undefined, selectedQuality || undefined);
        }}
        onBack={handleBack}
      />
    );
  }

  // A local download plays regardless of the play-session network state -
  // don't gate on isError/isLoading/streamUrl when we already have bytes on
  // disk (see the videoSource useMemo above for the same rationale). A native
  // error is different: it means THIS playback died (network or local file),
  // so it always surfaces.
  if (nativeError || (isError && !localFilePath)) {
    return <ErrorScreen onBack={handleBack} />;
  }

  if (!localFilePath && (isLoading || !streamUrl)) {
    return <LoadingScreen />;
  }

  const seekProgress = effectiveDuration > 0 ? currentTime / effectiveDuration : 0;

  // Casting mode: show controller instead of local player
  if (castConnected) {
    return (
      <CastControllerScreen
        title={playerTitle}
        currentTime={castCurrentTime}
        duration={castDuration}
        isPlaying={castIsPlaying}
        skipFwd={skipFwd}
        skipBwd={skipBwd}
        onSeekToPosition={(p) => castClient?.seek?.({ position: p })}
        onPlayPause={() => { if (castIsPlaying) castClient?.pause?.(); else castClient?.play?.(); }}
        onBack={handleBack}
      />
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

        <GestureIndicators
          seekFlash={seekFlash}
          volumeIndicator={volumeIndicator}
          brightnessIndicator={brightnessIndicator}
          skipFwd={skipFwd}
          skipBwd={skipBwd}
        />

        {/* Skip intro */}
        <SkipIntroOverlay
          mediaId={mediaId}
          filePath={media?.filePath}
          currentTime={currentTime}
          onSkip={handleSkipIntro}
        />

        {nextEp.visible && nextEp.nextEpisode && <NextEpisodeOverlay nextEp={nextEp} />}

        {/* Invisible scrim under the settings panel: a tap outside the panel
            closes it (equivalent of the web's handleOverlayClick). The
            player's gestures are suspended while the panel is open. */}
        {settingsOpen && (
          <Pressable
            style={StyleSheet.absoluteFill}
            onPress={() => setSettingsOpen(false)}
            accessible={false}
          />
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

            {/* Top-right cluster: watch-together, then cast (matches the web
                player layout) - fades with the rest of the controls. */}
            <PlayerTopRightCluster
              syncConnected={syncPlay.state.connected}
              castConnected={castConnected}
              castAvailable={!!CastContext}
              onCast={() => { handleCast(); resetHideTimer(); }}
              onSyncPress={() => {
                if (syncPlay.state.connected) syncPlay.leaveRoom();
                else setActiveSheet("syncPlay");
                resetHideTimer();
              }}
              style={{ paddingTop: Math.max(16, insets.top), paddingRight: Math.max(20, insets.right) }}
            />

            <View
              style={[styles.bottomControls, { paddingBottom: Math.max(20, insets.bottom), paddingLeft: Math.max(20, insets.left), paddingRight: Math.max(20, insets.right) }]}
              onLayout={(e) => setControlsHeight(e.nativeEvent.layout.height)}
            >
              <PlayerSeekBar
                mediaId={mediaId}
                progress={seekProgress}
                currentTime={currentTime}
                effectiveDuration={effectiveDuration}
                chapters={chapters}
                onPreview={seekPreview}
                onCommit={seekTo}
                onScrubbingChange={(s) => { seekingRef.current = s; }}
              />

              {/* Transport row, copy of the web bar: episodes/chapters,
                  +/-Ns, time, volume, then "..." toward the secondary row. */}
              <View style={styles.transportRow}>
                <View style={{ flex: 1 }}>
                  <PlayerControlsRow
                    isPlaying={isPlaying}
                    isBuffering={isBuffering}
                    currentTime={currentTime}
                    effectiveDuration={effectiveDuration}
                    skipFwd={skipFwd}
                    skipBwd={skipBwd}
                    isFullscreen={isFullscreen}
                    volume={volume}
                    muted={muted}
                    prevEpisode={prevEpisode?.id ? { id: prevEpisode.id } : null}
                    nextEpisode={nextEpisode?.id ? { id: nextEpisode.id } : null}
                    hasPrevChapter={chapters.length > 0 && currentChapterIndex > 0}
                    hasNextChapter={chapters.length > 0 && currentChapterIndex >= 0 && currentChapterIndex < chapters.length - 1}
                    hasChapters={chapters.length > 0}
                    settingsOpen={settingsOpen}
                    sleepTimerActive={sleepTimer.state !== null}
                    pipSupported={pipSupported}
                    onSeekBy={seekBy}
                    onTogglePlayPause={togglePlayPause}
                    onToggleMute={toggleMute}
                    onVolumeChange={handleVolumeChange}
                    onEpisodeChange={handleNavigateEpisode}
                    onPrevChapter={skipToPrevChapter}
                    onNextChapter={skipToNextChapter}
                    onOpenChapters={() => setActiveSheet("chapters")}
                    onToggleSettings={() => { setSettingsOpen((v) => !v); resetHideTimer(); }}
                    onOpenStreamInfo={() => setActiveSheet("streamInfo")}
                    onOpenSleepTimer={() => setActiveSheet("sleepTimer")}
                    onPip={handlePip}
                    onToggleFullscreen={toggleFullscreen}
                  />
                </View>

                {/* Episodes panel entry point (web parity: PlayerEpisodesPanel).
                    Only current-season episodes are listed here - see the
                    seasonEpisodes comment above for why there's no season
                    selector. */}
                {seasonEpisodes.length > 1 && (
                  <Pressable
                    onPress={() => { setActiveSheet("episodes"); resetHideTimer(); }}
                    hitSlop={8}
                    style={styles.episodesBtn}
                    accessible={true}
                    accessibilityLabel={t("player.controls.episodes", "Épisodes")}
                    accessibilityRole="button"
                  >
                    <Ionicons name="list" size={ICON_SIZE.lg} color="#fff" />
                  </Pressable>
                )}
              </View>

            </View>

            {/* Settings panel anchored above the bar, right-aligned - same
                popup as the web player's gear icon. Sibling of the bar and
                positioned in pixels: a percentage bottom broke the
                ScrollView's measured height (panel drawn but the touch
                area shrunk to a few pixels). */}
            {settingsOpen && (
              <PlayerSettingsPanel
                audioStreams={audioStreams}
                currentAudioIndex={selectedAudioIndex}
                onAudioChange={handleAudioSelect}
                subtitleStreams={subtitleStreams}
                currentSubtitleIndex={selectedSubtitleIndex}
                onSubtitleChange={handleSubtitleSelect}
                currentQuality={selectedQuality}
                onQualityChange={handleQualitySelect}
                playbackSpeed={playbackSpeed}
                onSpeedChange={handleSpeedSelect}
                brightness={brightness}
                onBrightnessChange={handleBrightnessChange}
                onClose={() => setSettingsOpen(false)}
                style={{
                  position: "absolute",
                  right: Math.max(20, insets.right),
                  bottom: controlsHeight + 8,
                }}
              />
            )}
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

      <ChaptersPicker
        visible={activeSheet === "chapters"}
        onClose={() => setActiveSheet(null)}
        chapters={chapters}
        currentTimeMs={currentTime * 1000}
        onSeek={handleChapterSeek}
      />

      <BottomSheet
        visible={activeSheet === "episodes"}
        onClose={() => setActiveSheet(null)}
        title={t("player.controls.episodes", "Épisodes")}
      >
        {seasonEpisodes.map((ep) => {
          const isActive = ep.id === mediaId;
          return (
            <Pressable
              key={ep.id}
              style={[styles.episodeRow, { borderBottomColor: theme.colors.border }]}
              onPress={() => { handleNavigateEpisode(ep.id); setActiveSheet(null); }}
              accessible={true}
              accessibilityLabel={ep.title || `E${ep.indexNumber ?? "?"}`}
              accessibilityRole="button"
              accessibilityState={{ selected: isActive }}
            >
              <Text style={[styles.episodeIndex, { color: theme.colors.textMuted }]}>
                E{ep.indexNumber ?? "?"}
              </Text>
              <Text
                style={[styles.episodeTitle, { color: isActive ? theme.colors.accent : theme.colors.text }]}
                numberOfLines={1}
              >
                {ep.title}
              </Text>
              {isActive && <Ionicons name="checkmark" size={ICON_SIZE.sm} color={theme.colors.accent} />}
            </Pressable>
          );
        })}
      </BottomSheet>

      <StreamInfoPanel
        visible={activeSheet === "streamInfo"}
        onClose={() => setActiveSheet(null)}
        streamInfo={streamInfo}
      />

      <SleepTimerPicker
        visible={activeSheet === "sleepTimer"}
        onClose={() => setActiveSheet(null)}
        state={sleepTimer.state}
        onPickMinutes={(m) => sleepTimer.setMinutes(m)}
        onPickEndOfEpisode={() => sleepTimer.setEndOfEpisode()}
        onCancel={() => sleepTimer.cancel()}
      />

      <SyncPlayModal
        visible={activeSheet === "syncPlay"}
        onClose={() => setActiveSheet(null)}
        creating={syncCreating}
        joining={syncJoining}
        onCreateRoom={async () => {
          setSyncCreating(true);
          try {
            await syncPlay.createRoom(mediaId);
            setActiveSheet(null);
          } catch {
            toast.error(t("media.syncplay.createFailed", "Failed to create room"));
          } finally { setSyncCreating(false); }
        }}
        onJoinRoom={async (code) => {
          setSyncJoining(true);
          try {
            await syncPlay.joinByCode(code);
            setActiveSheet(null);
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
  transportRow: {
    flexDirection: "row",
    alignItems: "center",
  },
  episodesBtn: {
    padding: 4,
    marginLeft: 4,
  },
  episodeRow: {
    flexDirection: "row",
    alignItems: "center",
    paddingVertical: 14,
    borderBottomWidth: StyleSheet.hairlineWidth,
    gap: 10,
  },
  episodeIndex: {
    fontSize: FONT_SIZE.xs,
    fontVariant: ["tabular-nums"],
    width: 32,
  },
  episodeTitle: {
    fontSize: FONT_SIZE.sm,
    flex: 1,
  },
});
