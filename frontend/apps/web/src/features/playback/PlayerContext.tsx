import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import type { AudioStream } from "./AudioTrackSelector";
import type { SubtitleStream } from "./SubtitleSelector";
import type { Chapter } from "@/hooks/useChapters";
import type { SubtitleStyle } from "@/hooks/usePlaybackPrefs";
import type { EpisodeNavItem, SeasonNavItem } from "./PlayerEpisodesPanel";
import type { QualityLevel } from "@evey/api";
import { useMediaSession } from "./hooks/useMediaSession";
import { useVolumeBoost } from "./hooks/useVolumeBoost";
import { getVolumeBoost, setVolumeBoost } from "@/hooks/usePlaybackPrefs";

// Stable empty defaults: inline `= []` default params allocate a fresh array on
// every render, which would churn the `value` memo (and every consumer) on each
// position tick now that the time state lives in its own context.
const EMPTY_CHAPTERS: Chapter[] = [];
const EMPTY_SEASONS: SeasonNavItem[] = [];
const EMPTY_EPISODES: EpisodeNavItem[] = [];

// effectiveDuration prefers a server-known duration (accurate for transcodes,
// where the <video> element only reports the current segment window) and falls
// back to the element's own duration. Exported so the real logic is unit-tested
// directly instead of being reimplemented in the test.
export function computeEffectiveDuration(knownDurationSec: number | undefined, duration: number): number {
  return knownDurationSec && knownDurationSec > 0
    ? knownDurationSec
    : duration && isFinite(duration) && duration > 0
      ? duration
      : 0;
}

export interface PlayerState {
  videoRef: React.RefObject<HTMLVideoElement | null>;
  playing: boolean;
  duration: number;
  volume: number;
  muted: boolean;
  /** Amplification above 100% (1..2). 1 = no boost; the base 0-100% is `volume`. */
  boost: number;
  /** Set the >100% amplification factor (1..2); persisted and applied live. */
  setBoost: (boost: number) => void;
  fullscreen: boolean;
  isPip: boolean;

  knownDurationSec?: number;
  transcodeOffset: number;
  effectiveDuration: number;

  audioStreams: AudioStream[];
  subtitleStreams: SubtitleStream[];
  currentAudioIndex: number;
  currentQuality: number;
  currentSubtitleTrackIndex: number | null;
  playbackSpeed: number;
  autoQuality: boolean;
  autoQualityLabel?: string;
  availableQualities?: readonly QualityLevel[];

  chapters: Chapter[];
  mediaId?: string;
  mediaType?: string;
  streamToken?: string;

  title?: string;
  brightness: number;
  skipForwardSeconds: number;
  skipBackwardSeconds: number;

  seasons: SeasonNavItem[];
  currentSeasonId: string;
  episodes: EpisodeNavItem[];
  currentEpisodeId: string;

  subtitleStyle?: SubtitleStyle;
  subtitleOffsetMs?: number;
  onSubtitleOffsetChange?: (ms: number) => void;
  audioOffsetMs?: number;
  onAudioOffsetChange?: (ms: number) => void;
  audioOffsetAvailable?: boolean;

}

const PlayerContext = createContext<PlayerState | null>(null);

export function usePlayerContext(): PlayerState {
  const ctx = useContext(PlayerContext);
  if (!ctx) throw new Error("usePlayerContext must be used within PlayerProvider");
  return ctx;
}

// High-frequency playback position, split into its OWN context so the ~4×/sec
// timeupdate tick re-renders only the components that actually display the
// position (the seek bar, the time label) - not the whole control bar with its
// buttons and panels. PlayerState (above) deliberately no longer carries these.
export interface PlayerTime {
  currentTime: number;
  absoluteTime: number;
  bufferedEnd: number;
}

const PlayerTimeContext = createContext<PlayerTime | null>(null);
// A ref mirror of the same values, for event handlers that need the *current*
// position at call time (skip ±, chapter skip, "play on") without subscribing
// to the tick and re-rendering every 250ms.
const PlayerTimeRefContext = createContext<React.RefObject<PlayerTime> | null>(null);

export function usePlayerTime(): PlayerTime {
  const ctx = useContext(PlayerTimeContext);
  if (!ctx) throw new Error("usePlayerTime must be used within PlayerProvider");
  return ctx;
}

export function usePlayerTimeRef(): React.RefObject<PlayerTime> {
  const ctx = useContext(PlayerTimeRefContext);
  if (!ctx) throw new Error("usePlayerTimeRef must be used within PlayerProvider");
  return ctx;
}

