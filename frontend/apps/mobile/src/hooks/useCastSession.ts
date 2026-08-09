import { useCallback, useEffect, useRef } from "react";
import type { VideoPlayer } from "expo-video";
import type { V1StreamInfo } from "@evey/api";
import { eveyFetch } from "@evey/api";
import { setAudioBoost } from "../lib/audioBoost";
import { fetchStreamToken } from "./usePlaySession";
import { buildCastStreamUrl, pickCastProfile } from "../lib/castUrl";
import { ensureNotificationPermission } from "../lib/notificationPermission";
import { describeCastTracks, shouldReportTracks } from "../lib/castTrackBeacon";
import { resumePositionAfterCast } from "../lib/castPlaybackState";
import {
  CastContext, useCastStateHook, useRemoteMediaClientHook,
  useMediaStatusHook, useCastDeviceHook, CastStateEnum,
  getConnectedDeviceModelName,
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
  // The receiver's own stream volume/mute. Needed because muteLocalPlayer parks
  // the local player at volume 0: without these the session was reported as
  // muted at 0% while the television played out loud (see castPlaybackState.ts).
  const castVolume = castMediaStatus?.volume;
  const castIsMuted = castMediaStatus?.isMuted;
  // Forwarded to the progress reporter: a report naming a cast device is what
  // makes the server leave admin remote commands for the TV's beacon instead
  // of this phone (see useProgressReporter). Generic fallback while the
  // useCastDevice hook has not delivered yet - the name is display-only.
  const castDeviceName = castConnected ? (castDevice?.friendlyName ?? "Chromecast") : "";

  // Report the receiver's own track list once per shape (see castTrackBeacon).
  // A track selector during a cast cannot be designed without knowing what the
  // receiver exposes for an HLS stream, and that is not knowable from here.
  const lastTracksRef = useRef<string | null>(null);
  useEffect(() => {
    if (!castConnected) {
      lastTracksRef.current = null;
      return;
    }
    const line = describeCastTracks(castMediaStatus?.mediaInfo?.mediaTracks, castMediaStatus?.activeTrackIds);
    if (!shouldReportTracks(lastTracksRef.current, line)) return;
    lastTracksRef.current = line;
    eveyFetch({
      url: `/api/v1/media/${encodeURIComponent(mediaId)}/player-event`,
      method: "POST",
      data: {
        type: "cast_media_tracks",
        details: line,
        fatal: false, code: 0, variant: 0, segment: 0, posSec: 0,
      },
    }).catch(() => { /* telemetry must never break a cast */ });
  }, [castConnected, castMediaStatus?.mediaInfo?.mediaTracks, castMediaStatus?.activeTrackIds, mediaId]);

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
    // The device comes from the LIVE session, not the useCastDevice hook: the
    // hook only delivers on a later render, and the auto-load fires right on
    // connection - reading it here reported model=unknown for a real TV and
    // silently floored the capability profile (prod, 2026-07-25 00:51).
    const modelName = (await getConnectedDeviceModelName()) ?? castDevice?.modelName ?? null;
    // The profile table keys on modelName SUBSTRINGS whose exact 2024-device
    // values (Chromecast HD, Google TV Streamer) are published nowhere - this
    // beacon puts the raw string in the admin logs on the first real
    // connection, so the table gets corrected on evidence, not folklore.
    eveyFetch({
      url: `/api/v1/media/${encodeURIComponent(mediaId)}/player-event`,
      method: "POST",
      data: {
        type: "cast_receiver_model",
        details: `model=${modelName ?? "unknown"}`,
        fatal: false, code: 0, variant: 0, segment: 0, posSec: 0,
      },
    }).catch(() => { /* telemetry must never break a cast */ });
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
        const profile = pickCastProfile(modelName);
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
          // The Default Media Receiver PLAYS contentId - handing it the raw
          // media UUID left the TV spinning forever without a single request
          // to the server (first real-device test, a series episode, 2026-07-25).
          // The web sender that works on this TV sends the URL as contentId;
          // do exactly the same, contentUrl kept for CAF receivers.
          contentId: castUrl,
          contentUrl: castUrl,
          contentType: isHls ? "application/x-mpegurl" : "video/mp4",
          streamType: "buffered",
          metadata: { type: "movie", title },
          // Our HLS segments are fMP4: without these hints the receiver
          // assumes MPEG2-TS and loads forever with no picture - the exact
          // web fix validated on this TV on 2026-07-22 (b9bc22c98), ported.
          ...(isHls ? { hlsSegmentFormat: "FMP4", hlsVideoSegmentFormat: "FMP4" } : {}),
        },
        startTime: currentTime,
      });
    } catch (e) { console.warn("[CAST] loadMedia failed:", e); }
  }, [streamUrl, title, mediaId, currentTime, muteLocalPlayer, streamInfo, serverUrl, castDevice?.modelName]);

  const handleCast = useCallback(async () => {
    if (!CastContext) return;
    // Android 13+: the Cast media notification (the foreground service that
    // keeps this sender alive with the screen off) needs POST_NOTIFICATIONS.
    // Awaited so its dialog does not race the cast picker below.
    await ensureNotificationPermission();
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

  // Track the receiver's last known position WHILE casting: by the time the
  // session is gone, so is its media status, so it cannot be read at the moment
  // it is needed.
  const lastCastPositionRef = useRef<number | null>(null);
  useEffect(() => {
    if (castConnected && castCurrentTime > 0) {
      lastCastPositionRef.current = castCurrentTime;
    }
  }, [castConnected, castCurrentTime]);

  // Unmute the local player when the cast disconnects, and put it where the
  // television actually got to - it was parked at the position the cast started
  // from, so without this, stopping a cast rewound the viewer by the whole
  // length of what they had just watched.
  useEffect(() => {
    if (castConnected) return;
    castLoadedForMedia.current = null;
    unmuteLocalPlayer();
    const resumeAt = resumePositionAfterCast(lastCastPositionRef.current);
    lastCastPositionRef.current = null;
    if (resumeAt === null || !player) return;
    try {
      player.currentTime = resumeAt;
    } catch (err) {
      console.warn("released", err);
    }
  }, [castConnected, unmuteLocalPlayer, player]);

  return { castConnected, castClient, castCurrentTime, castDuration, castIsPlaying, castVolume, castIsMuted, castDeviceName, handleCast };
}
