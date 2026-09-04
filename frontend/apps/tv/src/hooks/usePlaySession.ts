import { useState, useEffect, useRef, useCallback } from "react";
import { Dimensions } from "react-native";
import {
  playbackServiceGetStreamInfo,
  playbackServiceStartStream,
  playbackServiceStopStream,
  eveyFetch,
} from "@evey/api";
import type { V1StreamInfo, V1WatchProgress } from "@evey/api";
import { getServerUrl } from "../lib/server-config";
import { getDeviceAudioCodecs } from "../lib/deviceCodecs";
import { probeMaxOutputChannels } from "../lib/outputChannels";
import { logger } from "../lib/log";

const log = logger("play");

const DEVICE_ID = "evey-tv";
const DEVICE_NAME = "Evey TV";

const VIDEO_CODECS = ["h264", "hevc", "vp9"];
const CONTAINERS = ["mp4", "hls"];

interface StreamTokenResponse {
  token: string;
  mediaId: string;
}

// Mirrors the server's shed contract (HTTP 503 on a refused playback start):
// - too_many_streams: the box can't prepare one more stream because of OTHER
//   concurrent playbacks.
// - file_not_ready: unrelated to load - this specific file's seek index is
//   still being built and will serve fine once that finishes.
// Never surface the server's `message` field (English, technical) to users -
// the screen picks its own non-technical copy from this code.
type ShedReason = "too_many_streams" | "file_not_ready" | null;

// expo-video's PlayerError is just `{ message: string }` - ExoPlayer/AVPlayer
// give no structured HTTP status or response body, so there is no reliable
// way to read the shed code off a native player error. Instead, probe the
// manifest/remux URL directly before handing it to the player: the server
// returns the code both as an `X-Evey-Error` header and a JSON body (see
// transcode_handler_vod.go's writeShed). A second identical GET for the same
// token right after (the native player fetching the same URL) is expected
// and safe - repeated HLS manifest requests are idempotent, the same as a
// manifest reload on seek, so this doesn't consume a second capacity slot.
async function detectShed(url: string): Promise<ShedReason> {
  try {
    const res = await fetch(url);
    if (res.status !== 503) return null;
    const headerCode = res.headers.get("X-Evey-Error");
    if (headerCode === "too_many_streams" || headerCode === "file_not_ready") return headerCode;
    try {
      const body = (await res.json()) as { code?: string };
      if (body.code === "too_many_streams" || body.code === "file_not_ready") return body.code;
    } catch {
      // Not parseable JSON - fall through to the default below.
    }
    // No usable code: default to the "too many streams" framing. It's the
    // more common shed cause, and its "try again shortly" copy reads fine
    // either way.
    return "too_many_streams";
  } catch {
    // The preflight itself failed (offline, DNS, ...) - let the native player
    // attempt playback and surface its own generic error if it also fails.
    return null;
  }
}

export interface PlaySessionResult {
  streamUrl: string | null;
  sessionId: string | null;
  streamInfo: V1StreamInfo | null;
  isLoading: boolean;
  isError: boolean;
  // Set instead of isError when the session is no longer valid. Kept apart
  // because the answer is completely different: nothing is wrong with the
  // stream, the viewer simply has to sign in again. Collapsing it into
  // isError is what put "Failed to load stream" on screen for an expired
  // token, and sent the owner hunting a codec bug that did not exist.
  isAuthError: boolean;
  // Set instead of isError when the server refused to start THIS request for
  // one of the two shed reasons above - distinct from a genuine failure.
  shedReason: ShedReason;
  stopSession: () => void;
  restartWithTracks: (audioIndex?: number, subtitleIndex?: number, maxHeight?: number) => void;
  progress: V1WatchProgress | null;
}

async function fetchStreamToken(mediaId: string): Promise<string> {
  // Native player (expo-video/ExoPlayer): like a Chromecast, it fetches
  // segments itself and can't re-mint the short sliding token mid-stream, so
  // a living-room pause over 5 minutes would 401 on the next segment.
  // purpose=native returns the long-lived (12h) window instead - the exact
  // fix mobile received; the TV had been left on the 5-minute default.
  const res = await eveyFetch<StreamTokenResponse>({
    url: `/api/v1/media/${encodeURIComponent(mediaId)}/stream-token?purpose=native`,
    method: "POST",
  });
  return res.token;
}

function appendCodecParams(params: URLSearchParams, maxAudioChannels?: number): void {
  params.set("videoCodecs", VIDEO_CODECS.join(","));
  params.set("audioCodecs", getDeviceAudioCodecs().join(","));
  params.set("containers", CONTAINERS.join(","));
  // Channel ceiling of the OUTPUT, not of the decoder - see lib/outputChannels.
  // Omitted when the box cannot answer, which leaves the server profile's own
  // default in place: an unknown output must not silently lose surround.
  if (maxAudioChannels != null) params.set("maxAudioChannels", String(maxAudioChannels));
}

