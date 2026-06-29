import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useAuthStore } from "@evey/store";
import { useUserServiceGetCurrentUser, useUserServiceListProfiles } from "@evey/api";
import { RotateCcw, RotateCw, Play, Pause, Loader, TriangleAlert, Volume2, Sun } from "lucide-react";
import Hls from "hls.js";
import type { V1StreamInfo } from "@evey/api";
import type { AudioStream } from "./AudioTrackSelector";
import type { SubtitleStream } from "./SubtitleSelector";
import { PlayerControls } from "./PlayerControls";
import type { EpisodeNavItem, SeasonNavItem } from "./PlayerControls";
import { PlayerProvider } from "./PlayerContext";
import { nominalQualityTier } from "./playerUtils";
import { ProgressReporter } from "./ProgressReporter";
import { AutoMarkWatched } from "./AutoMarkWatched";
import { NextEpisodeOverlay } from "./NextEpisodeOverlay";
import { SkipIntroButton } from "./SkipIntroButton";
import { KeyboardShortcutsOverlay } from "./KeyboardShortcutsOverlay";
import { DownloadButton } from "./DownloadButton";
import type { Chapter } from "@/hooks/useChapters";
import { usePlaybackSettings } from "@/hooks/usePlaybackSettings";
import {
  getPreferredAudioLang,
  setPreferredAudioLang,
  getPreferredSubtitleLang,
  setPreferredSubtitleLang,
  getSeriesTrackPrefs,
  setSeriesTrackPrefs,
  getSubtitleStyle,
  setSubtitleStyle,
  type SubtitleStyle,
} from "@/hooks/usePlaybackPrefs";
import { buildTranscodeHlsUrl, getSelectedBurnInTrack, resolveDefaultAudio, resolveDefaultSubtitle } from "./hooks/hlsPlayerUtils";
import { useDirectPlayback } from "./hooks/useDirectPlayback";
import { useTranscodePlayback } from "./hooks/useTranscodePlayback";
import { useSubtitleManager } from "./hooks/useSubtitleManager";
import { usePlayerKeyboard } from "./hooks/usePlayerKeyboard";
import { useTouchGestures } from "./hooks/useTouchGestures";
import { useOverlayTime } from "./hooks/useOverlayTime";
import { useDLNA } from "@/hooks/useDLNA";

export type { AudioStream, SubtitleStream, EpisodeNavItem, SeasonNavItem };

// How long playback must be CONTINUOUSLY stalled before the buffering overlay
// appears. Micro-stalls (segment boundaries, fast seeks, rendition switches)
// resolve well under this and must not flash a spinner over a playing video;
// genuine stalls (cold transcode start on a weak host, network loss) exceed it
// and still get user feedback.
const BUFFERING_SPINNER_DELAY_MS = 1500;

interface HLSPlayerProps {
  streamInfo: V1StreamInfo;
  sessionId: string;
  mediaId: string;
  mediaType?: string;
  streamToken: string;
  onStreamTokenRefresh: () => Promise<string>;
  // Mints a long-lived token for Chromecast playback (the device can't
  // re-validate the short sliding token mid-stream). Optional — without
  // it the cast falls back to the regular session token.
  onCastTokenRequest?: () => Promise<string>;
  startPositionMs?: number;
  knownDurationMs?: number;
  filePath?: string;
  onEnded?: () => void;
  audioStreams?: AudioStream[];
  subtitleStreams?: SubtitleStream[];
  title?: string;
  onClose?: () => void;
  seasons?: SeasonNavItem[];
  currentSeasonId?: string;
  episodes?: EpisodeNavItem[];
  currentEpisodeId?: string;
  onSeasonChange?: (id: string) => void;
  onEpisodeChange?: (id: string) => void;
  chapters?: Chapter[];
  seriesId?: string;
  onVideoRef?: (el: HTMLVideoElement | null) => void;
}

