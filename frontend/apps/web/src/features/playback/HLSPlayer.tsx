import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { RotateCcw, RotateCw, Play, Pause, Loader, TriangleAlert } from "lucide-react";
import Hls from "hls.js";
import type { V1StreamInfo } from "@evey/api";
import type { AudioStream } from "./AudioTrackSelector";
import type { SubtitleStream } from "./SubtitleSelector";
import { PlayerControls } from "./PlayerControls";
import type { EpisodeNavItem, SeasonNavItem } from "./PlayerControls";
import { ProgressReporter } from "./ProgressReporter";
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
import { getSelectedBurnInTrack, resolveDefaultAudio, resolveDefaultSubtitle } from "./hooks/hlsPlayerUtils";
import { useDirectPlayback } from "./hooks/useDirectPlayback";
import { useTranscodePlayback } from "./hooks/useTranscodePlayback";
import { useSubtitleManager } from "./hooks/useSubtitleManager";
import { usePlayerKeyboard } from "./hooks/usePlayerKeyboard";
import { useTouchGestures } from "./hooks/useTouchGestures";
import { useOverlayTime } from "./hooks/useOverlayTime";

export type { AudioStream, SubtitleStream, EpisodeNavItem, SeasonNavItem };

interface HLSPlayerProps {
  streamInfo: V1StreamInfo;
  sessionId: string;
  mediaId: string;
  mediaType?: string;
  streamToken: string;
  onStreamTokenRefresh: () => Promise<string>;
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
  const hlsRef = useRef<InstanceType<typeof Hls> | null>(null);
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
  const hlsLevelsRef = useRef<{ height: number }[]>([]);
  const [currentSubtitle, setCurrentSubtitle] = useState<number | null>(() =>
    resolveDefaultSubtitle(subtitleStreams, seriesId, getSeriesTrackPrefs, getPreferredSubtitleLang),
  );

  const selectedBurnInTrack = getSelectedBurnInTrack(currentSubtitle, subtitleStreams);

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
  const skipFwd = playbackSettings?.skipForwardSeconds ?? 10;
  const skipBwd = playbackSettings?.skipBackwardSeconds ?? 10;
  const skipFwdRef = useRef(skipFwd);
  skipFwdRef.current = skipFwd;
  const skipBwdRef = useRef(skipBwd);
  skipBwdRef.current = skipBwd;
  const transcodeOffsetRef = useRef(0);

  const isDirectPlay = streamInfo.playMethod === "PLAY_METHOD_DIRECT_PLAY";
  const isDirectStream = streamInfo.playMethod === "PLAY_METHOD_DIRECT_STREAM";

  // --- Hooks ---

  useDirectPlayback({
    videoRef, hlsRef, positionRef, streamInfo, startPositionMs,
    transcodeMode, isDirectPlay, isDirectStream, onEnded,
    setTranscodeMode, setIsBuffering,
  });