// The 0-based position of an absolute media-stream index among streams of one
// type - the form the HLS query params are counted in. Mirrors the mobile twin
// (apps/mobile/src/hooks/usePlaySession.ts).
function typeRelativeIndex(
  streamInfo: V1StreamInfo | undefined,
  type: "MEDIA_STREAM_TYPE_AUDIO" | "MEDIA_STREAM_TYPE_SUBTITLE",
  absoluteIndex: number | undefined,
): number | undefined {
  if (absoluteIndex == null || absoluteIndex <= 0) return undefined;
  const ofType = (streamInfo?.mediaStreams ?? []).filter((s) => s.type === type);
  const pos = ofType.findIndex((s) => (s.index ?? -1) === absoluteIndex);
  return pos >= 0 ? pos : undefined;
}

// audioTrack / subtitleTrack are TYPE-RELATIVE, not the absolute media-stream
// index. They MUST land in the HLS master URL, and until 2026-09-04 the TV app
// left them out entirely while telling the gRPC calls about them - so the
// server negotiated one thing and served another:
//
//   - the manifest's audio is picked by ?audioTrack= (transcode_handler_vod.go),
//     so every TV stream played the default track whatever the picker said;
//   - a bitmap subtitle is burnt in only when ?subtitleTrack= names it
//     (transcode_params.go), so selecting one drew nothing;
//   - and without either param the URL is byte-identical for every track, so
//     switching language rebuilt the same URL and the native player never
//     reloaded - the pick could not even take effect on the next segment.
export function buildStreamUrl(
  serverUrl: string,
  mediaId: string,
  streamInfo: V1StreamInfo,
  token: string,
  maxAudioChannels?: number,
  audioTrack?: number,
  subtitleTrack?: number | null,
): string {
  if (streamInfo.playMethod === "PLAY_METHOD_DIRECT_PLAY") {
    return `${serverUrl}/api/v1/media/${mediaId}/stream?token=${token}`;
  }
  // DIRECT_STREAM (container-only mismatch, both streams copy-eligible) falls
  // through to the HLS master on purpose - see the mobile twin
  // (apps/mobile/src/hooks/usePlaySession.ts) for the full reasoning:
  // /transcode?remux=1 is a piped ffmpeg stdout with `Accept-Ranges: none`, so
  // the native player marks it unseekable and resume / timeline / skip all die.
  const params = new URLSearchParams({ token });
  appendCodecParams(params, maxAudioChannels);
  if (audioTrack != null && audioTrack >= 0) params.set("audioTrack", String(audioTrack));
  if (subtitleTrack != null && subtitleTrack >= 0) params.set("subtitleTrack", String(subtitleTrack));
  // Server publishes a complete VOD manifest from t=0 (no live-edge snap), so
  // there is no start offset to declare: the player seeks natively on the
  // absolute timeline. A startSec param used to be sent here, but no handler
  // ever parsed it - it only tricked the client into offsetting its clock.
  return `${serverUrl}/api/v1/media/${mediaId}/hls/master.m3u8?${params.toString()}`;
}