export interface PlayerProviderProps {
  videoRef: React.RefObject<HTMLVideoElement | null>;
  knownDurationSec?: number;
  transcodeOffset?: number;
  audioStreams: AudioStream[];
  subtitleStreams: SubtitleStream[];
  currentAudioIndex: number;
  currentQuality: number;
  currentSubtitleTrackIndex: number | null;
  playbackSpeed?: number;
  autoQuality?: boolean;
  autoQualityLabel?: string;
  availableQualities?: readonly QualityLevel[];
  chapters?: Chapter[];
  mediaId?: string;
  mediaType?: string;
  streamToken?: string;
  title?: string;
  brightness?: number;
  skipForwardSeconds?: number;
  skipBackwardSeconds?: number;
  seasons?: SeasonNavItem[];
  currentSeasonId?: string;
  episodes?: EpisodeNavItem[];
  currentEpisodeId?: string;
  subtitleStyle?: SubtitleStyle;
  subtitleOffsetMs?: number;
  onSubtitleOffsetChange?: (ms: number) => void;
  audioOffsetMs?: number;
  onAudioOffsetChange?: (ms: number) => void;
  audioOffsetAvailable?: boolean;

  /** Navigate to an episode (for Media Session prev/next). Optional. */
  onPlayEpisode?: (episodeId: string) => void;
  /** The film is playing on another device (Cast / Remote Playback fling):
   *  the OS transport must not reach this element. */
  remotePlaybackActive?: boolean;
  children: React.ReactNode;
}

