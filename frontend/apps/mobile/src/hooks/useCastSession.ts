import { useCallback, useEffect, useRef } from "react";
import type { VideoPlayer } from "expo-video";
import type { V1StreamInfo } from "@evey/api";
import { eveyFetch, playbackServiceGetStreamInfo } from "@evey/api";
import { setAudioBoost } from "../lib/audioBoost";
import { fetchStreamToken } from "./usePlaySession";
import { askReceiverCaps, buildCastStreamUrl, castProfileFor, castStreamInfoParams, type CapsAnswer } from "@evey/media-caps";
import { buildCastCustomData } from "@evey/media-caps/launch";
import { ensureNotificationPermission } from "../lib/notificationPermission";
import { describeCastTracks, shouldReportTracks } from "../lib/castTrackBeacon";
import { resumePositionAfterCast } from "../lib/castPlaybackState";
import { observeCastStall, initialCastStallTracker, PLAYER_STATE_PLAYING } from "../lib/castStallBeacon";
import {
  CastContext, useCastStateHook, useRemoteMediaClientHook,
  useMediaStatusHook, useCastDeviceHook, CastStateEnum,
  getConnectedDeviceModelName, openCapsChannel, type CapsChannel,
} from "./useChromecast";

// Swaps the URL's token param for a cast-purpose one, preserving every other
// query param (codecs, remux, ...). Only the CASTED url is touched - the
// local player keeps its native-purpose token untouched.
function withCastToken(url: string, castToken: string): string {
  const u = new URL(url);
  u.searchParams.set("token", castToken);
  return u.toString();
}

// askConnectedReceiverCaps runs the caps handshake on a channel opened on the
// session connected right now and closed right after - a late answer from a
// session the caller has moved on from lands on a destroyed channel. Never
// throws: no channel, no answer, is the model/floor path, not a broken cast.
async function askConnectedReceiverCaps(): Promise<CapsAnswer | null> {
  let channel: CapsChannel | null = null;
  try {
    channel = await openCapsChannel();
    if (!channel) return null;
    return await askReceiverCaps(channel.transport);
  } catch {
    return null;
  } finally {
    await channel?.close();
  }
}

interface UseCastSessionArgs {
  player: VideoPlayer;
  /** Puts the local player back on the bar's level (trim, device, boost) once a cast ends. */
  restoreLocalVolume: () => void;
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
  // casting the phone-negotiated streamUrl as-is (see @evey/media-caps castUrl).
  streamInfo: V1StreamInfo | null;
  serverUrl: string | null;
}