export function HLSPlayer({
  streamInfo,
  sessionId,
  mediaId,
  mediaType,
  streamToken,
  onStreamTokenRefresh,
  onCastTokenRequest,
  startPositionMs = 0,
  knownDurationMs,
  filePath,
  onEnded,
  audioStreams = [],
  subtitleStreams = [],
  title,
  onClose,
  seasons,
  currentSeasonId,
  episodes,
  currentEpisodeId,
  onSeasonChange,
  onEpisodeChange,
  chapters = [],
  seriesId,
  onVideoRef,
}: HLSPlayerProps) {
  const { t } = useTranslation();
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const containerRef = useRef<HTMLDivElement | null>(null);
  const hlsRef = useRef<InstanceType<typeof Hls> | null>(null);
  const controlsToggleRef = useRef<(() => void) | null>(null);
  // Set by PlayerControls to its Document-PiP-aware toggle, so the "p" keyboard
  // shortcut opens the same rich PiP window as the on-screen button instead of
  // the bare-<video> legacy PiP.
  const pipToggleRef = useRef<(() => void) | null>(null);
  const positionRef = useRef(startPositionMs / 1000);
  const knownDurationSecRef = useRef(knownDurationMs ? knownDurationMs / 1000 : 0);
  knownDurationSecRef.current = knownDurationMs ? knownDurationMs / 1000 : 0;
  // Only direct_play streams the original file progressively. Everything else —
  // transcode AND direct_stream (a container-only remux served by the HLS copy
  // rung) — runs through the HLS pipeline (useTranscodePlayback), which owns
  // stream-token injection, 401 refresh and CMCD. direct_stream used to be
  // lumped with direct_play and silently played the original (often undecodable)
  // container; routing it through HLS is what makes the remux actually play.
  const [transcodeMode, setTranscodeMode] = useState(
    streamInfo.playMethod !== "PLAY_METHOD_DIRECT_PLAY"
  );

  // Fetch the active profile to read audio/subtitle language preferences.
  const { data: currentUser } = useUserServiceGetCurrentUser();
  const activeProfileId = useAuthStore((s) => s.profileId);
  const { data: profilesResp } = useUserServiceListProfiles(
    currentUser?.id ?? "",
    { query: { enabled: !!currentUser?.id } },
  );
  const activeProfile = useMemo(
    () =>
      profilesResp?.profiles?.find((p) => p.id === activeProfileId) ??
      profilesResp?.profiles?.find((p) => p.isDefault) ??
      profilesResp?.profiles?.[0],
    [profilesResp, activeProfileId],
  );

  // Build ordered language lists from profile.preferences, falling back to
  // locale as a last resort (e.g. profile with locale "fr" but no explicit
  // audio_languages still prefers French audio).
  const profileAudioLangs = useMemo((): string[] => {
    const raw = activeProfile?.preferences?.["audio_languages"] ?? "";
    const fromPrefs = raw.split(",").map((s) => s.trim()).filter(Boolean);
    const locale = activeProfile?.locale ?? "";
    // Append locale as final fallback only if not already in the list.
    if (locale && !fromPrefs.includes(locale)) return [...fromPrefs, locale];
    return fromPrefs;
  }, [activeProfile]);

  const profileSubtitleLangs = useMemo((): string[] => {
    const raw = activeProfile?.preferences?.["subtitle_languages"] ?? "";
    return raw.split(",").map((s) => s.trim()).filter(Boolean);
  }, [activeProfile]);

  // Initialize audio/subtitle from streams + whatever prefs are available at
  // first render (profile may not be loaded yet — profileAudioLangs is empty).
  const defaultAudio = resolveDefaultAudio(
    audioStreams,
    seriesId,
    getSeriesTrackPrefs,
    getPreferredAudioLang,
    profileAudioLangs.length > 0 ? profileAudioLangs : undefined,
  );
  const [currentAudioIndex, setCurrentAudioIndex] = useState(defaultAudio?.index ?? 0);
  const [currentTrackIndex, setCurrentTrackIndex] = useState(defaultAudio?.trackIndex ?? 0);
  const [currentQuality, setCurrentQuality] = useState(0);
  const [autoQuality, setAutoQuality] = useState(true);

  // Resolved audio language — used for forced subtitle matching.
  const defaultAudioLang = defaultAudio?.language ?? defaultAudio?.title;
  const [currentSubtitle, setCurrentSubtitle] = useState<number | null>(() =>
    resolveDefaultSubtitle(
      subtitleStreams,
      seriesId,
      getSeriesTrackPrefs,
      getPreferredSubtitleLang,
      profileSubtitleLangs.length > 0 ? profileSubtitleLangs : undefined,
      defaultAudioLang,
    ),
  );

  // Track whether the user has made a manual track selection in this session.
  // If so, we must not override with profile prefs when they load asynchronously.
  const userPickedAudioRef = useRef(false);
  const userPickedSubtitleRef = useRef(false);

  // When the profile finishes loading, apply its audio/subtitle preferences if
  // the user hasn't picked a track manually and the initial resolution used no
  // profile data (i.e. profileAudioLangs was empty at mount time).
  const profileLoadedRef = useRef(false);
  useEffect(() => {
    if (profileLoadedRef.current) return;
    if (profileAudioLangs.length === 0 && profileSubtitleLangs.length === 0) return;
    profileLoadedRef.current = true;

    if (!userPickedAudioRef.current) {
      const best = resolveDefaultAudio(
        audioStreams,
        seriesId,
        getSeriesTrackPrefs,
        getPreferredAudioLang,
        profileAudioLangs.length > 0 ? profileAudioLangs : undefined,
      );
      if (best && best.index !== currentAudioIndex) {
        setCurrentAudioIndex(best.index);
        setCurrentTrackIndex(best.trackIndex);
      }
    }

    if (!userPickedSubtitleRef.current) {
      const currentAudioStream = audioStreams.find((s) => s.index === currentAudioIndex);
      const audioLang = currentAudioStream?.language ?? currentAudioStream?.title;
      const bestSub = resolveDefaultSubtitle(
        subtitleStreams,
        seriesId,
        getSeriesTrackPrefs,
        getPreferredSubtitleLang,
        profileSubtitleLangs.length > 0 ? profileSubtitleLangs : undefined,
        audioLang,
      );
      if (bestSub !== currentSubtitle) {
        setCurrentSubtitle(bestSub);
      }
    }
  // Intentionally omitting currentAudioIndex/currentSubtitle to avoid re-running
  // after the state updates triggered by this same effect.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [profileAudioLangs, profileSubtitleLangs]);

  const selectedBurnInTrack = getSelectedBurnInTrack(currentSubtitle, subtitleStreams);
  const {
    castSdkReady, connected: castConnected, deviceName: castDeviceName,
    castPlayback, startCast, stopCast, castPlayPause, castSeek, castReload, castSetVolume, castSetMuted,
  } = useDLNA();
  // Cast button visibility. The native Google Cast SDK (castSdkReady)
  // works for any user on Chrome/Chromium. The server-side DLNA/cast
  // path only discovers devices on the *server's* LAN, so it's only
  // meaningful for the admin (the operator, guaranteed-local) — exposing
  // it to remote users just yields a confusing empty device list.
  const isAdmin = useAuthStore((s) => s.isAdmin);
  const castAvailable = castSdkReady || isAdmin;

  const handleCastToggle = useCallback(async () => {
    if (castConnected) {
      stopCast();
      return;
    }
    // Build a URL the Chromecast can fetch independently (absolute, with token)
    let url: string;
    // Resume the cast at the local player's current ABSOLUTE position rather
    // than restarting from 0. Only applies to transcode (the server bakes the
    // start offset into the HLS stream); direct play hands the receiver the
    // full file, which it can seek on its own.
    let castStartSec = 0;
    if (streamInfo.directPlayUrl) {
      // Direct play — append token if not already present
      const hasToken = streamInfo.directPlayUrl.includes("token=");
      url = hasToken ? streamInfo.directPlayUrl : `${streamInfo.directPlayUrl}${streamInfo.directPlayUrl.includes("?") ? "&" : "?"}token=${encodeURIComponent(streamTokenRef.current)}`;
    } else {
      // Transcode — HLS master playlist, started at the current position.
      castStartSec = positionRef.current > 0 ? Math.floor(positionRef.current) : 0;
      url = buildTranscodeHlsUrl(mediaId, streamTokenRef.current, currentTrackIndex, null, castStartSec);
    }
    // Swap in a long-lived cast token. The Chromecast fetches segments on
    // its own and buffers minutes ahead, then goes quiet — the regular
    // 5-minute sliding token expires during those quiet stretches and the
    // cast drops mid-film. The local player dodges this via xhrSetup; the
    // device can't, so it needs a token that outlives the whole session.
    if (onCastTokenRequest) {
      try {
        const castTok = await onCastTokenRequest();
        const u = new URL(url, window.location.origin);
        u.searchParams.set("token", castTok);
        url = `${u.pathname}?${u.searchParams.toString()}`;
      } catch (err) {
        console.warn("[Cast] cast token request failed, using session token:", err);
      }
    }
    if (import.meta.env.DEV) console.log("[Cast] handleCastToggle — url:", url, "mode:", streamInfo.directPlayUrl ? "direct" : "transcode");
    // Remember the exact negotiated URL so handleCastSeek can reload at a
    // new startSec without reconstructing it (the server picks /transcode
    // vs /hls/master.m3u8 + codec params during negotiation — rebuilding
    // by hand would diverge).
    castBaseUrlRef.current = url;
    castTranscodeOffsetRef.current = castStartSec;
    startCast(url, title);
  }, [castConnected, stopCast, startCast, streamInfo.directPlayUrl, title, mediaId, currentTrackIndex, onCastTokenRequest]);

  const [playbackSpeed, setPlaybackSpeed] = useState(1);
  const anyPanelOpenRef = useRef(false);
  const closePanelRef = useRef<(() => void) | null>(null);
  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;
  const [externalSubUrl, setExternalSubUrl] = useState<{ url: string; label: string } | null>(null);
  const [subtitleStyle, setSubtitleStyleState] = useState<SubtitleStyle>(() => getSubtitleStyle());

  function updateSubtitleStyle(patch: Partial<SubtitleStyle>) {
    setSubtitleStyleState((prev) => {
      const next = { ...prev, ...patch };
      setSubtitleStyle(next);
      return next;
    });
  }
  const [showShortcuts, setShowShortcuts] = useState(false);
  const [isBuffering, setIsBuffering] = useState(false);
  const [isPlaying, setIsPlaying] = useState(false);
  // Debounced gate in front of setIsBuffering: the raw `waiting` event fires on
  // every micro-stall (segment boundary, fast seek, ABR switch) and flashing
  // the spinner over a video that resumes within a second reads as "the app is
  // struggling" when nothing is wrong. Only surface the overlay when buffering
  // PERSISTS past the threshold; hide immediately on `playing`. Stable identity
  // (useCallback []) is required: both playback hooks exclude the setter from
  // their effect deps on that assumption.
  const bufferingTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const setIsBufferingDebounced = useCallback((v: boolean) => {
    if (v) {
      if (bufferingTimerRef.current !== null) return; // already counting down
      bufferingTimerRef.current = setTimeout(() => {
        bufferingTimerRef.current = null;
        setIsBuffering(true);
      }, BUFFERING_SPINNER_DELAY_MS);
      return;
    }
    if (bufferingTimerRef.current !== null) {
      clearTimeout(bufferingTimerRef.current);
      bufferingTimerRef.current = null;
    }
    setIsBuffering(false);
  }, []);
  useEffect(() => () => {
    if (bufferingTimerRef.current !== null) clearTimeout(bufferingTimerRef.current);
  }, []);
  const [fatalError, setFatalError] = useState(false);
  const wasPausedRef = useRef(false);
  const playbackSpeedRef = useRef(playbackSpeed);
  playbackSpeedRef.current = playbackSpeed;
  const subtitleStreamsRef = useRef(subtitleStreams);
  subtitleStreamsRef.current = subtitleStreams;
  const currentSubtitleRef = useRef(currentSubtitle);
  currentSubtitleRef.current = currentSubtitle;
  const streamTokenRef = useRef(streamToken);
  streamTokenRef.current = streamToken;
  const onStreamTokenRefreshRef = useRef(onStreamTokenRefresh);
  onStreamTokenRefreshRef.current = onStreamTokenRefresh;
  const { data: playbackSettings } = usePlaybackSettings();
  const skipFwd = playbackSettings?.skipForwardSeconds ?? 30;
  const skipBwd = playbackSettings?.skipBackwardSeconds ?? 30;
  const skipFwdRef = useRef(skipFwd);
  skipFwdRef.current = skipFwd;
  const skipBwdRef = useRef(skipBwd);
  skipBwdRef.current = skipBwd;
  const transcodeOffsetRef = useRef(0);
  // Cast-side mirror of transcodeOffsetRef: an HLS cast plays a manifest
  // built at startSec=X, so the device's reported currentTime is local to
  // that stream and the absolute position is castTranscodeOffset + it.
  const castTranscodeOffsetRef = useRef(0);
  // The exact URL handed to the cast device, captured at cast start so a
  // seek can reload it at a new startSec without re-deriving the endpoint.
  const castBaseUrlRef = useRef("");

  const isDirectPlay = streamInfo.playMethod === "PLAY_METHOD_DIRECT_PLAY";

  // --- Hooks ---

  useDirectPlayback({
    videoRef, positionRef, streamInfo, startPositionMs,
    transcodeMode, onEnded,
    setTranscodeMode, setIsBuffering: setIsBufferingDebounced,
    streamTokenRef, onStreamTokenRefreshRef,
  });

  const { transcodeStartTime, setTranscodeStartTime, hlsLevels, autoLevelHeight } = useTranscodePlayback({
    videoRef, hlsRef, positionRef, transcodeOffsetRef,
    wasPausedRef, streamTokenRef, onStreamTokenRefreshRef,
    transcodeMode, mediaId, currentTrackIndex, selectedBurnInTrack,
    startPositionMs, onEnded, setIsBuffering: setIsBufferingDebounced, setFatalError,
  });

  useSubtitleManager({
    videoRef, currentSubtitle, externalSubUrl, mediaId, streamToken,
    selectedBurnInTrack, transcodeStartTime, currentTrackIndex, subtitleStyle,
  });

  // Sync playbackRate
  useEffect(() => {
    const video = videoRef.current;
    if (video) video.playbackRate = playbackSpeed;
  }, [playbackSpeed]);

  const [actionFlash, setActionFlash] = useState<"play" | "pause" | null>(null);
  const actionFlashTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  function showActionFlash(action: "play" | "pause") {
    if (actionFlashTimer.current) clearTimeout(actionFlashTimer.current);
    setActionFlash(action);
    actionFlashTimer.current = setTimeout(() => setActionFlash(null), 500);
  }

  const handleSeekRequest = useCallback((absoluteTimeSec: number) => {
    const video = videoRef.current;
    if (!video) return;
    // VOD pipeline (transcode + direct): the manifest covers the full
    // media duration. SegmentOrchestrator spawns a rafale at the target
    // segment with `-copyts`, so hls.js can seek via video.currentTime
    // and the served fragment's tfdt lines up with the EXTINF position.
    // No more sliding-window restart branch — that legacy code computed
    // currentTime relative to a transcodeOffset the new pipeline doesn't
    // carry, and produced UI/playhead drift on resume.
    video.currentTime = absoluteTimeSec;
  }, []);

  const seekRef = useRef(handleSeekRequest);
  seekRef.current = handleSeekRequest;

  // Cast seek. Direct-play casts stream a complete file the device can
  // seek natively. HLS / remux casts play a progressively-built manifest,
  // so seeking outside its window means reloading the stream at
  // startSec=target on the device — same idea as setTranscodeStartTime
  // for the local player. We reuse the exact negotiated cast URL and only
  // overwrite startSec, so the endpoint (/transcode vs /hls/master.m3u8)
  // and codec params stay whatever the server picked.
  const handleCastSeek = useCallback((absoluteTimeSec: number): boolean => {
    if (isDirectPlay) {
      return castSeek(absoluteTimeSec);
    }
    const base = castBaseUrlRef.current;
    if (!base) {
      // No captured URL (shouldn't happen once casting) — best effort.
      return castSeek(absoluteTimeSec);
    }
    const u = new URL(base, window.location.origin);
    u.searchParams.set("startSec", String(Math.floor(absoluteTimeSec)));
    castTranscodeOffsetRef.current = absoluteTimeSec;
    void castReload(`${u.pathname}?${u.searchParams.toString()}`, title);
    return true;
  }, [isDirectPlay, castSeek, castReload, title]);

  // Canonical subtitle selection — shared by the settings panel picker and the
  // keyboard "c" shortcut. Crucially, when toggling a burn-in track (PGS/VobSub)
  // in transcode mode it restarts the transcode from the current position so the
  // server re-encodes with the subtitle burned in. The raw setCurrentSubtitle
  // setter alone flips the state without re-encoding, so a burn-in subtitle
  // picked via "c" never actually appeared.
  const handleSubtitleChange = useCallback((trackIndex: number | null) => {
    userPickedSubtitleRef.current = true;
    const prevBurnIn = selectedBurnInTrack;
    const nextSub = trackIndex !== null ? subtitleStreams.find((st) => st.trackIndex === trackIndex) : null;
    const nextBurnIn = nextSub?.burnIn ? trackIndex : null;

    setCurrentSubtitle(trackIndex);

    if (transcodeMode && prevBurnIn !== nextBurnIn) {
      setTranscodeStartTime(positionRef.current);
    }

    if (trackIndex === null) {
      setPreferredSubtitleLang(null);
      if (seriesId) {
        const cur = getSeriesTrackPrefs(seriesId) ?? {};
        setSeriesTrackPrefs(seriesId, { ...cur, subtitleLang: null });
      }
    } else {
      const lang = nextSub?.language ?? nextSub?.title ?? null;
      setPreferredSubtitleLang(lang);
      if (seriesId) {
        const cur = getSeriesTrackPrefs(seriesId) ?? {};
        setSeriesTrackPrefs(seriesId, { ...cur, subtitleLang: lang });
      }
    }
  }, [selectedBurnInTrack, subtitleStreams, transcodeMode, setCurrentSubtitle, setTranscodeStartTime, setPreferredSubtitleLang, seriesId, getSeriesTrackPrefs, setSeriesTrackPrefs]);

  usePlayerKeyboard({
    videoRef, positionRef, knownDurationSecRef, seekRef, skipFwdRef, skipBwdRef,
    playbackSpeedRef, subtitleStreamsRef, currentSubtitleRef, anyPanelOpenRef,
    closePanelRef, onCloseRef, setPlaybackSpeed, onSubtitleChange: handleSubtitleChange,
    setShowShortcuts, showActionFlash, pipToggleRef,
  });

  const { overlayTime, overlayDuration } = useOverlayTime({
    videoRef, transcodeOffsetRef, knownDurationSecRef,
  });

  const toggleFullscreen = useCallback(() => {
    const el = containerRef.current;
    if (!el) return;
    if (!document.fullscreenElement) void el.requestFullscreen();
    else void document.exitFullscreen();
  }, []);

  const handleSingleTap = useCallback(() => {
    if (controlsToggleRef.current) controlsToggleRef.current();
  }, []);

  const { seekFlash, brightness, setBrightnessValue, swipeIndicator, handleTouchStart, handleTouchMove, handleTouchEnd, handleWheel, cleanupTimers } = useTouchGestures({
    videoRef, positionRef, knownDurationSecRef, seekRef, skipFwdRef, skipBwdRef, onToggleFullscreen: toggleFullscreen, onSingleTap: handleSingleTap,
  });

  // Auto-pause local player when casting, resume when cast stops
  const wasPlayingBeforeCast = useRef(false);
  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    if (castConnected) {
      wasPlayingBeforeCast.current = !video.paused;
      if (!video.paused) video.pause();
    } else if (wasPlayingBeforeCast.current) {
      video.play().catch((err) => console.warn("video.play failed", err));
      wasPlayingBeforeCast.current = false;
    }
  }, [castConnected]);

  // Sync isPlaying from the video element so the buffering overlay can
  // distinguish "stall while playing" (show only discrete spinner) from
  // "stall while paused/not-yet-started" (show full-screen overlay).
  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    const onPlay = () => setIsPlaying(true);
    const onPause = () => setIsPlaying(false);
    const onEnded = () => setIsPlaying(false);
    video.addEventListener("play", onPlay);
    video.addEventListener("pause", onPause);
    video.addEventListener("ended", onEnded);
    setIsPlaying(!video.paused);
    return () => {
      video.removeEventListener("play", onPlay);
      video.removeEventListener("pause", onPause);
      video.removeEventListener("ended", onEnded);
    };
  // videoRef is a stable ref object; the effect needs to run once on mount.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Auto-fullscreen on landscape rotation (touch devices)
  useEffect(() => {
    if (!("ontouchstart" in window) || typeof window.matchMedia !== "function") return;
    const mql = window.matchMedia("(orientation: landscape)");
    const handler = (e: MediaQueryListEvent) => {
      const container = containerRef.current;
      if (!container) return;
      if (e.matches && !document.fullscreenElement) {
        void container.requestFullscreen().catch((err) => console.warn("requestFullscreen failed", err));
      }
    };
    mql.addEventListener("change", handler);
    return () => mql.removeEventListener("change", handler);
  }, []);

  // Cleanup timers on unmount
  useEffect(() => {
    return () => {
      if (actionFlashTimer.current) clearTimeout(actionFlashTimer.current);
      cleanupTimers();
    };
  }, []);

  // Sync hls.js level with autoQuality / currentQuality state
  useEffect(() => {
    const hls = hlsRef.current;
    if (!hls || !transcodeMode) return;
    // hls.js ABR is intentionally OFF on this transcode path: free ABR loads
    // several rungs at once on a transcode box (each = a concurrent decode), they
    // contend, time out, and it collapses to the 360p floor. Setting currentLevel
    // to a fixed index pins one rung (and disables ABR). "Auto" pins the TOP
    // (calibrated-sustainable) rung — one decode, stable, no switching — and the
    // viewer drops manually via the picker for a title too heavy to hold it.
    if (autoQuality) {
      hls.currentLevel = hls.levels.length > 0 ? hls.levels.length - 1 : -1;
    } else if (currentQuality > 0) {
      const idx = hls.levels.findIndex((l: { height: number }) => l.height === currentQuality);
      if (idx >= 0) hls.currentLevel = idx;
    } else {
      hls.currentLevel = hls.levels.length > 0 ? hls.levels.length - 1 : -1;
    }
    // hlsLevels MUST be a dep: this effect's mount run happens BEFORE hls.js
    // has parsed the manifest (hls.levels is empty → currentLevel=-1, which
    // ENABLES free ABR instead of pinning). Nothing else re-runs the effect
    // afterwards, so on a multi-rung ladder (the hardware path) playback was
    // left on free ABR — just-in-time segment production wrecks its bandwidth
    // estimate (multi-second TTFB on cache misses) and it collapses to the
    // 360p floor: the "Auto downgrades hard on HW transcode" bug. The single-
    // rendition software path masked it (one level = nowhere to fall).
    // hlsLevels is set in MANIFEST_PARSED, so depending on it re-runs the pin
    // exactly when the levels exist.
  }, [autoQuality, currentQuality, transcodeMode, hlsLevels]);

  // In Auto, surface the rung ABR actually picked (e.g. "1080p") so the menu
  // shows "Auto (1080p)" rather than a redundant "Auto". Empty until the first
  // level switch (panel then shows a bare "Auto").
  // Label the rung by its nearest standard tier, not the raw encoded height: a
  // 2.40:1 master's "1080p" rung is 1920×798, and "798p" reads as a bug.
  const autoQualityLabel = autoQuality && autoLevelHeight ? `${nominalQualityTier(autoLevelHeight)}p` : undefined;

  const nextEpisode = (() => {
    if (!episodes || !currentEpisodeId) return null;
    const idx = episodes.findIndex((e) => e.id === currentEpisodeId);
    if (idx < 0 || idx >= episodes.length - 1) return null;
    return episodes[idx + 1];
  })();

  const handleNextEpisode = useCallback((id: string) => {
    onEpisodeChange?.(id);
  }, [onEpisodeChange]);

  function handleQualityChange(quality: number) {
    setAutoQuality(false);
    setCurrentQuality(quality);
  }

  function handleAutoQualityToggle(auto: boolean) {
    setAutoQuality(auto);
  }

  function handleAudioChange(index: number) {
    userPickedAudioRef.current = true;
    const stream = audioStreams.find((s) => s.index === index);
    setCurrentAudioIndex(index);
    if (stream) {
      setCurrentTrackIndex(stream.trackIndex);
      const lang = stream.language ?? stream.title ?? null;
      setPreferredAudioLang(lang);
      if (seriesId && lang) {
        const cur = getSeriesTrackPrefs(seriesId) ?? {};
        setSeriesTrackPrefs(seriesId, { ...cur, audioLang: lang });
      }
    }

    if (!transcodeMode) {
      if (hlsRef.current) {
        hlsRef.current.audioTrack = stream?.trackIndex ?? 0;
        return;
      }
      const video = videoRef.current as (HTMLVideoElement & { audioTracks?: { length: number; [i: number]: { id: string; enabled: boolean } } }) | null;
      if (!video) return;
      const tracks = video.audioTracks;
      if (!tracks) return;
      for (let i = 0; i < tracks.length; i++) {
        tracks[i].enabled = tracks[i].id === String(index);
      }
    } else {
      setTranscodeStartTime(positionRef.current);
    }
  }

  return (
    <div
      ref={containerRef}
      data-player-container
      className="absolute inset-0 w-full h-full bg-black"
      onTouchStart={handleTouchStart}
      onTouchMove={handleTouchMove}
      onTouchEnd={handleTouchEnd}
      onWheel={handleWheel}
    >
      <video ref={(el) => { videoRef.current = el; onVideoRef?.(el); }} className="w-full h-full" playsInline style={brightness !== 1 ? { filter: `brightness(${brightness})` } : undefined} />
      {seekFlash === "left" && (
        <div className="absolute left-0 top-0 bottom-0 w-1/2 flex items-center justify-center pointer-events-none z-[6]">
          <div className="rounded-full bg-white/20 w-16 h-16 flex items-center justify-center">
            <RotateCcw size={32} className="text-white" />
          </div>
        </div>
      )}
      {seekFlash === "right" && (
        <div className="absolute right-0 top-0 bottom-0 w-1/2 flex items-center justify-center pointer-events-none z-[6]">
          <div className="rounded-full bg-white/20 w-16 h-16 flex items-center justify-center">
            <RotateCw size={32} className="text-white" />
          </div>
        </div>
      )}
      {swipeIndicator && (
        <div
          className={`absolute top-0 bottom-0 w-16 flex items-center justify-center pointer-events-none z-[6] ${swipeIndicator.type === "volume" ? "right-4" : "left-4"}`}
        >
          <div className="flex flex-col items-center gap-2 rounded-xl bg-black/60 px-3 py-4 backdrop-blur-sm">
            {swipeIndicator.type === "volume" ? <Volume2 size={20} className="text-white" /> : <Sun size={20} className="text-white" />}
            <div className="w-1 h-24 rounded-full bg-white/20 relative overflow-hidden">
              <div
                className="absolute bottom-0 left-0 right-0 rounded-full bg-white"
                style={{ height: `${Math.round((swipeIndicator.type === "volume" ? swipeIndicator.value : (swipeIndicator.value - 0.2) / 1.3) * 100)}%` }}
              />
            </div>
            <span className="text-white text-xs font-medium">{Math.round(swipeIndicator.value * 100)}%</span>
          </div>
        </div>
      )}
      {actionFlash && (
        <div className="absolute inset-0 flex items-center justify-center pointer-events-none z-[6]">
          <div className="rounded-full bg-black/40 w-16 h-16 flex items-center justify-center animate-ping-once">
            {actionFlash === "play" ? (
              <Play size={32} className="text-white ml-1" fill="currentColor" />
            ) : (
              <Pause size={32} className="text-white" fill="currentColor" />
            )}
          </div>
        </div>
      )}
      {isBuffering && !isPlaying && (
        <div className="absolute inset-0 flex items-center justify-center pointer-events-none z-[5]">
          <div className="flex flex-col items-center gap-2">
            <Loader size={40} className="animate-spin text-white/80" />
            <span className="text-white/80 text-sm font-medium">{t("player.buffering")}</span>
          </div>
        </div>
      )}
      {fatalError && (
        <div className="absolute inset-0 flex items-center justify-center z-[10] bg-[var(--overlay-bg-strong)]">
          <div className="flex flex-col items-center gap-4 text-white">
            <TriangleAlert size={48} className="text-[var(--color-danger)]" strokeWidth={1.5} />
            <p className="text-lg font-medium">{t("player.playbackFailed")}</p>
            <p className="text-sm text-[var(--overlay-text-muted)]">{t("player.unableToPlayAnyQuality")}</p>
            <div className="flex items-center gap-2">
              <button
                onClick={() => { setFatalError(false); window.location.reload(); }}
                className="px-4 py-2 rounded text-sm font-medium bg-[var(--overlay-btn-hover-bg)]"
              >
                {t("player.retry")}
              </button>
              {/* Playability fallback: if the server can't deliver a playable
                  stream, let the user download the original file instead of
                  being stuck on the error. */}
              {streamToken && (
                <DownloadButton mediaId={mediaId} token={streamToken} title={title} />
              )}
            </div>
          </div>
        </div>
      )}
      <ProgressReporter sessionId={sessionId} videoRef={videoRef} transcodeOffsetRef={transcodeOffsetRef} mediaId={mediaId} mediaType={mediaType} onRemoteStop={onClose} />
      <AutoMarkWatched
        mediaId={mediaId}
        videoRef={videoRef}
        knownDurationSec={knownDurationMs ? knownDurationMs / 1000 : undefined}
        transcodeOffsetRef={transcodeOffsetRef}
      />
      <SkipIntroButton
        mediaId={mediaId}
        filePath={filePath}
        currentTime={overlayTime}
        onSkip={(t) => handleSeekRequest(t)}
      />
      {nextEpisode && (
        <NextEpisodeOverlay
          nextEpisode={nextEpisode}
          currentTime={overlayTime}
          duration={overlayDuration}
          onPlayNow={handleNextEpisode}
          onCancel={() => {}}
        />
      )}
      <PlayerProvider
        videoRef={videoRef}
        knownDurationSec={knownDurationMs ? knownDurationMs / 1000 : undefined}
        audioStreams={audioStreams}
        subtitleStreams={subtitleStreams}
        title={title}
        currentAudioIndex={currentAudioIndex}
        currentQuality={currentQuality}
        autoQuality={autoQuality}
        autoQualityLabel={autoQualityLabel}
        currentSubtitleTrackIndex={currentSubtitle}
        playbackSpeed={playbackSpeed}
        seasons={seasons}
        currentSeasonId={currentSeasonId}
        episodes={episodes}
        currentEpisodeId={currentEpisodeId}
        onPlayEpisode={onEpisodeChange}
        chapters={chapters}
        mediaId={mediaId}
        mediaType={mediaType}
        transcodeOffset={transcodeOffsetRef.current}
        skipForwardSeconds={skipFwd}
        skipBackwardSeconds={skipBwd}
        subtitleStyle={subtitleStyle}
        streamToken={streamToken}
        brightness={brightness}
        availableQualities={hlsLevels}
      >
        <PlayerControls
          streamInfo={streamInfo}
          onClose={onClose}
          onAudioChange={handleAudioChange}
          onQualityChange={handleQualityChange}
          onAutoQualityToggle={handleAutoQualityToggle}
          onSubtitleChange={handleSubtitleChange}
          onSpeedChange={setPlaybackSpeed}
          onSeasonChange={onSeasonChange}
          onEpisodeChange={onEpisodeChange}
          onSeekRequest={handleSeekRequest}
          onSubtitleStyleChange={updateSubtitleStyle}
          onExternalSubtitle={(blobUrl, label) => {
            setCurrentSubtitle(null);
            setExternalSubUrl({ url: blobUrl, label });
          }}
          onPanelOpenChange={(anyOpen) => { anyPanelOpenRef.current = anyOpen; }}
          closePanelRef={closePanelRef}
          controlsToggleRef={controlsToggleRef}
          pipToggleRef={pipToggleRef}
          onActionFlash={showActionFlash}
          onBrightnessChange={setBrightnessValue}
          castAvailable={castAvailable}
          castConnected={castConnected}
          castDeviceName={castDeviceName}
          castPlayback={castPlayback}
          castTranscodeOffset={castTranscodeOffsetRef.current}
          onCastToggle={handleCastToggle}
          onCastPlayPause={castPlayPause}
          onCastSeek={handleCastSeek}
          onCastSetVolume={castSetVolume}
          onCastSetMuted={castSetMuted}
          isBuffering={isBuffering}
          isPlaying={isPlaying}
        />
      </PlayerProvider>
      {showShortcuts && <KeyboardShortcutsOverlay onClose={() => setShowShortcuts(false)} skipForwardSeconds={skipFwd} skipBackwardSeconds={skipBwd} />}
    </div>
  );
}
