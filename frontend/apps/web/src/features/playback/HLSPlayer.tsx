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
import type { Chapter } from "@/hooks/useChapters";

const canPlayHevc = (() => {
  const v = document.createElement("video");
  return !!(v.canPlayType('video/mp4; codecs="hvc1"') || v.canPlayType('video/mp4; codecs="hev1"'));
})();

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
  const [currentSubtitle, setCurrentSubtitle] = useState<number | null>(null);
  const accessToken = useAuthStore((s) => s.accessToken);

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

  // Transcode path — reload video with current audio track and quality whenever they change
  useEffect(() => {
    if (!transcodeMode) return;
    const video = videoRef.current;
    if (!video) return;

    if (hlsRef.current) {
      hlsRef.current.destroy();
      hlsRef.current = null;
    }

    // positionRef is updated by timeupdate — survives cleanup resetting video.currentTime to 0
    const savedTime = positionRef.current;

    const token = accessToken ?? "";
    const params = new URLSearchParams();
    if (token) params.set("token", token);
    params.set("audioTrack", String(currentTrackIndex));
    if (currentQuality > 0) params.set("videoScale", String(currentQuality));
    if (canPlayHevc) params.set("remux", "1");
    const url = `/api/v1/media/${mediaId}/transcode?${params.toString()}`;

    function onTimeUpdate() { positionRef.current = video!.currentTime; }
    function onMetadata() {
      if (videoRef.current) videoRef.current.currentTime = savedTime;
    }
    function handleEnded() { onEnded?.(); }
    function handleError() {
      const e = video!.error;
      if (e) console.error("Video error:", e.code, e.message);
    }
    video.addEventListener("timeupdate", onTimeUpdate);
    video.addEventListener("loadedmetadata", onMetadata, { once: true });
    video.addEventListener("ended", handleEnded);
    video.addEventListener("error", handleError);
    video.src = url;
    video.play().catch(() => {});

    return () => {
      video.removeEventListener("timeupdate", onTimeUpdate);
      video.removeEventListener("ended", handleEnded);
      video.removeEventListener("loadedmetadata", onMetadata);
      video.removeEventListener("error", handleError);
      video.src = "";
    };
  }, [transcodeMode, mediaId, accessToken, onEnded, currentTrackIndex, currentQuality]);

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
    track.src = `/api/v1/media/${mediaId}/subtitle/${currentSubtitle}?token=${accessToken}`;
    track.default = true;
    video.appendChild(track);

    track.addEventListener("load", () => {
      for (let i = 0; i < video.textTracks.length; i++) {
        video.textTracks[i].mode = video.textTracks[i] === (track as HTMLTrackElement).track ? "showing" : "hidden";
      }
    }, { once: true });
  }, [currentSubtitle, mediaId, accessToken]);

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
          e.preventDefault();
          video.currentTime = Math.max(0, video.currentTime - 10);
          break;
        case "ArrowRight": {
          e.preventDefault();
          const dur = knownDurationSecRef.current > 0 ? knownDurationSecRef.current : (video.duration && isFinite(video.duration)) ? video.duration : 0;
          video.currentTime = Math.min(dur || 0, video.currentTime + 10);
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
      }
    }

    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, []);

  // Track time for next-episode overlay
  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    function onTime() {
      const v = videoRef.current;
      if (!v) return;
      setOverlayTime(v.currentTime);
      const d = v.duration;
      if (d && isFinite(d) && d > 0) setOverlayDuration(d);
      else if (knownDurationSecRef.current > 0) setOverlayDuration(knownDurationSecRef.current);
    }
    video.addEventListener("timeupdate", onTime);
    return () => video.removeEventListener("timeupdate", onTime);
  }, []);

  const nextEpisode = (() => {
    if (!episodes || !currentEpisodeId) return null;
    const idx = episodes.findIndex((e) => e.id === currentEpisodeId);
    if (idx < 0 || idx >= episodes.length - 1) return null;
    return episodes[idx + 1];
  })();

  const handleNextEpisode = useCallback((id: string) => {
    onEpisodeChange?.(id);
  }, [onEpisodeChange]);

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
    }
    // In transcodeMode, currentTrackIndex state change triggers the effect above
  }

  return (
    <div className="relative w-full h-full bg-black">
      <video ref={videoRef} className="w-full h-full" playsInline />
      <ProgressReporter sessionId={sessionId} videoRef={videoRef} />
      <SkipIntroButton
        filePath={filePath}
        currentTime={overlayTime}
        onSkip={(t) => { if (videoRef.current) videoRef.current.currentTime = t; }}
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
        onQualityChange={setCurrentQuality}
        currentSubtitleTrackIndex={currentSubtitle}
        onSubtitleChange={setCurrentSubtitle}
        seasons={seasons}
        currentSeasonId={currentSeasonId}
        episodes={episodes}
        currentEpisodeId={currentEpisodeId}
        onSeasonChange={onSeasonChange}
        onEpisodeChange={onEpisodeChange}
        chapters={chapters}
      />
    </div>
  );
}