export function useCastSession({
  player,
  restoreLocalVolume,
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
  const castIsPlaying = castMediaStatus?.playerState === PLAYER_STATE_PLAYING;
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

  // Tell the server what the SENDER sees when the receiver stops answering -
  // a PLAYING position frozen for CAST_STALL_SEC, or an IDLE state carrying a
  // reason - one beacon per situation. The 2026-08-31 receiver death left the
  // phone spinning with nothing in the logs about the sender's side; this is
  // the evidence a recovery gesture needs before it can be designed. Ticks
  // every 5 s because a frozen status delivers no re-render on its own.
  const stallRef = useRef(initialCastStallTracker);
  const castPlayerState = castMediaStatus?.playerState;
  const castIdleReason = (castMediaStatus as { idleReason?: string | number | null } | null)?.idleReason;
  useEffect(() => {
    if (!castConnected) {
      stallRef.current = initialCastStallTracker;
      return;
    }
    const observe = () => {
      const { next, report } = observeCastStall(stallRef.current, {
        connected: true,
        playerState: castPlayerState,
        idleReason: castIdleReason,
        posSec: castCurrentTime,
        nowMs: Date.now(),
      });
      stallRef.current = next;
      if (!report) return;
      eveyFetch({
        url: `/api/v1/media/${encodeURIComponent(mediaId)}/player-event`,
        method: "POST",
        data: {
          type: "cast_sender_stall",
          details: report,
          fatal: false, code: 0, variant: 0, segment: 0, posSec: Math.round(castCurrentTime),
        },
      }).catch(() => { /* telemetry must never break a cast */ });
    };
    observe();
    const timer = setInterval(observe, 5000);
    return () => clearInterval(timer);
  }, [castConnected, castPlayerState, castIdleReason, castCurrentTime, mediaId]);

  const muteLocalPlayer = useCallback(() => {
    if (!player) return;
    try { player.pause(); } catch (err) { console.warn("released", err); }
    try { player.volume = 0; } catch (err) { console.warn("released", err); }
    void setAudioBoost(1); // drop the amplification while muted for casting
  }, [player]);

  // Read through a ref: depending on the level re-ran the disconnect effect on
  // every volume change, which wrote the raw level over the player's trim.
  const restoreRef = useRef(restoreLocalVolume);
  restoreRef.current = restoreLocalVolume;
  const unmuteLocalPlayer = useCallback(() => {
    if (!player) return;
    try { restoreRef.current(); } catch (err) { console.warn("released", err); }
  }, [player]);

  // Which MEDIA the receiver was last loaded with, and where the receiver got
  // to while casting (by the time the session is gone, so is its media status,
  // so it cannot be read at the moment it is needed). Both live above
  // castLoadMedia because a re-load reads them.
  const castLoadedForMedia = useRef<string | null>(null);
  const lastCastPositionRef = useRef<number | null>(null);
  // Generation of the load in flight: bumped by every load and by a
  // disconnect, so the awaits below (token, receiver caps, verdict) can tell
  // that the session they started on is gone and drop their result instead
  // of loading a stale negotiation onto whatever is connected now.
  const castLoadGenRef = useRef(0);

  const castLoadMedia = useCallback(async (client: any) => {
    if (!streamUrl || !title) return;
    const gen = ++castLoadGenRef.current;
    const stale = () => castLoadGenRef.current !== gen;
    // Re-loading the SAME title (re-cast from the button, a re-negotiated URL)
    // resumes where the television is, not where the muted local player was
    // parked - which is the position the cast started from, i.e. zero for a
    // film cast from the start. Live 2026-08-31: a mid-cast re-login re-loaded
    // the receiver at 0 after 54 minutes of film.
    const resumeFromReceiver = castLoadedForMedia.current === mediaId ? lastCastPositionRef.current : null;
    const startTime = resumeFromReceiver ?? currentTime;
    castLoadedForMedia.current = mediaId;
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
      if (stale()) return;
      if (streamInfo && serverUrl) {
        // The RECEIVER decodes the stream, not this phone: the SERVER judges
        // the receiver's direct play with the receiver's profile, the same
        // rules every client gets - this phone keeps no rule set of its own.
        // The profile is what the receiver OBSERVED about itself when it
        // answers the caps channel (the display behind it included: a 4K box
        // on a 1080p screen answers 1080p, which no model name can know),
        // the model table otherwise, the floor when nothing is known. Asked
        // BEFORE the verdict: a table entry richer than the real screen would
        // let the server hand the receiver a raw HEVC/4K file it cannot show.
        const answer = await askConnectedReceiverCaps();
        if (stale()) return;
        eveyFetch({
          url: `/api/v1/media/${encodeURIComponent(mediaId)}/player-event`,
          method: "POST",
          data: {
            type: "cast_receiver_caps",
            details: answer
              ? `answered=1 codecs=${answer.caps.videoCodecs.join("/")} maxHeight=${answer.caps.maxHeight} h264=${answer.caps.h264MaxHeight ?? "level" + answer.caps.maxH264Level} hdr=${answer.caps.hdr ? 1 : 0} ch=${answer.caps.maxAudioChannels} ua=${answer.userAgent ?? "-"}`
              : "answered=0",
            fatal: false, code: 0, variant: 0, segment: 0, posSec: 0,
          },
        }).catch(() => { /* telemetry must never break a cast */ });
        const profile = castProfileFor(answer?.caps ?? null, modelName);
        const receiverInfo = await playbackServiceGetStreamInfo(mediaId, castStreamInfoParams(profile)).catch(() => null);
        if (stale()) return;
        castUrl = buildCastStreamUrl(serverUrl, mediaId, receiverInfo, castToken, profile);
      } else {
        castUrl = withCastToken(streamUrl, castToken);
      }
    } catch (err) {
      console.warn("[Cast] cast token mint failed, casting with native token:", err);
    }
    if (stale()) return;
    // Never the full URL in a release build: a cast token is minted for 12
    // hours and rides in the query string, so this line put a ready-to-use
    // stream link into Logcat on every cast - readable by adb, by an OEM
    // "send logs to support" flow, or by anything on a rooted device. The
    // media id is what debugging actually needs.
    if (process.env.NODE_ENV !== "production") {
      console.log("[Cast] loadMedia URL:", castUrl);
    } else {
      console.log("[Cast] loadMedia mediaId:", mediaId);
    }
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
          // Cast Connect: what an Evey TV app needs to play this with its OWN
          // player - a media id and a position, never a URL. The Web Receiver
          // ignores it and reads contentId above, so one load feeds both.
          customData: buildCastCustomData({
            mediaId,
            positionSec: startTime,
            serverOrigin: serverUrl ?? "",
          }),
        },
        startTime,
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

  // Auto-load when the cast client becomes available after the dialog - keyed
  // on the MEDIA, never on the stream URL. The URL is re-minted whenever the
  // play session is re-issued (a token refresh refused mid-cast logs the app
  // out, the saved credentials log it back in, StartStream runs again): keyed
  // on the URL, that re-issue re-LOADed the receiver, which interrupted the
  // running film and restarted it from zero (live, 2026-08-31, 54 min in).
  // The receiver holds its own 12-hour cast token; a new sender URL changes
  // nothing for it.
  useEffect(() => {
    if (!castClient || !castConnected || !streamUrl) return;
    if (castLoadedForMedia.current === mediaId) return;
    castLoadMedia(castClient);
  }, [castClient, castConnected, streamUrl, mediaId, castLoadMedia]);

  // Track the receiver's last known position WHILE casting.
  useEffect(() => {
    if (castConnected && castCurrentTime > 0) {
      lastCastPositionRef.current = castCurrentTime;
    }
  }, [castConnected, castCurrentTime]);

  // Unmute the local player when the cast disconnects, and put it where the
  // television actually got to - it was parked at the position the cast started
  // from, so without this, stopping a cast rewound the viewer by the whole
  // length of what they had just watched.
  const wasConnectedRef = useRef(false);
  useEffect(() => {
    const wasConnected = wasConnectedRef.current;
    wasConnectedRef.current = castConnected;
    if (castConnected || !wasConnected) return;
    castLoadedForMedia.current = null;
    // A load negotiating for the session that just ended must not land on
    // the next one: its caps, its verdict, its URL were that session's.
    castLoadGenRef.current += 1;
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
