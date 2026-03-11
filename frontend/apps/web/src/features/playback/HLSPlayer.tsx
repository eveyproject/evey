import { useCallback, useEffect, useRef, useState } from "react";
import Hls from "hls.js";
import { useAuthStore } from "@evey/store";
import type { V1StreamInfo } from "@evey/api/src/generated/playback.schemas";
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
import { canPlayHevc } from "@/utils/codec";

const SPEED_OPTIONS = [0.5, 0.75, 1, 1.25, 1.5, 2];

export type { AudioStream, SubtitleStream, EpisodeNavItem, SeasonNavItem };

interface HLSPlayerProps {
  streamInfo: V1StreamInfo;
  sessionId: string;
  mediaId: string;
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
}

export function HLSPlayer({
  streamInfo,
  sessionId,
  mediaId,
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
}: HLSPlayerProps) {
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const hlsRef = useRef<InstanceType<typeof Hls> | null>(null);
  // Track current playback position across reloads (cleanup resets video.currentTime to 0)
  const positionRef = useRef(startPositionMs / 1000);
  const knownDurationSecRef = useRef(knownDurationMs ? knownDurationMs / 1000 : 0);
  knownDurationSecRef.current = knownDurationMs ? knownDurationMs / 1000 : 0;
  // Start in transcode mode when there's no HLS URL (always the case with current backend)
  const [transcodeMode, setTranscodeMode] = useState(!streamInfo.transcodeUrl);
  const [overlayTime, setOverlayTime] = useState(0);
  const [overlayDuration, setOverlayDuration] = useState(0);
  const defaultAudio = audioStreams.find((s) => s.isDefault) ?? audioStreams[0];
  const [currentAudioIndex, setCurrentAudioIndex] = useState(defaultAudio?.index ?? 0);
  const [currentTrackIndex, setCurrentTrackIndex] = useState(defaultAudio?.trackIndex ?? 0);
  const [currentQuality, setCurrentQuality] = useState(0); // 0 = original
  const [autoQuality, setAutoQuality] = useState(true);
  const [currentSubtitle, setCurrentSubtitle] = useState<number | null>(null);
  const [playbackSpeed, setPlaybackSpeed] = useState(1);
  const [showShortcuts, setShowShortcuts] = useState(false);
  const playbackSpeedRef = useRef(playbackSpeed);
  playbackSpeedRef.current = playbackSpeed;
  const subtitleStreamsRef = useRef(subtitleStreams);
  subtitleStreamsRef.current = subtitleStreams;
  const currentSubtitleRef = useRef(currentSubtitle);
  currentSubtitleRef.current = currentSubtitle;
  const accessToken = useAuthStore((s) => s.accessToken);
  const { data: playbackSettings } = usePlaybackSettings();
  const skipFwd = playbackSettings?.skipForwardSeconds ?? 10;
  const skipBwd = playbackSettings?.skipBackwardSeconds ?? 10;
  const skipFwdRef = useRef(skipFwd);
  skipFwdRef.current = skipFwd;
  const skipBwdRef = useRef(skipBwd);
  skipBwdRef.current = skipBwd;
  // Offset applied when we restart transcode from a non-zero position
  const transcodeOffsetRef = useRef(0);

  // HLS path — only used when transcodeUrl is available
  useEffect(() => {
    const video = videoRef.current;
    if (!video || transcodeMode) return;

    let hlsInstance: InstanceType<typeof Hls> | null = null;

    if (streamInfo.transcodeUrl && Hls.isSupported()) {
      hlsInstance = new Hls();
      hlsRef.current = hlsInstance;
      hlsInstance.loadSource(streamInfo.transcodeUrl);
      hlsInstance.attachMedia(video);
      hlsInstance.on(Hls.Events.MANIFEST_PARSED, () => {
        video.currentTime = startPositionMs / 1000;
        video.play().catch(() => {});
      });
      hlsInstance.on(Hls.Events.AUDIO_TRACKS_UPDATED, (_, data) => {
        if (data.audioTracks.length === 0) setTranscodeMode(true);
      });
    } else if (streamInfo.directPlayUrl) {
      video.src = streamInfo.directPlayUrl;
      video.currentTime = startPositionMs / 1000;
      video.play().catch(() => {});
    }

    function handleCanPlay() {
      setTimeout(() => {
        const v = videoRef.current;
        if (!v || transcodeMode) return;
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        const tracks = (v as any).audioTracks as { length: number } | undefined;
        if (tracks && tracks.length === 0) setTranscodeMode(true);
      }, 3000);
    }
    function handleEnded() { onEnded?.(); }
    function handleError() {
      const e = video!.error;
      if (e) console.error("Video error:", e.code, e.message);
    }
    video.addEventListener("ended", handleEnded);
    video.addEventListener("canplay", handleCanPlay);
    video.addEventListener("error", handleError);

    return () => {
      video.removeEventListener("ended", handleEnded);
      video.removeEventListener("canplay", handleCanPlay);
      video.removeEventListener("error", handleError);
      if (hlsInstance) {
        hlsInstance.destroy();
        hlsRef.current = null;
      }
    };
  }, [streamInfo, startPositionMs, onEnded, transcodeMode]);

  // Transcode path — reload video with current audio track, quality and startTime
  const [transcodeStartTime, setTranscodeStartTime] = useState(startPositionMs / 1000);

  useEffect(() => {
    if (!transcodeMode) return;
    const video = videoRef.current;
    if (!video) return;

    if (hlsRef.current) {
      hlsRef.current.destroy();
      hlsRef.current = null;
    }

    const offset = transcodeStartTime;
    transcodeOffsetRef.current = offset;

    const token = accessToken ?? "";
    const params = new URLSearchParams();
    if (token) params.set("token", token);
    params.set("audioTrack", String(currentTrackIndex));
    if (currentQuality > 0) params.set("videoScale", String(currentQuality));
    if (canPlayHevc) params.set("remux", "1");
    if (offset > 0) params.set("startTime", String(offset));
    const url = `/api/v1/media/${mediaId}/transcode?${params.toString()}`;

    function onTimeUpdate() {
      positionRef.current = transcodeOffsetRef.current + video!.currentTime;
    }
    function handleEnded() { onEnded?.(); }
    function handleError() {
      const e = video!.error;
      if (e) console.error("Video error:", e.code, e.message);
    }
    video.addEventListener("timeupdate", onTimeUpdate);
    video.addEventListener("ended", handleEnded);
    video.addEventListener("error", handleError);
    video.src = url;
    video.play().catch(() => {});

    return () => {
      video.removeEventListener("timeupdate", onTimeUpdate);
      video.removeEventListener("ended", handleEnded);
      video.removeEventListener("error", handleError);
      video.src = "";
    };
  }, [transcodeMode, mediaId, accessToken, onEnded, currentTrackIndex, currentQuality, transcodeStartTime]);

  // Subtitle track management — imperatively add/remove <track> element
  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;

    const existing = video.querySelector("track[data-dynamic]");
    if (existing) existing.remove();

    if (currentSubtitle === null || !accessToken) return;

    const track = document.createElement("track");
    track.setAttribute("data-dynamic", "1");
    track.kind = "subtitles";
    track.label = "Selected";
    track.src = `/api/v1/media/${mediaId}/subtitle/${currentSubtitle}?token=${accessToken}`;
    track.default = true;
    video.appendChild(track);

    // Force browser to load the track (dynamically added tracks aren't auto-loaded)
    if (track.track) track.track.mode = "showing";

    // Ensure all other text tracks are hidden
    for (let i = 0; i < video.textTracks.length; i++) {
      if (video.textTracks[i] !== track.track) {
        video.textTracks[i].mode = "hidden";
      }
    }
  }, [currentSubtitle, mediaId, accessToken, transcodeStartTime, currentTrackIndex, currentQuality]);

  // Sync playbackRate with playbackSpeed state
  useEffect(() => {
    const video = videoRef.current;
    if (video) video.playbackRate = playbackSpeed;
  }, [playbackSpeed]);

  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      const video = videoRef.current;
      if (!video) return;

      switch (e.key) {
        case " ":
        case "k":
          e.preventDefault();
          if (video.paused) video.play().catch(() => {});
          else video.pause();
          break;
        case "ArrowLeft":
        case "j":
          e.preventDefault();
          seekRef.current(Math.max(0, positionRef.current - skipBwdRef.current));
          break;
        case "ArrowRight":
        case "l": {
          e.preventDefault();
          const dur = knownDurationSecRef.current > 0 ? knownDurationSecRef.current : (video.duration && isFinite(video.duration)) ? video.duration : 0;
          seekRef.current(Math.min(dur || video.currentTime + skipFwdRef.current, positionRef.current + skipFwdRef.current));
          break;
        }
        case "ArrowUp":
          e.preventDefault();
          video.volume = Math.min(1, video.volume + 0.1);
          break;
        case "ArrowDown":
          e.preventDefault();
          video.volume = Math.max(0, video.volume - 0.1);
          break;
        case "m":
          video.muted = !video.muted;
          break;
        case "f": {
          const container = video.parentElement;
          if (!container) break;
          if (!document.fullscreenElement) void container.requestFullscreen();
          else void document.exitFullscreen();
          break;
        }
        case "c": {
          const subs = subtitleStreamsRef.current;
          const cur = currentSubtitleRef.current;
          if (subs.length === 0) break;
          if (cur === null) setCurrentSubtitle(subs[0].trackIndex);
          else setCurrentSubtitle(null);
          break;
        }
        case "?":
          e.preventDefault();
          setShowShortcuts((p) => !p);
          break;
        case "<":
        case ",": {
          e.preventDefault();
          const curSpeed = playbackSpeedRef.current;
          const idx = SPEED_OPTIONS.indexOf(curSpeed);
          if (idx > 0) setPlaybackSpeed(SPEED_OPTIONS[idx - 1]);
          break;
        }
        case ">":
        case ".": {
          e.preventDefault();
          const curSpeed = playbackSpeedRef.current;
          const idx = SPEED_OPTIONS.indexOf(curSpeed);
          if (idx < SPEED_OPTIONS.length - 1) setPlaybackSpeed(SPEED_OPTIONS[idx + 1]);
          break;
        }
        case "0": case "1": case "2": case "3": case "4":
        case "5": case "6": case "7": case "8": case "9": {
          e.preventDefault();
          const dur = knownDurationSecRef.current > 0 ? knownDurationSecRef.current : (video.duration && isFinite(video.duration)) ? video.duration : 0;
          if (dur > 0) seekRef.current(dur * (Number(e.key) / 10));
          break;
        }
      }
    }

    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, []);

  // Track time for next-episode overlay (absolute position)
  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    function onTime() {
      const v = videoRef.current;
      if (!v) return;
      const absTime = transcodeOffsetRef.current + v.currentTime;
      setOverlayTime(absTime);
      const d = knownDurationSecRef.current;
      if (d > 0) setOverlayDuration(d);
      else {
        const vd = v.duration;
        if (vd && isFinite(vd) && vd > 0) setOverlayDuration(transcodeOffsetRef.current + vd);
      }
    }
    video.addEventListener("timeupdate", onTime);
    return () => video.removeEventListener("timeupdate", onTime);
  }, []);

  // Adaptive quality: measure buffer fill rate and adjust quality dynamically
  const QUALITY_LEVELS = [0, 1080, 720, 480, 360];
  const slowCountRef = useRef(0);
  const lastBufferCheckRef = useRef({ time: 0, buffered: 0 });
  const qualityStableRef = useRef(0);

  useEffect(() => {
    if (!transcodeMode || !autoQuality) {
      slowCountRef.current = 0;
      lastBufferCheckRef.current = { time: 0, buffered: 0 };
      qualityStableRef.current = 0;
      return;
    }
    const video = videoRef.current;
    if (!video) return;

    lastBufferCheckRef.current = { time: Date.now(), buffered: 0 };
    qualityStableRef.current = Date.now();

    const interval = setInterval(() => {
      const v = videoRef.current;
      if (!v || v.paused) return;
      const b = v.buffered;
      if (b.length === 0) return;

      const now = Date.now();
      const currentBuffered = b.end(b.length - 1);
      const prev = lastBufferCheckRef.current;
      const elapsed = (now - prev.time) / 1000;
      lastBufferCheckRef.current = { time: now, buffered: currentBuffered };

      if (elapsed < 3) return;
      const fillRate = (currentBuffered - prev.buffered) / elapsed;

      const curIdx = QUALITY_LEVELS.indexOf(currentQuality);

      if (fillRate < 0.8) {
        slowCountRef.current++;
        if (slowCountRef.current >= 2 && curIdx < QUALITY_LEVELS.length - 1) {
          const pos = positionRef.current;
          setCurrentQuality(QUALITY_LEVELS[curIdx + 1]);
          setTranscodeStartTime(pos);
          slowCountRef.current = 0;
          qualityStableRef.current = Date.now();
        }
      } else {
        slowCountRef.current = 0;
        if (fillRate > 2.5 && curIdx > 0 && (now - qualityStableRef.current) > 30000) {
          const pos = positionRef.current;
          setCurrentQuality(QUALITY_LEVELS[curIdx - 1]);
          setTranscodeStartTime(pos);
          qualityStableRef.current = Date.now();
        }
      }
    }, 5000);

    return () => clearInterval(interval);
  }, [transcodeMode, autoQuality, currentQuality]);

  const autoQualityLabel = autoQuality
    ? (currentQuality === 0 ? "Original" : `${currentQuality}p`)
    : undefined;

  const nextEpisode = (() => {
    if (!episodes || !currentEpisodeId) return null;
    const idx = episodes.findIndex((e) => e.id === currentEpisodeId);
    if (idx < 0 || idx >= episodes.length - 1) return null;
    return episodes[idx + 1];
  })();

  const handleNextEpisode = useCallback((id: string) => {
    onEpisodeChange?.(id);
  }, [onEpisodeChange]);

  const handleSeekRequest = useCallback((absoluteTimeSec: number) => {
    const video = videoRef.current;
    if (!video || !transcodeMode) {
      if (video) video.currentTime = absoluteTimeSec;
      return;
    }
    const offset = transcodeOffsetRef.current;
    const relativeTime = absoluteTimeSec - offset;
    const buffered = video.buffered;
    const isBuffered = buffered.length > 0 && relativeTime >= 0 && relativeTime <= buffered.end(buffered.length - 1) + 2;
    if (isBuffered) {
      video.currentTime = relativeTime;
    } else {
      setTranscodeStartTime(absoluteTimeSec);
    }
  }, [transcodeMode]);

  const seekRef = useRef(handleSeekRequest);
  seekRef.current = handleSeekRequest;

  function handleQualityChange(quality: number) {
    setAutoQuality(false);
    setCurrentQuality(quality);
    if (transcodeMode) setTranscodeStartTime(positionRef.current);
  }

  function handleAutoQualityToggle(auto: boolean) {
    setAutoQuality(auto);
  }

  function handleAudioChange(index: number) {
    const stream = audioStreams.find((s) => s.index === index);
    setCurrentAudioIndex(index);
    if (stream) setCurrentTrackIndex(stream.trackIndex);

    if (!transcodeMode) {
      if (hlsRef.current) {
        hlsRef.current.audioTrack = stream?.trackIndex ?? 0;
        return;
      }
      const video = videoRef.current;
      if (!video) return;
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const tracks = (video as any).audioTracks as { length: number; [i: number]: { id: string; enabled: boolean } } | undefined;
      if (!tracks) return;
      for (let i = 0; i < tracks.length; i++) {
        tracks[i].enabled = tracks[i].id === String(index);
      }
    } else {
      setTranscodeStartTime(positionRef.current);
    }
  }

  return (
    <div className="relative w-full h-full bg-black">
      <video ref={videoRef} className="w-full h-full" playsInline />
      <ProgressReporter sessionId={sessionId} videoRef={videoRef} />
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
        onSubtitleChange={setCurrentSubtitle}
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
        onSeekRequest={handleSeekRequest}
        transcodeOffset={transcodeOffsetRef.current}
        skipForwardSeconds={skipFwd}
        skipBackwardSeconds={skipBwd}
      />
      {showShortcuts && <KeyboardShortcutsOverlay onClose={() => setShowShortcuts(false)} skipForwardSeconds={skipFwd} skipBackwardSeconds={skipBwd} />}
    </div>
  );
}
