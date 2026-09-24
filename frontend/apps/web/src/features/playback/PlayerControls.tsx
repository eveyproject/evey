import { memo, useCallback, useEffect, useRef, useState } from "react";
import { ArrowLeft, SkipBack, SkipForward, Pause, Play, Minimize, Maximize, PictureInPicture, PictureInPicture2, ChevronFirst, ChevronLast, Loader, MoreHorizontal, Cast } from "lucide-react";
import { RemotePlaybackMenu } from "./RemotePlaybackMenu";
import { startPlayback } from "./hooks/hlsPlayerUtils";
import { CONTROLS_FADE_MS, fadeClasses, useFadeParked } from "./hooks/useFadeParked";
import { useTranslation } from "react-i18next";
import type { SubtitleStyle } from "@/hooks/usePlaybackPrefs";
import type { SleepTimerState } from "@/hooks/useSleepTimer";
import { PlayerVolumeControl } from "./PlayerVolumeControl";
import { EpisodesPanel } from "./PlayerEpisodesPanel";
import type { EpisodeNavItem, SeasonNavItem } from "./PlayerEpisodesPanel";
import { SettingsPanel } from "./PlayerSettingsPanel";
import { StreamInfoPanel } from "./StreamInfoPanel";
import type { ServedVariant } from "@evey/client";
import { usePlayerContext, usePlayerTimeRef } from "./PlayerContext";
import { LiveProgressBar, LiveTimeLabel, LiveChaptersPanel, useActiveChapterIndex } from "./PlayerLiveControls";
import { useDocumentPiP } from "./hooks/useDocumentPiP";
import type { V1StreamInfo } from "@evey/api";
import { ICON } from "@/constants/iconSizes";

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
  /** Fires whenever the controls auto-hide/reveal, so sibling overlays (the
   *  top-right Watch-together / cast cluster) can fade in lockstep with the bars. */
  onVisibilityChange?: (visible: boolean) => void;
  /** Exposes the "reveal + restart the auto-hide timer" action so a sibling
   *  overlay (the top-right cluster) can keep the controls up while it's hovered,
   *  exactly like hovering the bars does. */
  revealRef?: React.MutableRefObject<(() => void) | null>;
  pipToggleRef?: React.MutableRefObject<(() => void) | null>;
  castConnected?: boolean;
  castDeviceName?: string | null;
  castPlayback?: { playerState: string | null; currentTime: number; duration: number; isPaused: boolean; isMediaLoaded: boolean; volume: number; isMuted: boolean };
  onCastPlayPause?: () => void;
  onCastSeek?: (timeSec: number) => boolean;
  onCastSetVolume?: (level: number) => void;
  onCastSetMuted?: (muted: boolean) => void;
  /** Whether a Chromecast path exists in this browser at all (Cast SDK ready,
   *  or the Chrome-Android Remote Playback API present). Gates the "Cast"
   *  section of the unified remote-playback menu. */
  castAvailable?: boolean;
  /** Chrome-Android Remote Playback fling session connected (separate from
   *  the Cast SDK's castConnected). */
  remoteConnected?: boolean;
  /** Single tap action for the Cast entry, already routed to the right
   *  transport (Cast SDK vs Remote Playback fling) by the caller. */
  onCastTrigger?: () => void;
  streamInfo?: V1StreamInfo;
  /** True when the stream was negotiated for the client-side AC-3/E-AC-3 decode
   *  (video-only master + out-of-band rendition). Says nothing about which relay
   *  leg is playing - see wasmAudioLeg. */
  wasmAudioActive?: boolean;
  /** The relay leg actually feeding the speakers, when the host knows it:
   *  "aac" (decoder booting), "wasm", "aac-latched". Passed to the info panel,
   *  which must not announce a client decode over the AAC transcode. */
  wasmAudioLeg?: string | null;
  /** The variant playing right now, so the info panel follows every quality change. */
  servedVariant?: ServedVariant | null;
  /** AUDIO-relative ordinal of the selected track, so the info panel reports IT. */
  audioTrackIndex?: number | null;
  /** True while the HLS pipeline is stalled waiting for segments. */
  isBuffering?: boolean;
  /** True when the video element is actively playing (not paused / not yet started). */
  isPlaying?: boolean;
  autoplayNext?: boolean;
  onAutoplayNextChange?: (enabled: boolean) => void;
  sleepTimerState?: SleepTimerState | null;
  onSleepTimerMinutes?: (minutes: number) => void;
  onSleepTimerEndOfEpisode?: () => void;
  onSleepTimerCancel?: () => void;
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
  onVisibilityChange,
  revealRef,
  pipToggleRef,
  castConnected,
  castDeviceName,
  castPlayback,
  onCastPlayPause,
  onCastSeek,
  onCastSetVolume,
  onCastSetMuted,
  castAvailable,
  remoteConnected,
  onCastTrigger,
  streamInfo,
  wasmAudioActive,
  wasmAudioLeg,
  servedVariant,
  audioTrackIndex,
  isBuffering,
  isPlaying,
  autoplayNext,
  onAutoplayNextChange,
  sleepTimerState,
  onSleepTimerMinutes,
  onSleepTimerEndOfEpisode,
  onSleepTimerCancel,
}: PlayerControlsProps) {
  const { t } = useTranslation();
  const ctx = usePlayerContext();
  const timeRef = usePlayerTimeRef();
  const docPiP = useDocumentPiP();
  const {
    videoRef, playing: localPlaying, volume: localVolume, muted: localMuted, boost, setBoost, fullscreen, isPip,
    effectiveDuration: localDuration, transcodeOffset,
    audioStreams, subtitleStreams, currentAudioIndex, currentQuality,
    currentSubtitleTrackIndex, playbackSpeed, autoQuality, autoQualityLabel,
    availableQualities, chapters, mediaId, mediaType,
    title, brightness, skipForwardSeconds, skipBackwardSeconds,
    seasons, currentSeasonId, episodes, currentEpisodeId, subtitleStyle,
    subtitleOffsetMs, onSubtitleOffsetChange, audioOffsetMs, onAudioOffsetChange, audioOffsetAvailable,
  } = ctx;

  // When casting, use cast playback state instead of local video state.
  // The device plays a complete timeline (file / VOD manifest), so its
  // currentTime IS the absolute position the seek bar / time display expect.
  const isCasting = castConnected && castPlayback?.isMediaLoaded;
  // The picture is on ANOTHER device: a Cast session (SDK or server-brokered)
  // or a Remote Playback fling. Nothing in this file may start the local
  // element while it holds - not even for the instant it takes the structural
  // guard in HLSPlayer to re-pause it, because that instant is audible and
  // visible (reported 2026-09-19). Deliberately wider than `isCasting`: a
  // session whose media has not loaded yet owns the playback all the same,
  // and that window is exactly when a tap used to fall through to the local
  // video.
  const remoteActive = !!castConnected || !!remoteConnected;
  const playing = isCasting ? !castPlayback.isPaused : localPlaying;
  const effectiveDuration = localDuration;
  const muted = isCasting ? (castPlayback.isMuted ?? false) : localMuted;
  // The slider's continuous 0-2 value: the native 0-100% comes from the element
  // (localVolume), the >100% zone from the boost gain. Casting can't amplify, so
  // it stays on the device's own 0-1 level.
  const volume = isCasting ? (castPlayback.volume ?? 1) : (boost > 1 ? boost : localVolume);

  // The live position no longer lives in this component's render (it's split
  // into PlayerTimeContext, consumed by the Live* leaves below) so the control
  // bar stops re-rendering 4×/sec. When casting it comes from the device prop;
  // pass that down to the leaves as castTimeSec. Handlers read the *current*
  // position at call time via the ref, never from render state.
  const castTimeSec = isCasting ? castPlayback.currentTime : undefined;
  const currentAbsoluteTime = () =>
    isCasting ? castPlayback.currentTime : timeRef.current.absoluteTime;
  const currentChapterIndex = useActiveChapterIndex(chapters, videoRef, transcodeOffset, castTimeSec);

  const [visible, setVisible] = useState(true);
  const parked = useFadeParked(visible, CONTROLS_FADE_MS);
  const [openPanel, setOpenPanel] = useState<"settings" | "episodes" | "chapters" | "streaminfo" | "remote" | null>(null);
  // On phones the secondary controls collapse behind a "⋯" button; this tracks
  // whether that overflow row is revealed. Inline (no overflow) from sm up.
  const [overflowOpen, setOverflowOpen] = useState(false);
  const settingsOpen = openPanel === "settings";
  const episodesOpen = openPanel === "episodes";
  const chaptersOpen = openPanel === "chapters";

  useEffect(() => {
    onPanelOpenChange?.(openPanel !== null);
  }, [openPanel, onPanelOpenChange]);

  // Mirror the visibility to the parent so overlays outside this component (the
  // top-right Watch-together / cast cluster) fade with the bars instead of
  // staying pinned on screen during playback.
  useEffect(() => {
    onVisibilityChange?.(visible);
  }, [visible, onVisibilityChange]);

  // Collapse the overflow row whenever the controls hide, so it doesn't reappear
  // already-open the next time the user taps to show the controls.
  useEffect(() => {
    if (!visible) setOverflowOpen(false);
  }, [visible]);

  useEffect(() => {
    if (closePanelRef) closePanelRef.current = () => setOpenPanel(null);
    return () => { if (closePanelRef) closePanelRef.current = null; };
  }, [closePanelRef]);

  const hideTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const clickTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const rootRef = useRef<HTMLDivElement | null>(null);

  // Keyboard focus parked on a control pins the bar visible: hiding it mid
  // arrow-keys breaks WCAG focus visibility (the volume slider literally
  // vanished under the user's fingers). Mouse-click focus does NOT pin, so
  // the hover UX keeps its 3s fade. Modality comes from our own listeners
  // rather than ":focus-visible", whose verdict for range inputs diverges
  // between engines (Chrome flags them focus-visible even on click).
  const inputModality = useRef<"pointer" | "keyboard">("pointer");
  const keyboardFocusWithin = useCallback(() => {
    const ae = document.activeElement;
    return (
      inputModality.current === "keyboard" &&
      ae instanceof HTMLElement &&
      rootRef.current?.contains(ae) === true
    );
  }, []);

  const scheduleHide = useCallback(() => {
    if (hideTimer.current) clearTimeout(hideTimer.current);
    // While the film plays on another device the bars STAY. Hiding chrome over
    // a moving picture is the point of the auto-hide; here the picture is a
    // frozen local frame and the bar is the only thing saying a TV is playing
    // and the only way to drive it - fading it after 3s of stillness left the
    // viewer in front of a dead screenshot ("je ne vois pas la barre de
    // contrôle", 2026-09-19).
    if (remoteActive) return;
    hideTimer.current = setTimeout(function tick() {
      if (keyboardFocusWithin()) {
        // Re-arm instead of hiding under the keyboard; re-check every 3s so
        // the bar still fades once focus leaves the overlay.
        hideTimer.current = setTimeout(tick, 3000);
        return;
      }
      setVisible(false);
    }, 3000);
  }, [keyboardFocusWithin, remoteActive]);

  // Entering (or leaving) a remote session re-evaluates the bars: pinned while
  // the device holds the picture, back to the 3s fade once it hands it back.
  useEffect(() => {
    if (remoteActive) {
      if (hideTimer.current) clearTimeout(hideTimer.current);
      setVisible(true);
    } else {
      scheduleHide();
    }
  }, [remoteActive, scheduleHide]);

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

  // Expose the Document-PiP-aware toggle to the parent so the "p" keyboard
  // shortcut uses it too. No deps: re-assign every render so the ref always
  // holds the latest closure (docPiP.available/toggle can change).
  useEffect(() => {
    if (!pipToggleRef) return;
    pipToggleRef.current = togglePip;
    return () => { pipToggleRef.current = null; };
  });

  const resetHideTimer = useCallback(() => {
    setVisible(true);
    // Don't auto-hide while ANY side panel is open - previously only the
    // settings panel was guarded, so a hover over the controls with the
    // episodes/chapters panel open would queue a hide that fired 3s
    // later and caused a brief flicker before the panel-open effect
    // forced visibility back to true.
    if (openPanel === null) scheduleHide();
  }, [openPanel, scheduleHide]);

  useEffect(() => {
    if (revealRef) revealRef.current = resetHideTimer;
    return () => { if (revealRef) revealRef.current = null; };
  }, [revealRef, resetHideTimer]);

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
    if (remoteActive) {
      // Drive the device that holds the picture, or do nothing at all (a
      // fling exposes no transport here). Never the local element.
      if (onCastPlayPause) onCastPlayPause();
      return;
    }
    const video = videoRef.current;
    if (!video) return;
    // A play() still in flight rejects with AbortError the moment a pause()
    // interrupts it - expected, not an error worth surfacing.
    if (video.paused) void startPlayback(video, wasmAudioActive);
    else video.pause();
  }

  // onCastSeek returns false when the cast session can't actually seek
  // (dead/stale session). In that case we must fall through to the
  // local player - otherwise a Chromecast that bugged out leaves every
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
    const val = Number(e.target.value); // 0..2
    if (isCasting && onCastSetVolume) {
      // The receiver device tops out at 100%; never hand it a boosted level.
      onCastSetVolume(Math.min(1, val));
      return;
    }
    const video = videoRef.current;
    if (!video) return;
    // Split the continuous 0-2 slider: 0-1 rides the native element volume,
    // 1-2 pins the element at full and amplifies the rest through the gain node.
    video.volume = Math.min(1, val);
    setBoost(val > 1 ? val : 1);
    // The muted-autoplay fallback leaves video.muted=true even after the user
    // grabs the slider; without this unmute, dragging the bar visibly raises
    // the slider but produces no sound - they have to also click the volume
    // icon. Interpret a >0 drag as "I want sound now" and clear the mute.
    if (val > 0 && video.muted) {
      video.muted = false;
    }
  }

  function togglePip() {
    // Prefer Document PiP (Chrome 116+): brings the WHOLE player UI into
    // the floating window - progress bar, subtitles, chapter skip, next
    // episode prompt - not just the bare <video>. Falls back to the
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

  // Click on the bare video surface: single = play/pause, double = fullscreen,
  // and with a panel open the click only closes it. This handler lives on a
  // DEDICATED surface element UNDER the bars (see the render below), never on
  // the root that wraps the bars: the bars and their panels are siblings of the
  // surface, so a click on the settings gear, on a menu entry or on the seek
  // bar cannot reach it - not by bubbling, not through a DOM-ancestry filter
  // that a portal or a re-render could defeat.
  function handleSurfaceClick() {
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
        if (remoteActive) {
          // Same rule as togglePlay: the surface tap follows the picture.
          if (onCastPlayPause) {
            onCastPlayPause();
            onActionFlash?.(playing ? "pause" : "play");
          }
        } else {
          const video = videoRef.current;
          if (video) {
            if (video.paused) {
              // The SAME resume the play button uses. Calling video.play()
              // straight skipped startPlayback's autoplay ladder, so the two
              // ways of resuming behaved differently - and the surface is the
              // one people actually use.
              void startPlayback(video, wasmAudioActive);
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
      ref={rootRef}
      className="absolute inset-0"
      onMouseMove={resetHideTimer}
      // Pressing keys on a focused control reveals the (possibly faded) bar,
      // pushes the hide timer back and flips the modality to keyboard so the
      // focus-pinning in scheduleHide engages; pointer activity flips it back.
      onKeyDown={() => { inputModality.current = "keyboard"; resetHideTimer(); }}
      onPointerDownCapture={() => { inputModality.current = "pointer"; }}
      onFocus={resetHideTimer}
      style={{ cursor: visible ? "default" : "none" }}
    >
      {/* Video surface: the only element whose click toggles play/pause or
          fullscreen. The bars below in the tree paint over it and stop their
          own clicks here, so no control can double as a surface tap. */}
      <div
        data-player-surface
        className="absolute inset-0"
        onClick={handleSurfaceClick}
      />

      {/* top bar */}
      <div
        className={`absolute top-0 left-0 right-0 flex items-center gap-3 py-3 [--fade-duration:300ms] ${fadeClasses(visible, parked)}`}
        style={{ background: "linear-gradient(to bottom, var(--overlay-bg-strong), transparent)", paddingLeft: "max(1rem, env(safe-area-inset-left, 1rem))", paddingRight: "max(1rem, env(safe-area-inset-right, 1rem))" }}
        data-controls
        onClick={(e) => e.stopPropagation()}
      >
        <button
          onClick={onClose}
          aria-label={t("common.back")}
          className="text-[var(--overlay-text)] hover:text-[var(--accent)] transition-colors-fast focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:ring-offset-1 rounded"
        >
          <ArrowLeft size={ICON.xl}  aria-hidden="true" />
        </button>
        {title && <span className="text-[var(--overlay-text)] text-sm font-medium truncate">{title}</span>}
        {isCasting && castDeviceName && (
          <span className="ml-auto flex items-center gap-1.5 text-[var(--accent)] text-xs font-medium">
            <Cast size={ICON.sm} />
            {castDeviceName}
          </span>
        )}
      </div>

      {/* bottom controls */}
      <div
        className={`absolute bottom-0 left-0 right-0 pt-8 flex flex-col gap-2 [--fade-duration:300ms] ${fadeClasses(visible, parked)}`}
        style={{ background: "linear-gradient(to top, var(--overlay-bg-strong), transparent)", paddingBottom: "max(1rem, env(safe-area-inset-bottom, 1rem))", paddingLeft: "max(1rem, env(safe-area-inset-left, 1rem))", paddingRight: "max(1rem, env(safe-area-inset-right, 1rem))" }}
        data-controls
        onClick={(e) => e.stopPropagation()}
      >
        {/* seek bar - subscribes to PlayerTimeContext on its own */}
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

        <div className="relative flex items-center gap-2 sm:gap-3">
          {/* previous episode */}
          {prevEpisode && onEpisodeChange && (
            <button
              onClick={() => onEpisodeChange(prevEpisode.id)}
              aria-label={t("player.controls.previousEpisode")}
              className="text-[var(--overlay-text)] hover:text-[var(--accent)] transition-colors-fast p-1 focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:ring-offset-1 rounded"
              title={prevEpisode.title ? `E${prevEpisode.indexNumber} - ${prevEpisode.title}` : undefined}
            >
              <SkipBack size={ICON.xl} fill="currentColor"  aria-hidden="true" />
            </button>
          )}

          {/* previous chapter */}
          {chapters.length > 0 && currentChapterIndex > 0 && (
            <button
              onClick={skipToPrevChapter}
              aria-label={t("player.controls.chapters")}
              className="text-[var(--overlay-text)] hover:text-[var(--accent)] transition-colors-fast p-1 focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:ring-offset-1 rounded"
            >
              <ChevronFirst size={ICON.xl}  aria-hidden="true" />
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
              <Pause size={ICON["2xl"]} fill="currentColor"  aria-hidden="true" />
            ) : (
              <Play size={ICON["2xl"]} fill="currentColor"  aria-hidden="true" />
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
              <ChevronLast size={ICON.xl}  aria-hidden="true" />
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
                <SkipForward size={ICON.xl} fill="currentColor"  aria-hidden="true" />
              </button>
            );
          })()}

          {/* time - subscribes to PlayerTimeContext on its own */}
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
            <Loader size={ICON.md} className="animate-spin text-white/70 shrink-0" aria-hidden="true" />
          )}

          <div className="flex-1" />

          {/* Overflow toggle - phones only. Taps reveal the secondary controls
              as a floating row above the bar; from sm up they stay inline. */}
          <button
            onClick={() => setOverflowOpen((v) => !v)}
            aria-label={t("player.controls.more")}
            aria-expanded={overflowOpen}
            className="sm:hidden text-[var(--overlay-text)] hover:text-[var(--accent)] transition-colors-fast p-1 focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:ring-offset-1 rounded"
          >
            <MoreHorizontal size={ICON.lg} aria-hidden="true" />
          </button>

          {/* Secondary controls. Below sm they live behind the ⋯ toggle as a
              floating row; from sm up the sm:* utilities reset them to inline. */}
          <div
            className={`items-center gap-2 sm:gap-3 sm:flex sm:static sm:bottom-auto sm:right-auto sm:mb-0 sm:p-0 sm:bg-transparent sm:shadow-none ${overflowOpen ? "flex absolute bottom-full right-0 mb-2 px-3 py-2 rounded-[var(--radius-md)] bg-[var(--overlay-bg-strong)] shadow-2xl" : "hidden"}`}
          >
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
            subtitleOffsetMs={subtitleOffsetMs}
            onSubtitleOffsetChange={onSubtitleOffsetChange}
            audioOffsetMs={audioOffsetMs}
            onAudioOffsetChange={onAudioOffsetChange}
            audioOffsetAvailable={audioOffsetAvailable}
            brightness={brightness}
            onBrightnessChange={onBrightnessChange}
            availableQualities={availableQualities}
            autoplayNext={autoplayNext}
            onAutoplayNextChange={onAutoplayNextChange}
            sleepTimerState={sleepTimerState}
            onSleepTimerMinutes={onSleepTimerMinutes}
            onSleepTimerEndOfEpisode={onSleepTimerEndOfEpisode}
            onSleepTimerCancel={onSleepTimerCancel}
          />

          {/* Stream Info - surfaces the backend's playback decision (method,
              codecs, transcode reasons), parity with the mobile app. */}
          {streamInfo && (
            <StreamInfoPanel
              streamInfo={streamInfo}
              wasmAudioActive={wasmAudioActive}
              wasmAudioLeg={wasmAudioLeg}
              served={servedVariant}
              audioTrackIndex={audioTrackIndex}
              open={openPanel === "streaminfo"}
              onToggle={() => setOpenPanel(openPanel === "streaminfo" ? null : "streaminfo")}
            />
          )}

          {/* Remote playback - single "send elsewhere" menu, unifying the
              Chromecast SDK toggle and the server-brokered Play On device
              list (Jellyfin-style), instead of two separate affordances. */}
          {mediaId && (
            <RemotePlaybackMenu
              mediaId={mediaId}
              getPositionMs={() => Math.floor(currentAbsoluteTime() * 1000)}
              open={openPanel === "remote"}
              onToggle={() => setOpenPanel(openPanel === "remote" ? null : "remote")}
              castAvailable={castAvailable}
              castConnected={castConnected}
              remoteConnected={remoteConnected}
              castDeviceName={castDeviceName}
              onCastTrigger={onCastTrigger}
            />
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
                <PictureInPicture2 size={ICON.lg}  aria-hidden="true" />
              ) : (
                <PictureInPicture size={ICON.lg}  aria-hidden="true" />
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
              <Minimize size={ICON.lg}  aria-hidden="true" />
            ) : (
              <Maximize size={ICON.lg}  aria-hidden="true" />
            )}
          </button>
          </div>
        </div>
      </div>
    </div>
  );
}

export const PlayerControls = memo(PlayerControlsInner);