export function usePlaySession(mediaId: string): PlaySessionResult {
  const [streamUrl, setStreamUrl] = useState<string | null>(null);
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [streamInfo, setStreamInfo] = useState<V1StreamInfo | null>(null);
  const [progress, setProgress] = useState<V1WatchProgress | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isError, setIsError] = useState(false);
  const [isAuthError, setIsAuthError] = useState(false);
  const [shedReason, setShedReason] = useState<ShedReason>(null);
  const sessionIdRef = useRef<string | null>(null);
  const startedRef = useRef(false);
  const prevMediaIdRef = useRef(mediaId);
  if (prevMediaIdRef.current !== mediaId) {
    prevMediaIdRef.current = mediaId;
    startedRef.current = false;
  }
  const stoppedRef = useRef(false);
  const tokenRef = useRef<string | null>(null);
  const serverUrlRef = useRef<string>("");
  const streamInfoRef = useRef<V1StreamInfo | null>(null);

  const startStream = useCallback(
    async (audioStreamIndex?: number, subtitleStreamIndex?: number, maxHeight?: number) => {
      try {
        const serverUrl = serverUrlRef.current || ((await getServerUrl()) ?? "");
        serverUrlRef.current = serverUrl;
        const screen = Dimensions.get("screen");
        const screenH = Math.round(screen.height * (screen.scale ?? 1));

        const [info, wp, maxAudioChannels] = await Promise.all([
          playbackServiceGetStreamInfo(mediaId, {
            supportedCodecs: VIDEO_CODECS,
            supportedAudioCodecs: getDeviceAudioCodecs(),
            supportedContainers: CONTAINERS,
            maxWidth: Math.round(screen.width * (screen.scale ?? 1)),
            maxHeight: maxHeight && maxHeight > 0 ? Math.min(maxHeight, screenH) : screenH,
            audioStreamIndex,
            subtitleStreamIndex,
          }),
          eveyFetch<V1WatchProgress>({
            url: `/api/v1/playback/${mediaId}/progress`,
            method: "GET",
          }).catch(() => null),
          // Re-probed at every negotiation, not memoised at startup: the output
          // device changes mid-session (headphones, dock, amplifier).
          probeMaxOutputChannels(),
        ]);

        if (wp) setProgress(wp);

        let token = tokenRef.current;
        if (!token) {
          token = await fetchStreamToken(mediaId);
          tokenRef.current = token;
        }
        const audioTrack = typeRelativeIndex(info, "MEDIA_STREAM_TYPE_AUDIO", audioStreamIndex);
        const subtitleTrack = typeRelativeIndex(info, "MEDIA_STREAM_TYPE_SUBTITLE", subtitleStreamIndex);
        const url = buildStreamUrl(serverUrl, mediaId, info, token, maxAudioChannels, audioTrack, subtitleTrack);

        // Direct play streams the original file with no orchestrator involved,
        // so it cannot be shed - only probe the transcode/HLS paths.
        if (info.playMethod !== "PLAY_METHOD_DIRECT_PLAY") {
          const shed = await detectShed(url);
          if (shed) {
            if (!stoppedRef.current) setShedReason(shed);
            return;
          }
        }

        const startPosMs = wp?.positionMs && !wp.played ? wp.positionMs : "0";
        const session = await playbackServiceStartStream({
          mediaItemId: mediaId,
          playMethod: info.playMethod ?? "PLAY_METHOD_TRANSCODE",
          deviceId: DEVICE_ID,
          deviceName: DEVICE_NAME,
          clientName: DEVICE_NAME,
          startPositionMs: startPosMs,
          audioStreamIndex,
          subtitleStreamIndex,
        });

        if (!stoppedRef.current) {
          streamInfoRef.current = info;
          setStreamInfo(info);
          setStreamUrl(url);
          if (session.id) {
            sessionIdRef.current = session.id;
            setSessionId(session.id);
          }
        }
      } catch (err) {
        // The shared fetcher tags an unrecoverable auth failure with
        // httpStatus 401 (it has already tried the refresh by then).
        if ((err as { httpStatus?: number })?.httpStatus === 401) {
          setIsAuthError(true);
        } else {
          setIsError(true);
        }
      } finally {
        setIsLoading(false);
      }
    },
    [mediaId],
  );

  useEffect(() => {
    if (!mediaId || startedRef.current) return;
    startedRef.current = true;
    stoppedRef.current = false;
    startStream();
  }, [mediaId, startStream]);

  const stopSession = useCallback(() => {
    if (stoppedRef.current) return;
    stoppedRef.current = true;
    const sid = sessionIdRef.current;
    if (sid) {
      playbackServiceStopStream({ sessionId: sid }).catch((err) =>
        log.warn("stop session failed:", err),
      );
    }
  }, []);

  const restartWithTracks = useCallback(
    (audioIndex?: number, subtitleIndex?: number, maxHeight?: number) => {
      const sid = sessionIdRef.current;
      if (sid) {
        playbackServiceStopStream({ sessionId: sid }).catch((err) =>
          log.warn("stop before restart failed:", err),
        );
        sessionIdRef.current = null;
      }
      stoppedRef.current = false;
      // Forget the cached stream token: a retry that reuses the token which
      // just 401'd rebuilds the same dead URL and fails identically - the
      // only exit was leaving the player. Minting fresh is the point of retry.
      tokenRef.current = null;
      setIsError(false);
      setShedReason(null);
      setIsLoading(true);
      startStream(audioIndex, subtitleIndex, maxHeight);
    },
    [startStream],
  );

  // restartAtPosition used to live here: a transcode seek tore the session down
  // and restarted it at the target. It is gone because the VOD manifest covers
  // the whole timeline - the player seeks natively (see seekBy), which is both
  // instant and free of the offset bookkeeping that reported doubled positions.

  useEffect(() => {
    return () => stopSession();
  }, [stopSession]);

  return { streamUrl, sessionId, streamInfo, isLoading, isError, isAuthError, shedReason, stopSession, restartWithTracks, progress };
}
