import { useCallback, useEffect, useRef } from "react";
import type { VideoPlayer } from "expo-video";
import { setAudioBoost } from "../lib/audioBoost";
import {
  CastContext, useCastStateHook, useRemoteMediaClientHook,
  useMediaStatusHook, CastStateEnum,
} from "./useChromecast";

interface UseCastSessionArgs {
  player: VideoPlayer;
  volume: number;
  streamUrl: string | null;
  mediaId: string;
  title: string | undefined;
  // startTime for castLoadMedia - not listed in the dispatch prompt's prop
  // list, but castLoadMedia's body (moved verbatim) reads it; without it the
  // cast session would always start from 0 instead of the local player's
  // current position. Flagged as a deviation in the report.
  currentTime: number;
  fallbackDurationSec: number;
}

export function useCastSession({
  player,
  volume,
  streamUrl,
  mediaId,
  title,
  currentTime,
  fallbackDurationSec,
}: UseCastSessionArgs) {
  const castState = useCastStateHook();
  const castClient = useRemoteMediaClientHook();
  const castMediaStatus = useMediaStatusHook();
  const castConnected = castState === CastStateEnum.CONNECTED;
  const castCurrentTime = castMediaStatus?.streamPosition ?? 0;
  const castDuration = castMediaStatus?.mediaInfo?.streamDuration ?? fallbackDurationSec;
  const castIsPlaying = castMediaStatus?.playerState === 2; // MediaPlayerState.PLAYING

  const muteLocalPlayer = useCallback(() => {
    if (!player) return;
    try { player.pause(); } catch (err) { console.warn("released", err); }
    try { player.volume = 0; } catch (err) { console.warn("released", err); }
    void setAudioBoost(1); // drop the amplification while muted for casting
  }, [player]);

  const unmuteLocalPlayer = useCallback(() => {
    if (!player) return;
    // volume is the 0-2 level: restore the native part and re-arm the boost.
    try {
      player.volume = Math.min(1, volume);
      void setAudioBoost(volume > 1 ? volume : 1);
    } catch (err) { console.warn("released", err); }
  }, [player, volume]);

  const castLoadMedia = useCallback(async (client: any) => {
    if (!streamUrl || !title) return;
    muteLocalPlayer();
    const castUrl = streamUrl;
    console.log("[Cast] loadMedia URL:", castUrl);
    const isHls = castUrl.includes("m3u8");
    try {
      await client.loadMedia({
        autoplay: true,
        mediaInfo: {
          contentId: mediaId,
          contentUrl: castUrl,
          contentType: isHls ? "application/x-mpegurl" : "video/mp4",
          streamType: "buffered",
          metadata: { type: "movie", title },
        },
        startTime: currentTime,
      });
    } catch (e) { console.warn("[CAST] loadMedia failed:", e); }
  }, [streamUrl, title, mediaId, currentTime, muteLocalPlayer]);

  const handleCast = useCallback(async () => {
    if (!CastContext) return;
    if (castConnected && castClient) {
      await castLoadMedia(castClient);
    } else {
      // Deliberately NOT muting here. Silencing the local player before the
      // picker opens leaves it paused at volume 0 when the user dismisses the
      // picker: dismissing changes no cast state, so no effect re-runs and
      // nothing undoes it. castLoadMedia mutes at the point the receiver
      // actually takes over, which is the only moment it is safe to.
      CastContext.showCastDialog?.();
    }
  }, [castConnected, castClient, castLoadMedia]);

  // Auto-load when cast client becomes available after dialog
  const castLoadedForMedia = useRef<string | null>(null);
  useEffect(() => {
    if (!castClient || !castConnected || !streamUrl) return;
    if (castLoadedForMedia.current === streamUrl) return;
    castLoadedForMedia.current = streamUrl;
    castLoadMedia(castClient);
  }, [castClient, castConnected, streamUrl, castLoadMedia]);

  // Unmute local player when cast disconnects
  useEffect(() => {
    if (!castConnected) {
      castLoadedForMedia.current = null;
      unmuteLocalPlayer();
    }
  }, [castConnected, unmuteLocalPlayer]);

  return { castConnected, castClient, castCurrentTime, castDuration, castIsPlaying, handleCast };
}