  const { transcodeStartTime, setTranscodeStartTime } = useTranscodePlayback({
    videoRef, hlsRef, hlsLevelsRef, positionRef, transcodeOffsetRef,
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
      const localTime = absoluteTimeSec - transcodeOffsetRef.current;
      if (localTime < 0 || (video.buffered.length > 0 && localTime > video.buffered.end(video.buffered.length - 1) + 10)) {
        setTranscodeStartTime(absoluteTimeSec);
        return;
      }
      video.currentTime = localTime;
    } else {
      video.currentTime = absoluteTimeSec;
    }
  }, [transcodeMode, setTranscodeStartTime]);

  const seekRef = useRef(handleSeekRequest);
  seekRef.current = handleSeekRequest;

  usePlayerKeyboard({
    videoRef, positionRef, knownDurationSecRef, seekRef, skipFwdRef, skipBwdRef,
    playbackSpeedRef, subtitleStreamsRef, currentSubtitleRef, anyPanelOpenRef,
    closePanelRef, onCloseRef, setPlaybackSpeed, setCurrentSubtitle,
    setShowShortcuts, showActionFlash,
  });

  const { overlayTime, overlayDuration } = useOverlayTime({
    videoRef, transcodeOffsetRef, knownDurationSecRef,
  });

  const { seekFlash, handleTouchStart, handleTouchEnd, handleWheel, cleanupTimers } = useTouchGestures({
    videoRef, positionRef, knownDurationSecRef, seekRef, skipFwdRef, skipBwdRef, showActionFlash,
  });

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
      className="absolute inset-0 bg-black"
      onTouchStart={handleTouchStart}
      onTouchEnd={handleTouchEnd}
      onWheel={handleWheel}
    >
      <video ref={(el) => { videoRef.current = el; onVideoRef?.(el); }} className="w-full h-full" playsInline />
      {seekFlash === "left" && (
        <div className="absolute left-0 top-0 bottom-0 w-1/2 flex items-center justify-center pointer-events-none" style={{ zIndex: 6 }}>
          <div className="rounded-full bg-white/20 w-16 h-16 flex items-center justify-center">
            <RotateCcw size={32} className="text-white" />
          </div>
        </div>
      )}
      {seekFlash === "right" && (
        <div className="absolute right-0 top-0 bottom-0 w-1/2 flex items-center justify-center pointer-events-none" style={{ zIndex: 6 }}>
          <div className="rounded-full bg-white/20 w-16 h-16 flex items-center justify-center">
            <RotateCw size={32} className="text-white" />
          </div>
        </div>
      )}
      {actionFlash && (
        <div className="absolute inset-0 flex items-center justify-center pointer-events-none" style={{ zIndex: 6 }}>
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
        <div className="absolute inset-0 flex items-center justify-center pointer-events-none" style={{ zIndex: 5 }}>
          <div className="flex flex-col items-center gap-2">
            <Loader size={40} className="animate-spin text-white/80" />
            <span className="text-white/80 text-sm font-medium">{t("player.buffering")}</span>
          </div>
        </div>
      )}
      {fatalError && (
        <div className="absolute inset-0 flex items-center justify-center" style={{ zIndex: 10, background: "var(--overlay-bg-strong)" }}>
          <div className="flex flex-col items-center gap-4 text-white">
            <TriangleAlert size={48} style={{ color: "var(--color-danger)" }} strokeWidth={1.5} />
            <p className="text-lg font-medium">{t("player.playbackFailed")}</p>
            <p className="text-sm" style={{ color: "var(--overlay-text-muted)" }}>{t("player.unableToPlayAnyQuality")}</p>
            <button
              onClick={() => { setFatalError(false); window.location.reload(); }}
              className="px-4 py-2 rounded text-sm font-medium"
              style={{ backgroundColor: "var(--overlay-btn-hover-bg)" }}
            >
              {t("player.retry")}
            </button>
          </div>
        </div>
      )}
      <ProgressReporter sessionId={sessionId} videoRef={videoRef} transcodeOffsetRef={transcodeOffsetRef} />
      <SkipIntroButton
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
      <PlayerControls
        videoRef={videoRef}
        knownDurationSec={knownDurationMs ? knownDurationMs / 1000 : undefined}
        audioStreams={audioStreams}
        subtitleStreams={subtitleStreams}
        title={title}
        onClose={onClose}
        currentAudioIndex={currentAudioIndex}
        onAudioChange={handleAudioChange}
        currentQuality={currentQuality}
        onQualityChange={handleQualityChange}
        autoQuality={autoQuality}
        onAutoQualityToggle={handleAutoQualityToggle}
        autoQualityLabel={autoQualityLabel}
        currentSubtitleTrackIndex={currentSubtitle}
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
        playbackSpeed={playbackSpeed}
        onSpeedChange={setPlaybackSpeed}
        seasons={seasons}
        currentSeasonId={currentSeasonId}
        episodes={episodes}
        currentEpisodeId={currentEpisodeId}
        onSeasonChange={onSeasonChange}
        onEpisodeChange={onEpisodeChange}
        chapters={chapters}
        mediaId={mediaId}
        mediaType={mediaType}
        onSeekRequest={handleSeekRequest}
        transcodeOffset={transcodeOffsetRef.current}
        skipForwardSeconds={skipFwd}
        skipBackwardSeconds={skipBwd}
        subtitleStyle={subtitleStyle}
        onSubtitleStyleChange={updateSubtitleStyle}
        onExternalSubtitle={(blobUrl, label) => {
          setCurrentSubtitle(null);
          setExternalSubUrl({ url: blobUrl, label });
        }}
        onPanelOpenChange={(anyOpen) => { anyPanelOpenRef.current = anyOpen; }}
        closePanelRef={closePanelRef}
        onActionFlash={showActionFlash}
        streamToken={streamToken}
      />
      {showShortcuts && <KeyboardShortcutsOverlay onClose={() => setShowShortcuts(false)} skipForwardSeconds={skipFwd} skipBackwardSeconds={skipBwd} />}
    </div>
  );
}
