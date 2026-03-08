import { useEffect, useRef, useState } from "react";
import Hls from "hls.js";
import { useAuthStore } from "@evey/store";
import type { V1StreamInfo } from "@evey/api/src/generated/playback.schemas";
import type { AudioStream } from "./AudioTrackSelector";
import type { SubtitleStream } from "./SubtitleSelector";
import { PlayerControls } from "./PlayerControls";
import { ProgressReporter } from "./ProgressReporter";

export type { AudioStream, SubtitleStream };

interface HLSPlayerProps {
  streamInfo: V1StreamInfo;
  sessionId: string;
  mediaId: string;
  startPositionMs?: number;
  onEnded?: () => void;
  audioStreams?: AudioStream[];
  subtitleStreams?: SubtitleStream[];
  title?: string;
  onClose?: () => void;
}

export function HLSPlayer({
  streamInfo,
  sessionId,
  mediaId,
  startPositionMs = 0,
  onEnded,
  audioStreams = [],
  subtitleStreams = [],
  title,
  onClose,
}: HLSPlayerProps) {
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const hlsRef = useRef<InstanceType<typeof Hls> | null>(null);
  // Track current playback position across reloads (cleanup resets video.currentTime to 0)
  const positionRef = useRef(startPositionMs / 1000);
  // Start in transcode mode when there's no HLS URL (always the case with current backend)
  const [transcodeMode, setTranscodeMode] = useState(!streamInfo.transcodeUrl);
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
        void video.play();
      });
      hlsInstance.on(Hls.Events.AUDIO_TRACKS_UPDATED, (_, data) => {
        if (data.audioTracks.length === 0) setTranscodeMode(true);
      });
    } else if (streamInfo.directPlayUrl) {
      video.src = streamInfo.directPlayUrl;
      video.currentTime = startPositionMs / 1000;
      void video.play();
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
    video.addEventListener("ended", handleEnded);
    video.addEventListener("canplay", handleCanPlay);

    return () => {
      video.removeEventListener("ended", handleEnded);
      video.removeEventListener("canplay", handleCanPlay);
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
    const url = `/api/v1/media/${mediaId}/transcode?${params.toString()}`;

    function onTimeUpdate() { positionRef.current = video!.currentTime; }
    function onMetadata() {
      if (videoRef.current) videoRef.current.currentTime = savedTime;
    }
    function handleEnded() { onEnded?.(); }
    video.addEventListener("timeupdate", onTimeUpdate);
    video.addEventListener("loadedmetadata", onMetadata, { once: true });
    video.addEventListener("ended", handleEnded);
    video.src = url;
    void video.play();

    return () => {
      video.removeEventListener("timeupdate", onTimeUpdate);
      video.removeEventListener("ended", handleEnded);
      video.removeEventListener("loadedmetadata", onMetadata);
      video.src = "";
    };
  }, [transcodeMode, mediaId, accessToken, startPositionMs, onEnded, currentTrackIndex, currentQuality]);

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
      <video ref={videoRef} className="w-full h-full" playsInline crossOrigin="anonymous" />
      <ProgressReporter sessionId={sessionId} videoRef={videoRef} />
      <PlayerControls
        videoRef={videoRef}
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
      />
    </div>
  );
}
