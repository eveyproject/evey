import { useState, useEffect, useRef, useCallback } from "react";
import { Platform } from "react-native";
import * as Network from "expo-network";
import {
  playbackServiceGetStreamInfo,
  playbackServiceStartStream,
  playbackServiceStopStream,
  eveyFetch,
} from "@evey/api";
import type { V1StreamInfo, V1WatchProgress } from "@evey/api";
import { getServerUrl } from "../lib/server-config";
import { getDeviceAudioCodecs, getDeviceHdrSupport } from "../lib/deviceCodecs";

const DEVICE_ID = "evey-mobile";
const DEVICE_NAME = "Evey Mobile";

const VIDEO_CODECS = ["h264", "hevc", "vp9"];
// Android's player (ExoPlayer/AndroidXMedia3) demuxes Matroska/WebM natively, so
// a compatible-codec .mkv (the bulk of a self-hosted library) can DIRECT-PLAY
// instead of being needlessly transcoded - which is what breaks in-timeline
// seeking (transcode serves a sliding window) and shifts colors (ffmpeg). iOS's
// AVPlayer can't demux MKV, so it stays on mp4/hls and lets the server remux it.
const CONTAINERS = Platform.OS === "android" ? ["mp4", "mkv", "webm", "hls"] : ["mp4", "hls"];

// Probe the connection state via expo-network so the server can apply the
// per-profile max_bitrate_cellular_kbps cap when relevant. Anything that
// isn't explicitly cellular is reported as "wifi" - wired/ethernet/unknown
// don't deserve the cap, and the API only has those two values.
async function detectNetworkType(): Promise<"wifi" | "cellular"> {
  try {
    const state = await Network.getNetworkStateAsync();
    return state.type === Network.NetworkStateType.CELLULAR ? "cellular" : "wifi";
  } catch {
    return "wifi";
  }
}

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
  // The opaque stream token, exposed so the native player can also send it as an
  // Authorization: Bearer header on the source (segment auth without the token
  // riding in every URL) - the server accepts cookie/Bearer/query alike.
  streamToken: string | null;
  sessionId: string | null;
  streamInfo: V1StreamInfo | null;
  isLoading: boolean;
  isError: boolean;
  // Set instead of isError when the server refused to start THIS request for
  // one of the two shed reasons above - distinct from a genuine failure.
  shedReason: ShedReason;
  stopSession: () => void;
  restartWithTracks: (audioIndex?: number, subtitleIndex?: number, maxHeight?: number) => void;
  progress: V1WatchProgress | null;
}

// mediaId reaches these builders straight from the route params
// (useLocalSearchParams), which a deep link controls. Every id is
// percent-encoded so a crafted "..%2Fadmin" id cannot pivot the authenticated
// request onto another API path.
export async function fetchStreamToken(mediaId: string): Promise<string> {
  // Native player (expo-video/ExoPlayer/AVPlayer): like a Chromecast or a web
  // native <video>, it fetches segments itself and can't re-mint the short
  // sliding token mid-stream, so a pause would 401 on the 5-minute default.
  // purpose=native returns the long-lived (12h) window instead.
  const res = await eveyFetch<StreamTokenResponse>({
    url: `/api/v1/media/${encodeURIComponent(mediaId)}/stream-token?purpose=native`,
    method: "POST",
  });
  return res.token;
}

function appendCodecParams(params: URLSearchParams): void {
  params.set("videoCodecs", VIDEO_CODECS.join(","));
  // Audio codecs come from the device's real decoder list (MediaCodecList via
  // the MediaCaps native module) - a hardcoded list with ac3/eac3 direct-played
  // Dolby tracks on devices with no Dolby decoder: video without sound.
  params.set("audioCodecs", getDeviceAudioCodecs().join(","));
  params.set("containers", CONTAINERS.join(","));
  // hdr=1 lets the decision keep the video COPY rung for HDR10/HLG sources on
  // an HDR panel (exact colors + on-demand seek) instead of tonemapping to SDR.
  if (getDeviceHdrSupport()) params.set("hdr", "1");
}

