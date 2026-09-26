import { useEffect, useState } from "react";
import { useAutoMarkWatched } from "@evey/client";

interface AutoMarkWatchedProps {
  mediaId: string;
  videoRef: React.RefObject<HTMLVideoElement | null>;
  /** Optional override when the video element only sees a transcoded duration. */
  knownDurationSec?: number;
  /**
   * In transcode mode the `<video>` element's currentTime is local to the
   * current transcode segment (starts at 0 after every seek-restart). The
   * absolute playhead position the server cares about is `transcodeOffset
   * + video.currentTime`. Pass the offset ref so the 90% check uses the
   * absolute position; otherwise a user who seeks to 80% then plays 5min
   * never trips the threshold (5min / fullDuration is far from 90%).
   */
  transcodeOffsetRef?: React.RefObject<number>;
}

/**
 * The web player's feed for useAutoMarkWatched (the rule every player shares):
 * the <video> element's own clock, read on timeupdate without re-rendering the
 * player. Render-less; mount alongside the player.
 */
export function AutoMarkWatched({ mediaId, videoRef, knownDurationSec, transcodeOffsetRef }: AutoMarkWatchedProps) {
  const [clock, setClock] = useState({ pos: 0, dur: 0 });
  useAutoMarkWatched(mediaId, clock.pos, clock.dur);

  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    const tick = () => {
      const dur = knownDurationSec && knownDurationSec > 0 ? knownDurationSec : video.duration;
      setClock({ pos: (transcodeOffsetRef?.current ?? 0) + video.currentTime, dur: Number.isFinite(dur) ? dur : 0 });
    };
    video.addEventListener("timeupdate", tick);
    return () => video.removeEventListener("timeupdate", tick);
  }, [mediaId, videoRef, knownDurationSec, transcodeOffsetRef]);

  return null;
}
