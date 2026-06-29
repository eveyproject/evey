import { memo, useCallback, useEffect, useRef, useState } from "react";
import { ArrowLeft, SkipBack, SkipForward, Pause, Play, Minimize, Maximize, PictureInPicture, PictureInPicture2, ChevronFirst, ChevronLast, Cast, Loader } from "lucide-react";
import { PlayOnPicker } from "./PlayOnPicker";
import { useTranslation } from "react-i18next";
import type { SubtitleStyle } from "@/hooks/usePlaybackPrefs";
import { PlayerVolumeControl } from "./PlayerVolumeControl";
import { EpisodesPanel } from "./PlayerEpisodesPanel";
import type { EpisodeNavItem, SeasonNavItem } from "./PlayerEpisodesPanel";
import { SettingsPanel } from "./PlayerSettingsPanel";
import { StreamInfoPanel } from "./StreamInfoPanel";
import { usePlayerContext, usePlayerTimeRef } from "./PlayerContext";
import { LiveProgressBar, LiveTimeLabel, LiveChaptersPanel, useActiveChapterIndex } from "./PlayerLiveControls";
import { useDocumentPiP } from "./hooks/useDocumentPiP";
import type { V1StreamInfo } from "@evey/api";

export type { EpisodeNavItem, SeasonNavItem };

interface PlayerControlsProps {
  onClose?: () => void;
  onAudioChange: (index: number) => void;
  onQualityChange: (quality: number) => void;
  onSubtitleChange: (trackIndex: number | null) => void;
  onSpeedChange?: (speed: number) => void;
  onSeasonChange?: (id: string) => void;
  onEpisodeChange?: (id: string) => void;
  onSeekRequest?: (absoluteTimeSec: number) => void;
  onAutoQualityToggle?: (auto: boolean) => void;
  onExternalSubtitle?: (blobUrl: string, label: string) => void;
  onSubtitleStyleChange?: (patch: Partial<SubtitleStyle>) => void;
  onPanelOpenChange?: (anyOpen: boolean) => void;
  closePanelRef?: React.MutableRefObject<(() => void) | null>;
  onActionFlash?: (action: "play" | "pause") => void;
  onBrightnessChange?: (value: number) => void;
  controlsToggleRef?: React.MutableRefObject<(() => void) | null>;
  castAvailable?: boolean;
  castConnected?: boolean;
  castDeviceName?: string | null;
  castPlayback?: { playerState: string | null; currentTime: number; duration: number; isPaused: boolean; isMediaLoaded: boolean; volume: number; isMuted: boolean };
  // Absolute offset the current cast HLS stream was built at (startSec).
  // The device reports currentTime local to that stream, so the absolute
  // position is castTranscodeOffset + castPlayback.currentTime.
  castTranscodeOffset?: number;
  onCastToggle?: () => void;
  onCastPlayPause?: () => void;
  onCastSeek?: (timeSec: number) => boolean;
  onCastSetVolume?: (level: number) => void;
  onCastSetMuted?: (muted: boolean) => void;
  streamInfo?: V1StreamInfo;
  /** True while the HLS pipeline is stalled waiting for segments. */
  isBuffering?: boolean;
  /** True when the video element is actively playing (not paused / not yet started). */
  isPlaying?: boolean;
}

