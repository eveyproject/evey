import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useAuthStore } from "@evey/store";
import { RotateCcw, RotateCw, Play, Pause, Loader, TriangleAlert, Volume2, Sun } from "lucide-react";
import Hls from "hls.js";
import type { V1StreamInfo } from "@evey/api";
import type { AudioStream } from "./AudioTrackSelector";
import type { SubtitleStream } from "./SubtitleSelector";
import { PlayerControls } from "./PlayerControls";
import type { EpisodeNavItem, SeasonNavItem } from "./PlayerControls";
import { PlayerProvider } from "./PlayerContext";
import { ProgressReporter } from "./ProgressReporter";
import { AutoMarkWatched } from "./AutoMarkWatched";
import { NextEpisodeOverlay } from "./NextEpisodeOverlay";
import { SkipIntroButton } from "./SkipIntroButton";
import { KeyboardShortcutsOverlay } from "./KeyboardShortcutsOverlay";
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
import { buildTranscodeHlsUrl, getSelectedBurnInTrack, resolveDefaultAudio, resolveDefaultSubtitle, shouldRestartTranscodeSeek } from "./hooks/hlsPlayerUtils";
import { useDirectPlayback } from "./hooks/useDirectPlayback";
import { useTranscodePlayback } from "./hooks/useTranscodePlayback";
import { useSubtitleManager } from "./hooks/useSubtitleManager";
import { usePlayerKeyboard } from "./hooks/usePlayerKeyboard";
import { useTouchGestures } from "./hooks/useTouchGestures";
import { useOverlayTime } from "./hooks/useOverlayTime";
import { useDLNA } from "@/hooks/useDLNA";

export type { AudioStream, SubtitleStream, EpisodeNavItem, SeasonNavItem };

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
  const positionRef = useRef(startPositionMs / 1000);
  const knownDurationSecRef = useRef(knownDurationMs ? knownDurationMs / 1000 : 0);
  knownDurationSecRef.current = knownDurationMs ? knownDurationMs / 1000 : 0;
  const [transcodeMode, setTranscodeMode] = useState(
    streamInfo.playMethod !== "PLAY_METHOD_DIRECT_PLAY" &&
    streamInfo.playMethod !== "PLAY_METHOD_DIRECT_STREAM"
  );

  const defaultAudio = resolveDefaultAudio(audioStreams, seriesId, getSeriesTrackPrefs, getPreferredAudioLang);
  const [currentAudioIndex, setCurrentAudioIndex] = useState(defaultAudio?.index ?? 0);
  const [currentTrackIndex, setCurrentTrackIndex] = useState(defaultAudio?.trackIndex ?? 0);
  const [currentQuality, setCurrentQuality] = useState(0);
  const [autoQuality, setAutoQuality] = useState(true);
  const [currentSubtitle, setCurrentSubtitle] = useState<number | null>(() =>
    resolveDefaultSubtitle(subtitleStreams, seriesId, getSeriesTrackPrefs, getPreferredSubtitleLang),
  );

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
  const isDirectStream = streamInfo.playMethod === "PLAY_METHOD_DIRECT_STREAM";

  // --- Hooks ---

  useDirectPlayback({
    videoRef, hlsRef, positionRef, streamInfo, startPositionMs,
    transcodeMode, isDirectPlay, isDirectStream, onEnded,
    setTranscodeMode, setIsBuffering,
    streamTokenRef, onStreamTokenRefreshRef,
  });

  const { transcodeStartTime, setTranscodeStartTime, hlsLevels } = useTranscodePlayback({
    videoRef, hlsRef, positionRef, transcodeOffsetRef,
    wasPausedRef, streamTokenRef, onStreamTokenRefreshRef,
    transcodeMode, mediaId, currentTrackIndex, selectedBurnInTrack,
    startPositionMs, onEnded, setIsBuffering, setFatalError,
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
    if (transcodeMode) {
      // The transcode is a sliding-window stream: a target outside what hls.js
      // has buffered (forward past the edge, OR backward before the window's
      // start after a long watch) is no longer in the playlist and needs a
      // restart at the new offset. Only restarting forward-past-buffer let a
      // backward scrub fall through to an in-place seek that hls.js couldn't
      // satisfy, so the playhead snapped back to where it was.
      if (shouldRestartTranscodeSeek(absoluteTimeSec, transcodeOffsetRef.current, video.buffered)) {
        setTranscodeStartTime(absoluteTimeSec);
        return;
      }
      video.currentTime = absoluteTimeSec - transcodeOffsetRef.current;
    } else {
      video.currentTime = absoluteTimeSec;
    }
  }, [transcodeMode, setTranscodeStartTime]);

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

  usePlayerKeyboard({
    videoRef, positionRef, knownDurationSecRef, seekRef, skipFwdRef, skipBwdRef,
    playbackSpeedRef, subtitleStreamsRef, currentSubtitleRef, anyPanelOpenRef,
    closePanelRef, onCloseRef, setPlaybackSpeed, setCurrentSubtitle,
    setShowShortcuts, showActionFlash,
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
    if (autoQuality) {
      hls.currentLevel = -1;
    } else if (currentQuality > 0) {
      const idx = hls.levels.findIndex((l: { height: number }) => l.height === currentQuality);
      if (idx >= 0) hls.currentLevel = idx;
    } else {
      hls.currentLevel = -1;
    }
  }, [autoQuality, currentQuality, transcodeMode]);

  const autoQualityLabel = autoQuality ? "Auto" : undefined;

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
      {isBuffering && (
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
            <button
              onClick={() => { setFatalError(false); window.location.reload(); }}
              className="px-4 py-2 rounded text-sm font-medium bg-[var(--overlay-btn-hover-bg)]"
            >
              {t("player.retry")}
            </button>
          </div>
        </div>
      )}
      <ProgressReporter sessionId={sessionId} videoRef={videoRef} transcodeOffsetRef={transcodeOffsetRef} onRemoteStop={onClose} />
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
          onClose={onClose}
          onAudioChange={handleAudioChange}
          onQualityChange={handleQualityChange}
          onAutoQualityToggle={handleAutoQualityToggle}
          onSubtitleChange={(trackIndex) => {
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
              const s = subtitleStreams.find((st) => st.trackIndex === trackIndex);
              const lang = s?.language ?? s?.title ?? null;
              setPreferredSubtitleLang(lang);
              if (seriesId) {
                const cur = getSeriesTrackPrefs(seriesId) ?? {};
                setSeriesTrackPrefs(seriesId, { ...cur, subtitleLang: lang });
              }
            }
          }}
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
        />
      </PlayerProvider>
      {showShortcuts && <KeyboardShortcutsOverlay onClose={() => setShowShortcuts(false)} skipForwardSeconds={skipFwd} skipBackwardSeconds={skipBwd} />}
    </div>
  );
}