export function buildStreamUrl(
  serverUrl: string,
  mediaId: string,
  streamInfo: V1StreamInfo,
  token: string,
): string {
  if (streamInfo.playMethod === "PLAY_METHOD_DIRECT_PLAY") {
    return `${serverUrl}/api/v1/media/${encodeURIComponent(mediaId)}/stream?token=${token}`;
  }
  if (streamInfo.playMethod === "PLAY_METHOD_DIRECT_STREAM") {
    const p = new URLSearchParams({ token, remux: "1" });
    appendCodecParams(p);
    return `${serverUrl}/api/v1/media/${encodeURIComponent(mediaId)}/transcode?${p.toString()}`;
  }
  const params = new URLSearchParams({ token });
  appendCodecParams(params);
  // Server publishes a complete VOD manifest from t=0 (no live-edge snap), so
  // there is no start offset to declare: the player seeks natively on the
  // absolute timeline. A startSec param used to be sent here, but no handler
  // ever parsed it - it only tricked the client into offsetting its clock.
  return `${serverUrl}/api/v1/media/${encodeURIComponent(mediaId)}/hls/master.m3u8?${params.toString()}`;
}

export function usePlaySession(mediaId: string): PlaySessionResult {
  const [streamUrl, setStreamUrl] = useState<string | null>(null);
  const [streamToken, setStreamToken] = useState<string | null>(null);
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [streamInfo, setStreamInfo] = useState<V1StreamInfo | null>(null);
  const [progress, setProgress] = useState<V1WatchProgress | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isError, setIsError] = useState(false);
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
        // Screen size is NOT a decode capability: the display downscales high-res
        // content for free. Sending the screen dims made the server flag
        // resolution_exceeded and transcode/downscale a 1080p/4K title the device
        // plays natively - killing direct-play (and with it native seek + correct
        // colors). Cap resolution ONLY when the user explicitly picks a lower
        // quality via restartWithTracks(maxHeight). (Mirrors the web player.)
        const explicitMaxHeight = maxHeight && maxHeight > 0 ? maxHeight : undefined;
        const networkType = await detectNetworkType();

        const [info, wp] = await Promise.all([
          playbackServiceGetStreamInfo(mediaId, {
            supportedCodecs: VIDEO_CODECS,
            supportedAudioCodecs: getDeviceAudioCodecs(),
            supportedContainers: CONTAINERS,
            supportsHdr: getDeviceHdrSupport(),
            maxHeight: explicitMaxHeight,
            audioStreamIndex,
            subtitleStreamIndex,
            networkType,
          }),
          eveyFetch<V1WatchProgress>({
            url: `/api/v1/playback/${encodeURIComponent(mediaId)}/progress`,
            method: "GET",
          }).catch(() => null),
        ]);

        if (wp) setProgress(wp);

        let token = tokenRef.current;
        if (!token) {
          token = await fetchStreamToken(mediaId);
          tokenRef.current = token;
        }
        setStreamToken(token);
        const url = buildStreamUrl(serverUrl, mediaId, info, token);

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
        console.warn("[player] startStream failed:", err);
        setIsError(true);
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
        console.warn("stop session failed:", err),
      );
    }
  }, []);

  const restartWithTracks = useCallback(
    (audioIndex?: number, subtitleIndex?: number, maxHeight?: number) => {
      const sid = sessionIdRef.current;
      if (sid) {
        playbackServiceStopStream({ sessionId: sid }).catch((err) =>
          console.warn("stop before restart failed:", err),
        );
        sessionIdRef.current = null;
      }
      stoppedRef.current = false;
      setIsError(false);
      setShedReason(null);
      setIsLoading(true);
      startStream(audioIndex, subtitleIndex, maxHeight);
    },
    [startStream],
  );

  // restartAtPosition used to live here: a transcode seek tore the session down
  // and restarted it at the target. It is gone because the VOD manifest covers
  // the whole timeline - the player seeks natively (see seekTo/seekBy), which is
  // both instant and free of the offset bookkeeping that reported doubled positions.

  useEffect(() => {
    return () => stopSession();
  }, [stopSession]);

  return { streamUrl, streamToken, sessionId, streamInfo, isLoading, isError, shedReason, stopSession, restartWithTracks, progress };
}