function PlayerControlsInner({
  onClose,
  onAudioChange,
  onQualityChange,
  onSubtitleChange,
  onSpeedChange,
  onSeasonChange,
  onEpisodeChange,
  onSeekRequest,
  onAutoQualityToggle,
  onExternalSubtitle,
  onSubtitleStyleChange,
  onPanelOpenChange,
  closePanelRef,
  onActionFlash,
  onBrightnessChange,
  controlsToggleRef,
  castAvailable,
  castConnected,
  castDeviceName,
  castPlayback,
  castTranscodeOffset = 0,
  onCastToggle,
  onCastPlayPause,
  onCastSeek,
  onCastSetVolume,
  onCastSetMuted,
  streamInfo,
  isBuffering,
  isPlaying,
}: PlayerControlsProps) {
  const { t } = useTranslation();
  const ctx = usePlayerContext();
  const timeRef = usePlayerTimeRef();
  const docPiP = useDocumentPiP();
  const {
    videoRef, playing: localPlaying, volume: localVolume, muted: localMuted, fullscreen, isPip,
    effectiveDuration: localDuration, transcodeOffset,
    audioStreams, subtitleStreams, currentAudioIndex, currentQuality,
    currentSubtitleTrackIndex, playbackSpeed, autoQuality, autoQualityLabel,
    availableQualities, chapters, mediaId, mediaType,
    title, brightness, skipForwardSeconds, skipBackwardSeconds,
    seasons, currentSeasonId, episodes, currentEpisodeId, subtitleStyle,
  } = ctx;

  // When casting, use cast playback state instead of local video state.
  // The device's currentTime is local to the HLS stream it was handed
  // (built at startSec=castTranscodeOffset), so add the offset back to
  // get the absolute position the seek bar / time display expect.
  const isCasting = castConnected && castPlayback?.isMediaLoaded;
  const playing = isCasting ? !castPlayback.isPaused : localPlaying;
  const effectiveDuration = localDuration;
  const volume = isCasting ? (castPlayback.volume ?? 1) : localVolume;
  const muted = isCasting ? (castPlayback.isMuted ?? false) : localMuted;

  // The live position no longer lives in this component's render (it's split
  // into PlayerTimeContext, consumed by the Live* leaves below) so the control
  // bar stops re-rendering 4×/sec. When casting it comes from the device prop;
  // pass that down to the leaves as castTimeSec. Handlers read the *current*
  // position at call time via the ref, never from render state.
  const castTimeSec = isCasting ? castTranscodeOffset + castPlayback.currentTime : undefined;
  const currentAbsoluteTime = () =>
    isCasting ? castTranscodeOffset + castPlayback.currentTime : timeRef.current.absoluteTime;
  const currentChapterIndex = useActiveChapterIndex(chapters, videoRef, transcodeOffset, castTimeSec);

  const [visible, setVisible] = useState(true);
  const [openPanel, setOpenPanel] = useState<"settings" | "episodes" | "chapters" | "streaminfo" | null>(null);
  const settingsOpen = openPanel === "settings";
  const episodesOpen = openPanel === "episodes";
  const chaptersOpen = openPanel === "chapters";

  useEffect(() => {
    onPanelOpenChange?.(openPanel !== null);
  }, [openPanel, onPanelOpenChange]);

  useEffect(() => {
    if (closePanelRef) closePanelRef.current = () => setOpenPanel(null);
    return () => { if (closePanelRef) closePanelRef.current = null; };
  }, [closePanelRef]);

  const hideTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const clickTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const scheduleHide = useCallback(() => {
    if (hideTimer.current) clearTimeout(hideTimer.current);
    hideTimer.current = setTimeout(() => setVisible(false), 3000);
  }, []);

  useEffect(() => {
    if (controlsToggleRef) controlsToggleRef.current = () => {
      setVisible((v) => {
        if (v) {
          if (hideTimer.current) clearTimeout(hideTimer.current);
          return false;
        }
        scheduleHide();
        return true;
      });
    };
    return () => { if (controlsToggleRef) controlsToggleRef.current = null; };
  }, [controlsToggleRef, scheduleHide]);

  const resetHideTimer = useCallback(() => {
    setVisible(true);
    // Don't auto-hide while ANY side panel is open — previously only the
    // settings panel was guarded, so a hover over the controls with the
    // episodes/chapters panel open would queue a hide that fired 3s
    // later and caused a brief flicker before the panel-open effect
    // forced visibility back to true.
    if (openPanel === null) scheduleHide();
  }, [openPanel, scheduleHide]);

  useEffect(() => {
    if (settingsOpen || episodesOpen || chaptersOpen) {
      if (hideTimer.current) clearTimeout(hideTimer.current);
      setVisible(true);
    } else {
      scheduleHide();
    }
  }, [settingsOpen, episodesOpen, chaptersOpen, scheduleHide]);

  useEffect(() => {
    scheduleHide();
    return () => {
      if (hideTimer.current) clearTimeout(hideTimer.current);
    };
  }, [scheduleHide]);

  useEffect(() => {
    return () => {
      if (clickTimer.current) clearTimeout(clickTimer.current);
    };
  }, []);

  function togglePlay() {
    if (castConnected && onCastPlayPause) {
      onCastPlayPause();
      return;
    }
    const video = videoRef.current;
    if (!video) return;
    if (video.paused) void video.play();
    else video.pause();
  }

  // onCastSeek returns false when the cast session can't actually seek
  // (dead/stale session). In that case we must fall through to the
  // local player — otherwise a Chromecast that bugged out leaves every
  // seek silently swallowed.
  function seekTo(targetAbsolute: number) {
    if (isCasting && onCastSeek && onCastSeek(targetAbsolute)) return;
    if (onSeekRequest) {
      onSeekRequest(targetAbsolute);
    } else {
      const video = videoRef.current;
      if (video) video.currentTime = targetAbsolute;
    }
  }

  function handleSeek(e: React.ChangeEvent<HTMLInputElement>) {
    seekTo(Number(e.target.value));
  }

  function skipForward() {
    seekTo(Math.min(effectiveDuration, currentAbsoluteTime() + skipForwardSeconds));
  }

  function skipBackward() {
    seekTo(Math.max(0, currentAbsoluteTime() - skipBackwardSeconds));
  }

  const prevEpisode = (() => {
    if (!episodes || !currentEpisodeId) return null;
    const idx = episodes.findIndex((e) => e.id === currentEpisodeId);
    if (idx <= 0) return null;
    return episodes[idx - 1];
  })();

  function handleVolume(e: React.ChangeEvent<HTMLInputElement>) {
    const val = Number(e.target.value);
    if (isCasting && onCastSetVolume) {
      onCastSetVolume(val);
      return;
    }
    const video = videoRef.current;
    if (!video) return;
    video.volume = val;
    // The muted-autoplay fallback leaves video.muted=true even after the user
    // grabs the slider; without this unmute, dragging the bar visibly raises
    // the slider but produces no sound — they have to also click the volume
    // icon. Interpret a >0 drag as "I want sound now" and clear the mute.
    if (val > 0 && video.muted) {
      video.muted = false;
    }
  }

  function togglePip() {
    // Prefer Document PiP (Chrome 116+): brings the WHOLE player UI into
    // the floating window — progress bar, subtitles, chapter skip, next
    // episode prompt — not just the bare <video>. Falls back to the
    // standard PiP on Safari/Firefox where DocPiP doesn't exist.
    if (docPiP.available) {
      void docPiP.toggle();
      return;
    }
    const video = videoRef.current;
    if (!video || !document.pictureInPictureEnabled) return;
    if (document.pictureInPictureElement) {
      void document.exitPictureInPicture();
    } else {
      void video.requestPictureInPicture();
    }
  }

  function toggleMute() {
    if (isCasting && onCastSetMuted) {
      onCastSetMuted(!muted);
      return;
    }
    const video = videoRef.current;
    if (video) video.muted = !video.muted;
  }

  function toggleFullscreen() {
    const video = videoRef.current;
    if (!video) return;
    const container = video.closest("[data-player-container]") as HTMLElement | null;
    if (!container) return;
    if (!document.fullscreenElement) {
      void container.requestFullscreen();
    } else {
      void document.exitFullscreen();
    }
  }

  function handleOverlayClick(e: React.MouseEvent) {
    if ((e.target as HTMLElement).closest("button, input, [data-controls]")) return;
    if (openPanel) {
      setOpenPanel(null);
      return;
    }
    if (clickTimer.current) {
      clearTimeout(clickTimer.current);
      clickTimer.current = null;
      toggleFullscreen();
    } else {
      clickTimer.current = setTimeout(() => {
        clickTimer.current = null;
        if (isCasting && onCastPlayPause) {
          onCastPlayPause();
          onActionFlash?.(playing ? "pause" : "play");
        } else {
          const video = videoRef.current;
          if (video) {
            if (video.paused) {
              void video.play();
              onActionFlash?.("play");
            } else {
              video.pause();
              onActionFlash?.("pause");
            }
          }
        }
        resetHideTimer();
      }, 200);
    }
  }

  const handleChapterSeek = (timeSec: number) => {
    if (onSeekRequest) onSeekRequest(timeSec);
    else if (videoRef.current) videoRef.current.currentTime = timeSec;
  };

  const currentChapter = currentChapterIndex >= 0 ? chapters[currentChapterIndex] : null;

  function skipToNextChapter() {
    const nextIdx = currentChapterIndex >= 0 ? currentChapterIndex + 1 : 0;
    if (nextIdx < chapters.length) handleChapterSeek(chapters[nextIdx].startTimeMs / 1000);
  }

  function skipToPrevChapter() {
    const threshold = 3;
    if (currentChapterIndex > 0 && currentChapter) {
      const elapsed = currentAbsoluteTime() - currentChapter.startTimeMs / 1000;
      if (elapsed > threshold) {
        handleChapterSeek(currentChapter.startTimeMs / 1000);
      } else {
        handleChapterSeek(chapters[currentChapterIndex - 1].startTimeMs / 1000);
      }
    } else if (currentChapter) {
      handleChapterSeek(currentChapter.startTimeMs / 1000);
    }
  }

  return (
    <div
      className="absolute inset-0"
      onMouseMove={resetHideTimer}
      onClick={handleOverlayClick}
      style={{ cursor: visible ? "default" : "none" }}
    >
      {/* top bar */}
      <div
        className={`absolute top-0 left-0 right-0 flex items-center gap-3 py-3 transition-opacity-fast duration-300 ${visible ? "opacity-100" : "opacity-0 pointer-events-none"}`}
        style={{ background: "linear-gradient(to bottom, var(--overlay-bg-strong), transparent)", paddingLeft: "max(1rem, env(safe-area-inset-left, 1rem))", paddingRight: "max(1rem, env(safe-area-inset-right, 1rem))" }}
        data-controls
      >
        <button
          onClick={onClose}
          aria-label={t("common.back")}
          className="text-[var(--overlay-text)] hover:text-[var(--accent)] transition-colors-fast focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:ring-offset-1 rounded"
        >
          <ArrowLeft size={24}  aria-hidden="true" />
        </button>
        {title && <span className="text-[var(--overlay-text)] text-sm font-medium truncate">{title}</span>}
        {isCasting && castDeviceName && (
          <span className="ml-auto flex items-center gap-1.5 text-[var(--accent)] text-xs font-medium">
            <Cast size={14} />
            {castDeviceName}
          </span>
        )}
      </div>

      {/* bottom controls */}
      <div
        className={`absolute bottom-0 left-0 right-0 pt-8 flex flex-col gap-2 transition-opacity-fast duration-300 ${visible ? "opacity-100" : "opacity-0 pointer-events-none"}`}
        style={{ background: "linear-gradient(to top, var(--overlay-bg-strong), transparent)", paddingBottom: "max(1rem, env(safe-area-inset-bottom, 1rem))", paddingLeft: "max(1rem, env(safe-area-inset-left, 1rem))", paddingRight: "max(1rem, env(safe-area-inset-right, 1rem))" }}
        data-controls
      >
        {/* seek bar — subscribes to PlayerTimeContext on its own */}
        <LiveProgressBar
          castTimeSec={castTimeSec}
          effectiveDuration={effectiveDuration}
          transcodeOffset={transcodeOffset}
          chapters={chapters}
          mediaId={mediaId}
          mediaType={mediaType}
          onSeek={handleSeek}
          onChapterSeek={handleChapterSeek}
        />

        {/* current chapter label */}
        {currentChapter && (
          <div className="text-xs truncate px-1 text-[var(--text-muted)]">
            {currentChapter.title}
          </div>
        )}

        <div className="flex items-center gap-2 sm:gap-3">
          {/* previous episode */}
          {prevEpisode && onEpisodeChange && (
            <button
              onClick={() => onEpisodeChange(prevEpisode.id)}
              aria-label={t("player.controls.previousEpisode")}
              className="text-[var(--overlay-text)] hover:text-[var(--accent)] transition-colors-fast p-1 focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:ring-offset-1 rounded"
              title={prevEpisode.title ? `E${prevEpisode.indexNumber} - ${prevEpisode.title}` : undefined}
            >
              <SkipBack size={22} fill="currentColor"  aria-hidden="true" />
            </button>
          )}

          {/* previous chapter */}
          {chapters.length > 0 && currentChapterIndex > 0 && (
            <button
              onClick={skipToPrevChapter}
              aria-label={t("player.controls.chapters")}
              className="text-[var(--overlay-text)] hover:text-[var(--accent)] transition-colors-fast p-1 focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:ring-offset-1 rounded"
            >
              <ChevronFirst size={22}  aria-hidden="true" />
            </button>
          )}

          {/* skip backward */}
          <button
            onClick={skipBackward}
            aria-label={t("player.controls.rewind", { seconds: skipBackwardSeconds })}
            title={`${t("player.controls.rewind", { seconds: skipBackwardSeconds })} (J)`}
            className="text-[var(--overlay-text)] hover:text-[var(--accent)] transition-colors-fast p-1 focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:ring-offset-1 rounded"
          >
            <svg width="22" height="22" viewBox="0 0 24 24" fill="currentColor">
              <path d="M11.99 5V1l-5 5 5 5V7c3.31 0 6 2.69 6 6s-2.69 6-6 6-6-2.69-6-6h-2c0 4.42 3.58 8 8 8s8-3.58 8-8-3.58-8-8-8z" />
              <text x="12" y="15.5" textAnchor="middle" fontSize="7" fontWeight="bold" fill="currentColor">{skipBackwardSeconds}</text>
            </svg>
          </button>

          {/* play/pause */}
          <button
            onClick={togglePlay}
            aria-label={playing ? t("player.controls.pause") : t("player.controls.play")}
            title={playing ? `${t("player.controls.pause")} (K)` : `${t("player.controls.play")} (K)`}
            className="text-[var(--overlay-text)] hover:text-[var(--accent)] transition-colors-fast p-1 focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:ring-offset-1 rounded"
          >
            {playing ? (
              <Pause size={28} fill="currentColor"  aria-hidden="true" />
            ) : (
              <Play size={28} fill="currentColor"  aria-hidden="true" />
            )}
          </button>

          {/* skip forward */}
          <button
            onClick={skipForward}
            aria-label={t("player.controls.forward", { seconds: skipForwardSeconds })}
            title={`${t("player.controls.forward", { seconds: skipForwardSeconds })} (L)`}
            className="text-[var(--overlay-text)] hover:text-[var(--accent)] transition-colors-fast p-1 focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:ring-offset-1 rounded"
          >
            <svg width="22" height="22" viewBox="0 0 24 24" fill="currentColor">
              <path d="M12.01 5V1l5 5-5 5V7c-3.31 0-6 2.69-6 6s2.69 6 6 6 6-2.69 6-6h2c0 4.42-3.58 8-8 8s-8-3.58-8-8 3.58-8 8-8z" />
              <text x="12" y="15.5" textAnchor="middle" fontSize="7" fontWeight="bold" fill="currentColor">{skipForwardSeconds}</text>
            </svg>
          </button>

          {/* next chapter */}
          {chapters.length > 0 && currentChapterIndex >= 0 && currentChapterIndex < chapters.length - 1 && (
            <button
              onClick={skipToNextChapter}
              aria-label={t("player.controls.chapters")}
              className="text-[var(--overlay-text)] hover:text-[var(--accent)] transition-colors-fast p-1 focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:ring-offset-1 rounded"
            >
              <ChevronLast size={22}  aria-hidden="true" />
            </button>
          )}

          {/* next episode */}
          {episodes && episodes.length > 0 && onEpisodeChange && (() => {
            const idx = episodes.findIndex((e) => e.id === currentEpisodeId);
            const next = idx >= 0 && idx < episodes.length - 1 ? episodes[idx + 1] : null;
            if (!next) return null;
            return (
              <button
                onClick={() => onEpisodeChange(next.id)}
                aria-label={t("player.controls.nextEpisode")}
                className="text-[var(--overlay-text)] hover:text-[var(--accent)] transition-colors-fast p-1 focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:ring-offset-1 rounded"
                title={next.title ? `E${next.indexNumber} - ${next.title}` : undefined}
              >
                <SkipForward size={22} fill="currentColor"  aria-hidden="true" />
              </button>
            );
          })()}

          {/* time — subscribes to PlayerTimeContext on its own */}
          <LiveTimeLabel castTimeSec={castTimeSec} effectiveDuration={effectiveDuration} />

          {/* volume */}
          <PlayerVolumeControl
            volume={volume}
            muted={muted}
            onToggleMute={toggleMute}
            onVolumeChange={handleVolume}
          />

          {/* Discrete buffering indicator shown only during playback stalls */}
          {isBuffering && isPlaying && (
            <Loader size={16} className="animate-spin text-white/70 shrink-0" aria-hidden="true" />
          )}

          <div className="flex-1" />

          {/* Chapters panel */}
          {chapters.length > 0 && (
            <LiveChaptersPanel
              chapters={chapters}
              castTimeSec={castTimeSec}
              onSeek={handleChapterSeek}
              open={chaptersOpen}
              onToggle={() => setOpenPanel(chaptersOpen ? null : "chapters")}
            />
          )}

          {/* Episodes panel (only for TV episodes) */}
          {episodes.length > 0 && onEpisodeChange && (
            <EpisodesPanel
              seasons={seasons}
              currentSeasonId={currentSeasonId}
              episodes={episodes}
              currentEpisodeId={currentEpisodeId}
              onSeasonChange={onSeasonChange ?? (() => {})}
              onEpisodeChange={onEpisodeChange}
              open={episodesOpen}
              onToggle={() => setOpenPanel(episodesOpen ? null : "episodes")}
            />
          )}

          {/* Settings gear */}
          <SettingsPanel
            audioStreams={audioStreams}
            subtitleStreams={subtitleStreams}
            currentAudioIndex={currentAudioIndex}
            onAudioChange={onAudioChange}
            currentQuality={currentQuality}
            onQualityChange={onQualityChange}
            currentSubtitleTrackIndex={currentSubtitleTrackIndex}
            onSubtitleChange={onSubtitleChange}
            playbackSpeed={playbackSpeed}
            onSpeedChange={onSpeedChange}
            open={settingsOpen}
            onToggle={() => setOpenPanel(settingsOpen ? null : "settings")}
            autoQuality={autoQuality}
            onAutoQualityToggle={onAutoQualityToggle}
            autoQualityLabel={autoQualityLabel}
            onExternalSubtitle={onExternalSubtitle}
            subtitleStyle={subtitleStyle}
            onSubtitleStyleChange={onSubtitleStyleChange}
            brightness={brightness}
            onBrightnessChange={onBrightnessChange}
            availableQualities={availableQualities}
          />

          {/* Stream Info — surfaces the backend's playback decision (method,
              codecs, transcode reasons), parity with the mobile app. */}
          {streamInfo && (
            <StreamInfoPanel
              streamInfo={streamInfo}
              open={openPanel === "streaminfo"}
              onToggle={() => setOpenPanel(openPanel === "streaminfo" ? null : "streaminfo")}
            />
          )}

          {/* Play On — server-brokered remote control of first-party
              clients (TV app, mobile). Works for any browser, any
              network — no Chromecast SDK required. */}
          {mediaId && (
            <PlayOnPicker mediaId={mediaId} getPositionMs={() => Math.floor(currentAbsoluteTime() * 1000)} />
          )}

          {/* Cast */}
          {castAvailable && onCastToggle && (
            <button
              onClick={onCastToggle}
              aria-label={castConnected ? t("player.controls.stopCast") : t("player.controls.cast")}
              title={castConnected && castDeviceName ? castDeviceName : undefined}
              className={`transition-colors-fast p-1 focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:ring-offset-1 rounded ${castConnected ? "text-[var(--accent)]" : "text-[var(--overlay-text)] hover:text-[var(--accent)]"}`}
            >
              <Cast size={20}  aria-hidden="true" />
            </button>
          )}

          {/* Picture-in-Picture (Document PiP when available, falls back to the legacy video-only PiP) */}
          {typeof document !== "undefined" && (docPiP.available || document.pictureInPictureEnabled) && (
            <button
              onClick={togglePip}
              aria-label={
                docPiP.isOpen || isPip
                  ? t("player.controls.exitPip")
                  : t("player.controls.pip")
              }
              title={
                docPiP.isOpen || isPip
                  ? `${t("player.controls.exitPip")} (P)`
                  : `${t("player.controls.pip")} (P)`
              }
              className="text-[var(--overlay-text)] hover:text-[var(--accent)] transition-colors-fast p-1 focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:ring-offset-1 rounded"
            >
              {docPiP.isOpen || isPip ? (
                <PictureInPicture2 size={20}  aria-hidden="true" />
              ) : (
                <PictureInPicture size={20}  aria-hidden="true" />
              )}
            </button>
          )}

          {/* fullscreen */}
          <button
            onClick={toggleFullscreen}
            aria-label={fullscreen ? t("player.controls.exitFullscreen") : t("player.controls.fullscreen")}
            title={fullscreen ? `${t("player.controls.exitFullscreen")} (F)` : `${t("player.controls.fullscreen")} (F)`}
            className="text-[var(--overlay-text)] hover:text-[var(--accent)] transition-colors-fast p-1 focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:ring-offset-1 rounded"
          >
            {fullscreen ? (
              <Minimize size={20}  aria-hidden="true" />
            ) : (
              <Maximize size={20}  aria-hidden="true" />
            )}
          </button>
        </div>
      </div>
    </div>
  );
}

export const PlayerControls = memo(PlayerControlsInner);