export function PlayerProvider({
  videoRef,
  knownDurationSec,
  transcodeOffset = 0,
  audioStreams,
  subtitleStreams,
  currentAudioIndex,
  currentQuality,
  currentSubtitleTrackIndex,
  playbackSpeed = 1,
  autoQuality = false,
  autoQualityLabel,
  availableQualities,
  chapters = EMPTY_CHAPTERS,
  mediaId,
  mediaType,
  streamToken,
  title,
  brightness = 1,
  skipForwardSeconds = 30,
  skipBackwardSeconds = 30,
  seasons = EMPTY_SEASONS,
  currentSeasonId = "",
  episodes = EMPTY_EPISODES,
  currentEpisodeId = "",
  subtitleStyle,
  subtitleOffsetMs,
  onSubtitleOffsetChange,
  audioOffsetMs,
  onAudioOffsetChange,
  audioOffsetAvailable,
  onPlayEpisode,
  remotePlaybackActive = false,
  children,
}: PlayerProviderProps) {
  const [playing, setPlaying] = useState(false);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [bufferedEnd, setBufferedEnd] = useState(0);
  const [volume, setVolume] = useState(1);
  const [muted, setMuted] = useState(false);
  const [boost, setBoostState] = useState<number>(() => getVolumeBoost());
  const [fullscreen, setFullscreen] = useState(false);
  const [isPip, setIsPip] = useState(false);
  const lastTimeUpdate = useRef(0);

  const applyBoost = useVolumeBoost(videoRef);
  // Read-latest mirror so the "play" listener can re-arm the persisted boost
  // without the boost value being a dependency of the (element-scoped) effect.
  const boostRef = useRef(boost);
  boostRef.current = boost;

  const setBoost = useCallback(
    (next: number) => {
      const clamped = Math.min(2, Math.max(1, next));
      setBoostState(clamped);
      setVolumeBoost(clamped);
      applyBoost(clamped);
    },
    [applyBoost],
  );

  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;

    function onPlay() {
      setPlaying(true);
      // Re-arm a persisted boost once playback (a user gesture) starts: the
      // AudioContext can only resume from suspended inside a gesture, so we
      // can't apply the stored >100% level at mount.
      if (boostRef.current > 1) applyBoost(boostRef.current);
    }
    function onPause() { setPlaying(false); }
    function onTimeUpdate() {
      const now = performance.now();
      if (now - lastTimeUpdate.current < 250) return;
      lastTimeUpdate.current = now;
      setCurrentTime(video!.currentTime);
    }
    function onDurationChange() {
      const d = video!.duration;
      if (d && isFinite(d) && d > 0) setDuration(d);
    }
    function onVolumeChange() { setVolume(video!.volume); setMuted(video!.muted); }
    function onFullscreenChange() { setFullscreen(!!document.fullscreenElement); }
    function onEnterPip() { setIsPip(true); }
    function onLeavePip() { setIsPip(false); }
    function onProgress() {
      const b = video!.buffered;
      if (b.length > 0) setBufferedEnd(b.end(b.length - 1));
    }
    function onWheel(e: WheelEvent) {
      // Wheel over the video adjusts volume across the full 0-200% range: the
      // 0-100% part rides the native element volume, the 100-200% part the boost
      // gain (same split as the slider). Registered passive:false so
      // preventDefault actually stops the page scrolling under the cursor.
      e.preventDefault();
      const delta = e.deltaY < 0 ? 0.05 : -0.05;
      const current = boostRef.current > 1 ? boostRef.current : video!.volume;
      const next = Math.min(2, Math.max(0, current + delta));
      video!.volume = Math.min(1, next);
      if (next > 0 && video!.muted) video!.muted = false;
      setBoost(next > 1 ? next : 1);
    }

    // Wheel is caught on the player container (the video's parent) so scrolling
    // over the controls overlay counts too, matching the old handler's reach.
    const wheelTarget: HTMLElement = video.parentElement ?? video;

    video.addEventListener("play", onPlay);
    video.addEventListener("pause", onPause);
    video.addEventListener("timeupdate", onTimeUpdate);
    video.addEventListener("durationchange", onDurationChange);
    video.addEventListener("volumechange", onVolumeChange);
    video.addEventListener("progress", onProgress);
    wheelTarget.addEventListener("wheel", onWheel, { passive: false });
    video.addEventListener("enterpictureinpicture", onEnterPip);
    video.addEventListener("leavepictureinpicture", onLeavePip);
    document.addEventListener("fullscreenchange", onFullscreenChange);

    setPlaying(!video.paused);
    setCurrentTime(video.currentTime);
    const d = video.duration;
    if (d && isFinite(d) && d > 0) setDuration(d);
    setVolume(video.volume);

    return () => {
      video.removeEventListener("play", onPlay);
      video.removeEventListener("pause", onPause);
      video.removeEventListener("timeupdate", onTimeUpdate);
      video.removeEventListener("durationchange", onDurationChange);
      video.removeEventListener("volumechange", onVolumeChange);
      video.removeEventListener("progress", onProgress);
      wheelTarget.removeEventListener("wheel", onWheel);
      video.removeEventListener("enterpictureinpicture", onEnterPip);
      video.removeEventListener("leavepictureinpicture", onLeavePip);
      document.removeEventListener("fullscreenchange", onFullscreenChange);
    };
  }, [videoRef, applyBoost, setBoost]);

  const effectiveDuration = computeEffectiveDuration(knownDurationSec, duration);

  const absoluteTime = transcodeOffset + currentTime;

  // Mirror the live position into a ref so handlers can read it at call time
  // without subscribing to the tick. Written during render (read-latest pattern).
  const timeRef = useRef<PlayerTime>({ currentTime, absoluteTime, bufferedEnd });
  timeRef.current = { currentTime, absoluteTime, bufferedEnd };

  const timeValue = useMemo<PlayerTime>(
    () => ({ currentTime, absoluteTime, bufferedEnd }),
    [currentTime, absoluteTime, bufferedEnd],
  );

  // Publish to the OS Media Session (lock screen / Now Playing). Feature-
  // detected and additive - a no-op where unsupported.
  useMediaSession({
    videoRef,
    title,
    artworkUrl: mediaId ? `/api/v1/media/${mediaId}/artwork/poster` : undefined,
    playing,
    position: absoluteTime,
    duration: effectiveDuration,
    playbackRate: playbackSpeed,
    transcodeOffset,
    skipForwardSeconds,
    skipBackwardSeconds,
    episodes,
    currentEpisodeId,
    onPlayEpisode,
    remoteActive: remotePlaybackActive,
  });

  const value = useMemo<PlayerState>(() => ({
    videoRef,
    playing,
    duration,
    volume,
    muted,
    boost,
    setBoost,
    fullscreen,
    isPip,
    knownDurationSec,
    transcodeOffset,
    effectiveDuration,
    audioStreams,
    subtitleStreams,
    currentAudioIndex,
    currentQuality,
    currentSubtitleTrackIndex,
    playbackSpeed,
    autoQuality,
    autoQualityLabel,
    availableQualities,
    chapters,
    mediaId,
    mediaType,
    streamToken,
    title,
    brightness,
    skipForwardSeconds,
    skipBackwardSeconds,
    seasons,
    currentSeasonId,
    episodes,
    currentEpisodeId,
    subtitleStyle,
    subtitleOffsetMs,
    onSubtitleOffsetChange,
    audioOffsetMs,
    onAudioOffsetChange,
    audioOffsetAvailable,
  }), [
    videoRef,
    playing,
    duration,
    volume,
    muted,
    boost,
    setBoost,
    fullscreen,
    isPip,
    knownDurationSec,
    transcodeOffset,
    effectiveDuration,
    audioStreams,
    subtitleStreams,
    currentAudioIndex,
    currentQuality,
    currentSubtitleTrackIndex,
    playbackSpeed,
    autoQuality,
    autoQualityLabel,
    availableQualities,
    chapters,
    mediaId,
    mediaType,
    streamToken,
    title,
    brightness,
    skipForwardSeconds,
    skipBackwardSeconds,
    seasons,
    currentSeasonId,
    episodes,
    currentEpisodeId,
    subtitleStyle,
    subtitleOffsetMs,
    onSubtitleOffsetChange,
    audioOffsetMs,
    onAudioOffsetChange,
    audioOffsetAvailable,
  ]);

  return (
    <PlayerContext.Provider value={value}>
      <PlayerTimeRefContext.Provider value={timeRef}>
        <PlayerTimeContext.Provider value={timeValue}>
          {children}
        </PlayerTimeContext.Provider>
      </PlayerTimeRefContext.Provider>
    </PlayerContext.Provider>
  );
}
