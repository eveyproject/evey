import { useCallback, useEffect, useRef, useState } from "react";
import { ArrowLeft, SkipBack, SkipForward, Pause, Play, Minimize, Maximize, PictureInPicture, PictureInPicture2, ChevronFirst, ChevronLast } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { SubtitleStyle } from "@/hooks/usePlaybackPrefs";
import { formatTime } from "./playerUtils";
import { PlayerProgressBar } from "./PlayerProgressBar";
import { PlayerVolumeControl } from "./PlayerVolumeControl";
import { ChaptersPanel } from "./PlayerChaptersPanel";
import { EpisodesPanel } from "./PlayerEpisodesPanel";
import type { EpisodeNavItem, SeasonNavItem } from "./PlayerEpisodesPanel";
import { SettingsPanel } from "./PlayerSettingsPanel";
import { usePlayerContext } from "./PlayerContext";

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
}

export function PlayerControls({
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
}: PlayerControlsProps) {
  const { t } = useTranslation();
  const ctx = usePlayerContext();
  const {
    videoRef, playing, volume, muted, fullscreen, isPip,
    effectiveDuration, absoluteTime, bufferedEnd, transcodeOffset,
    audioStreams, subtitleStreams, currentAudioIndex, currentQuality,
    currentSubtitleTrackIndex, playbackSpeed, autoQuality, autoQualityLabel,
    availableQualities, chapters, mediaId, mediaType, streamToken,
    title, brightness, skipForwardSeconds, skipBackwardSeconds,
    seasons, currentSeasonId, episodes, currentEpisodeId, subtitleStyle,
  } = ctx;

  const [visible, setVisible] = useState(true);
  const [openPanel, setOpenPanel] = useState<"settings" | "episodes" | "chapters" | null>(null);
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
    if (!settingsOpen) scheduleHide();
  }, [settingsOpen, scheduleHide]);

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

  function togglePlay() {
    const video = videoRef.current;
    if (!video) return;
    if (video.paused) void video.play();
    else video.pause();
  }

  function handleSeek(e: React.ChangeEvent<HTMLInputElement>) {
    const targetAbsolute = Number(e.target.value);
    if (onSeekRequest) {
      onSeekRequest(targetAbsolute);
    } else {
      const video = videoRef.current;
      if (video) video.currentTime = targetAbsolute;
    }
  }

  function skipForward() {
    const target = Math.min(effectiveDuration, absoluteTime + skipForwardSeconds);
    if (onSeekRequest) onSeekRequest(target);
    else if (videoRef.current) videoRef.current.currentTime += skipForwardSeconds;
  }

  function skipBackward() {
    const target = Math.max(0, absoluteTime - skipBackwardSeconds);
    if (onSeekRequest) onSeekRequest(target);
    else if (videoRef.current) videoRef.current.currentTime -= skipBackwardSeconds;
  }

  const prevEpisode = (() => {
    if (!episodes || !currentEpisodeId) return null;
    const idx = episodes.findIndex((e) => e.id === currentEpisodeId);
    if (idx <= 0) return null;
    return episodes[idx - 1];
  })();

  function handleVolume(e: React.ChangeEvent<HTMLInputElement>) {
    const video = videoRef.current;
    if (!video) return;
    video.volume = Number(e.target.value);
  }

  function togglePip() {
    const video = videoRef.current;
    if (!video || !document.pictureInPictureEnabled) return;
    if (document.pictureInPictureElement) {
      void document.exitPictureInPicture();
    } else {
      void video.requestPictureInPicture();
    }
  }

  function toggleMute() {
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
        resetHideTimer();
      }, 200);
    }
  }

  const handleChapterSeek = (timeSec: number) => {
    if (onSeekRequest) onSeekRequest(timeSec);
    else if (videoRef.current) videoRef.current.currentTime = timeSec;
  };

  const currentChapterIndex = chapters.findIndex(
    (ch) => absoluteTime * 1000 >= ch.startTimeMs && absoluteTime * 1000 < ch.endTimeMs,
  );
  const currentChapter = currentChapterIndex >= 0 ? chapters[currentChapterIndex] : null;

  function skipToNextChapter() {
    const nextIdx = currentChapterIndex >= 0 ? currentChapterIndex + 1 : 0;
    if (nextIdx < chapters.length) handleChapterSeek(chapters[nextIdx].startTimeMs / 1000);
  }

  function skipToPrevChapter() {
    const threshold = 3;
    if (currentChapterIndex > 0 && currentChapter) {
      const elapsed = absoluteTime - currentChapter.startTimeMs / 1000;
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
          <ArrowLeft size={24} />
        </button>
        {title && <span className="text-[var(--overlay-text)] text-sm font-medium truncate">{title}</span>}
      </div>

      {/* bottom controls */}
      <div
        className={`absolute bottom-0 left-0 right-0 pt-8 flex flex-col gap-2 transition-opacity-fast duration-300 ${visible ? "opacity-100" : "opacity-0 pointer-events-none"}`}
        style={{ background: "linear-gradient(to top, var(--overlay-bg-strong), transparent)", paddingBottom: "max(1rem, env(safe-area-inset-bottom, 1rem))", paddingLeft: "max(1rem, env(safe-area-inset-left, 1rem))", paddingRight: "max(1rem, env(safe-area-inset-right, 1rem))" }}
        data-controls
      >
        {/* seek bar */}
        <PlayerProgressBar
          absoluteTime={absoluteTime}
          effectiveDuration={effectiveDuration}
          bufferedEnd={bufferedEnd}
          transcodeOffset={transcodeOffset}
          chapters={chapters}
          mediaId={mediaId}
          mediaType={mediaType}
          streamToken={streamToken}
          onSeek={handleSeek}
          onChapterSeek={handleChapterSeek}
        />

        {/* current chapter label */}
        {currentChapter && (
          <div className="text-xs truncate px-1" style={{ color: "var(--text-muted)" }}>
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
              <SkipBack size={22} fill="currentColor" />
            </button>
          )}

          {/* previous chapter */}
          {chapters.length > 0 && currentChapterIndex > 0 && (
            <button
              onClick={skipToPrevChapter}
              aria-label={t("player.controls.chapters")}
              className="text-[var(--overlay-text)] hover:text-[var(--accent)] transition-colors-fast p-1 focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:ring-offset-1 rounded"
            >
              <ChevronFirst size={22} />
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
              <Pause size={28} fill="currentColor" />
            ) : (
              <Play size={28} fill="currentColor" />
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
              <ChevronLast size={22} />
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
                <SkipForward size={22} fill="currentColor" />
              </button>
            );
          })()}

          {/* time */}
          <span className="text-[var(--overlay-text)] text-xs tabular-nums">
            {formatTime(absoluteTime)} / {formatTime(effectiveDuration)}
          </span>

          {/* volume */}
          <PlayerVolumeControl
            volume={volume}
            muted={muted}
            onToggleMute={toggleMute}
            onVolumeChange={handleVolume}
          />

          <div className="flex-1" />

          {/* Chapters panel */}
          {chapters.length > 0 && (
            <ChaptersPanel
              chapters={chapters}
              currentTimeSec={absoluteTime}
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

          {/* Picture-in-Picture */}
          {typeof document !== "undefined" && document.pictureInPictureEnabled && (
            <button
              onClick={togglePip}
              aria-label={isPip ? t("player.controls.exitPip") : t("player.controls.pip")}
              title={isPip ? `${t("player.controls.exitPip")} (P)` : `${t("player.controls.pip")} (P)`}
              className="text-[var(--overlay-text)] hover:text-[var(--accent)] transition-colors-fast p-1 focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:ring-offset-1 rounded"
            >
              {isPip ? (
                <PictureInPicture2 size={20} />
              ) : (
                <PictureInPicture size={20} />
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
              <Minimize size={20} />
            ) : (
              <Maximize size={20} />
            )}
          </button>
        </div>
      </div>
    </div>
  );
}
