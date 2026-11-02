import { useEffect } from "react";
import type { V1StreamInfo } from "@evey/api";
import { getDisplayModes, pickDisplayMode, setPreferredDisplayMode } from "@evey/media-caps";
import { logger } from "./log";

const log = logger("display");

// A transcode may halve a high frame rate (a 60 fps source on a 30 fps
// rung): above this, only the source played as it is says what the screen
// will receive.
const TRANSCODE_TRUSTED_FPS = 30.5;

/**
 * The frame rate the player will put on screen, or null when it cannot be
 * told: the source video track's, for a transcode only while it is low
 * enough that no rung changes it.
 */
export function playedFrameRate(info: V1StreamInfo | null | undefined): number | null {
  if (!info) return null;
  const video = info.mediaStreams?.find((s) => s.type === "MEDIA_STREAM_TYPE_VIDEO");
  const fps = video?.frameRate;
  if (!fps || !Number.isFinite(fps) || fps <= 0) return null;
  if (info.playMethod !== "PLAY_METHOD_DIRECT_PLAY" && fps > TRANSCODE_TRUSTED_FPS) return null;
  return fps;
}

/**
 * Puts the display in the mode the video's frame rate needs while the player
 * is mounted (25 fps -> 50 Hz), and gives the choice back to the system when
 * it leaves. Without it a 25 fps episode on a 59.94 Hz output holds its frames
 * two refreshes, then three: the motion stutters.
 */
export function useDisplayFrameRate(info: V1StreamInfo | null | undefined) {
  const fps = playedFrameRate(info);
  useEffect(() => {
    if (fps == null) return;
    const display = getDisplayModes();
    if (!display) return;
    const mode = pickDisplayMode(display.modes, display.currentId, fps);
    if (!mode) return;
    log.info(`display ${mode.width}x${mode.height}@${mode.refreshRate.toFixed(3)} for ${fps.toFixed(3)} fps`);
    setPreferredDisplayMode(mode.id);
    return () => {
      setPreferredDisplayMode(0);
    };
  }, [fps]);
}
