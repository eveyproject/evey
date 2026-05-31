import { createContext, useContext, useEffect, useMemo, useRef, useState } from "react";
import type { AudioStream } from "./AudioTrackSelector";
import type { SubtitleStream } from "./SubtitleSelector";
import type { Chapter } from "@/hooks/useChapters";
import type { SubtitleStyle } from "@/hooks/usePlaybackPrefs";
import type { EpisodeNavItem, SeasonNavItem } from "./PlayerEpisodesPanel";
import { useMediaSession } from "./hooks/useMediaSession";

export interface PlayerState {
  videoRef: React.RefObject<HTMLVideoElement | null>;
  playing: boolean;
  currentTime: number;
  duration: number;
  bufferedEnd: number;
  volume: number;
  muted: boolean;
  fullscreen: boolean;
  isPip: boolean;

  knownDurationSec?: number;
  transcodeOffset: number;
  effectiveDuration: number;
  absoluteTime: number;

  audioStreams: AudioStream[];
  subtitleStreams: SubtitleStream[];
  currentAudioIndex: number;
  currentQuality: number;
  currentSubtitleTrackIndex: number | null;
  playbackSpeed: number;
  autoQuality: boolean;
  autoQualityLabel?: string;
  availableQualities?: number[];

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
}

const PlayerContext = createContext<PlayerState | null>(null);

export function usePlayerContext(): PlayerState {
  const ctx = useContext(PlayerContext);
  if (!ctx) throw new Error("usePlayerContext must be used within PlayerProvider");
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
  availableQualities?: number[];
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
  chapters = [],
  mediaId,
  mediaType,
  streamToken,
  title,
  brightness = 1,
  skipForwardSeconds = 30,
  skipBackwardSeconds = 30,
  seasons = [],
  currentSeasonId = "",
  episodes = [],
  currentEpisodeId = "",
  subtitleStyle,
  children,
}: PlayerProviderProps) {
  const [playing, setPlaying] = useState(false);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [bufferedEnd, setBufferedEnd] = useState(0);
  const [volume, setVolume] = useState(1);
  const [muted, setMuted] = useState(false);
  const [fullscreen, setFullscreen] = useState(false);
  const [isPip, setIsPip] = useState(false);
  const lastTimeUpdate = useRef(0);

  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;

    function onPlay() { setPlaying(true); }
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

    video.addEventListener("play", onPlay);
    video.addEventListener("pause", onPause);
    video.addEventListener("timeupdate", onTimeUpdate);
    video.addEventListener("durationchange", onDurationChange);
    video.addEventListener("volumechange", onVolumeChange);
    video.addEventListener("progress", onProgress);
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
      video.removeEventListener("enterpictureinpicture", onEnterPip);
      video.removeEventListener("leavepictureinpicture", onLeavePip);
      document.removeEventListener("fullscreenchange", onFullscreenChange);
    };
  }, [videoRef]);

  const effectiveDuration = (knownDurationSec && knownDurationSec > 0)
    ? knownDurationSec
    : (duration && isFinite(duration) && duration > 0) ? duration : 0;

  const absoluteTime = transcodeOffset + currentTime;

  // Publish to the OS Media Session (lock screen / Now Playing). Feature-
  // detected and additive — a no-op where unsupported.
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
  });

  const value = useMemo<PlayerState>(() => ({
    videoRef,
    playing,
    currentTime,
    duration,
    bufferedEnd,
    volume,
    muted,
    fullscreen,
    isPip,
    knownDurationSec,
    transcodeOffset,
    effectiveDuration,
    absoluteTime,
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
  }), [
    videoRef,
    playing,
    currentTime,
    duration,
    bufferedEnd,
    volume,
    muted,
    fullscreen,
    isPip,
    knownDurationSec,
    transcodeOffset,
    effectiveDuration,
    absoluteTime,
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
  ]);

  return (
    <PlayerContext.Provider value={value}>
      {children}
    </PlayerContext.Provider>
  );
}
