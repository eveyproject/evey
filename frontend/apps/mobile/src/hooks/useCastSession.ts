import { useCallback, useEffect, useRef } from "react";
import type { VideoPlayer } from "expo-video";
import type { V1StreamInfo } from "@evey/api";
import { setAudioBoost } from "../lib/audioBoost";
import { fetchStreamToken } from "./usePlaySession";
import { buildCastStreamUrl, pickCastProfile } from "../lib/castUrl";
import {
  CastContext, useCastStateHook, useRemoteMediaClientHook,
  useMediaStatusHook, useCastDeviceHook, CastStateEnum,
} from "./useChromecast";

// Swaps the URL's token param for a cast-purpose one, preserving every other
// query param (codecs, remux, ...). Only the CASTED url is touched - the
// local player keeps its native-purpose token untouched.
function withCastToken(url: string, castToken: string): string {
  const u = new URL(url);
  u.searchParams.set("token", castToken);
  return u.toString();
}

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
  // streamInfo/serverUrl come straight from usePlaySession - buildCastStreamUrl
  // re-negotiates the URL against the RECEIVER's decode profile instead of
  // casting the phone-negotiated streamUrl as-is (see lib/castUrl.ts).
  streamInfo: V1StreamInfo | null;
  serverUrl: string | null;
}

export function useCastSession({
  player,
  volume,
  streamUrl,
  mediaId,
  title,
  currentTime,
  fallbackDurationSec,
  streamInfo,
  serverUrl,
}: UseCastSessionArgs) {
  const castState = useCastStateHook();
  const castClient = useRemoteMediaClientHook();
  const castMediaStatus = useMediaStatusHook();
  const castDevice = useCastDeviceHook();
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
    let castUrl = streamUrl;
    // The receiver's fetches only feed server-side progress tracking when
    // ridden on a purpose=cast token (see cast_observe.go) - the local
    // player's purpose=native token in streamUrl is invisible to it. Falling
    // back to the native token on mint failure still plays, just without
    // server-derived progress - better than a broken cast.
    try {
      const castToken = await fetchStreamToken(mediaId, { cast: true });
      if (streamInfo && serverUrl) {
        // The RECEIVER decodes the stream, not this phone: negotiating with
        // streamInfo/the receiver's profile (instead of casting the
        // phone-negotiated streamUrl) is what stops a copy rung the
        // Chromecast can't decode from producing a black screen on the TV.
        const profile = pickCastProfile(castDevice?.modelName);
        castUrl = buildCastStreamUrl(serverUrl, mediaId, streamInfo, castToken, profile);
      } else {
        castUrl = withCastToken(streamUrl, castToken);
      }
    } catch (err) {
      console.warn("[Cast] cast token mint failed, casting with native token:", err);
    }
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
  }, [streamUrl, title, mediaId, currentTime, muteLocalPlayer, streamInfo, serverUrl, castDevice?.modelName]);

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
