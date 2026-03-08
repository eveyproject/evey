import { useEffect, useRef, useState } from "react";
import Hls from "hls.js";
import type { V1StreamInfo } from "@evey/api/src/generated/playback.schemas";
import type { AudioStream } from "./AudioTrackSelector";
import type { SubtitleStream } from "./SubtitleSelector";
import { PlayerControls } from "./PlayerControls";
import { ProgressReporter } from "./ProgressReporter";

export type { AudioStream, SubtitleStream };

interface HLSPlayerProps {
  streamInfo: V1StreamInfo;
  sessionId: string;
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
  startPositionMs = 0,
  onEnded,
  audioStreams = [],
  subtitleStreams = [],
  title,
  onClose,
}: HLSPlayerProps) {
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const [currentAudioIndex, setCurrentAudioIndex] = useState(
    audioStreams.find((s) => s.isDefault)?.index ?? audioStreams[0]?.index ?? 0,
  );

  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;

    let hlsInstance: InstanceType<typeof Hls> | null = null;

    if (streamInfo.transcodeUrl && Hls.isSupported()) {
      hlsInstance = new Hls();
      hlsInstance.loadSource(streamInfo.transcodeUrl);
      hlsInstance.attachMedia(video);
      hlsInstance.on(Hls.Events.MANIFEST_PARSED, () => {
        video.currentTime = startPositionMs / 1000;
        void video.play();
      });
    } else if (streamInfo.directPlayUrl) {
      video.src = streamInfo.directPlayUrl;
      video.currentTime = startPositionMs / 1000;
      void video.play();
    }

    function handleEnded() {
      onEnded?.();
    }
    video.addEventListener("ended", handleEnded);

    return () => {
      video.removeEventListener("ended", handleEnded);
      if (hlsInstance) {
        hlsInstance.destroy();
        hlsInstance = null;
      }
    };
  }, [streamInfo, startPositionMs, onEnded]);

  function handleAudioChange(index: number) {
    setCurrentAudioIndex(index);
    const video = videoRef.current;
    if (!video) return;
    // audioTracks may not exist in all browsers
    // audioTracks is non-standard; cast to any to avoid TS DOM lib conflicts
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const tracks = (video as any).audioTracks as { length: number; [i: number]: { id: string; enabled: boolean } } | undefined;
    if (!tracks) return;
    for (let i = 0; i < tracks.length; i++) {
      tracks[i].enabled = tracks[i].id === String(index);
    }
  }

  return (
    <div className="relative w-full h-full bg-black">
      <video ref={videoRef} className="w-full h-full" playsInline />
      <ProgressReporter sessionId={sessionId} videoRef={videoRef} />
      <PlayerControls
        videoRef={videoRef}
        audioStreams={audioStreams}
        subtitleStreams={subtitleStreams}
        title={title}
        onClose={onClose}
        currentAudioIndex={currentAudioIndex}
        onAudioChange={handleAudioChange}
      />
    </div>
  );
}
