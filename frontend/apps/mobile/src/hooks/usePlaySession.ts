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
import { getDeviceAudioCodecs, getDeviceHdrSupport, getDeviceMaxH264Level, getFmp4AudioPassthrough } from "../lib/deviceCodecs";
import { probeMaxOutputChannels } from "../lib/outputChannels";

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
// - calibrating: the server is benchmarking its own encode capacity (~2 min,
//   only ever on an idle host); playback works again once it is done.
// - file_needs_remux: the FILE is stored in one-track blocks and no player
//   gets past the first; an admin re-muxes it from the media-health page.
// Never surface the server's `message` field (English, technical) to users -
// the screen picks its own non-technical copy from this code.
type ShedReason = "too_many_streams" | "file_not_ready" | "calibrating" | "file_needs_remux" | null;

// expo-video's PlayerError is just `{ message: string }` - ExoPlayer/AVPlayer
// give no structured HTTP status or response body, so there is no reliable
// way to read the shed code off a native player error. Instead, probe the
// manifest URL directly before handing it to the player: the server returns
// the code both as an `X-Evey-Error` header and a JSON body (see
// transcode_handler_vod.go's writeShed). A second identical GET for the same
// token right after (the native player fetching the same URL) is expected
// and safe - repeated HLS manifest requests are idempotent, the same as a
// manifest reload on seek, so this doesn't consume a second capacity slot.
//
// ONLY the HLS master is probeable, and the guard stays even though the player
// no longer builds a /transcode URL (see buildStreamUrl): that endpoint spawns
// one ffmpeg PER GET and streams its stdout, so probing it started a phantom
// remux of the whole film that nothing ever read (the fetch body was never
// consumed, so the socket - and ffmpeg - lived on), while its refusal is a 429
// this 503 contract never matched anyway. The abort is part of the same
// guarantee: the probe wants a status line, never a body, and closing the
// connection is what tells the server the probe is done.
async function detectShed(url: string): Promise<ShedReason> {
  if (!url.includes("master.m3u8")) return null;
  const ctrl = new AbortController();
  try {
    const res = await fetch(url, { signal: ctrl.signal });
    if (res.status !== 503) return null;
    const headerCode = res.headers.get("X-Evey-Error");
    if (headerCode === "too_many_streams" || headerCode === "file_not_ready" || headerCode === "calibrating" || headerCode === "file_needs_remux") return headerCode;
    try {
      const body = (await res.json()) as { code?: string };
      if (body.code === "too_many_streams" || body.code === "file_not_ready" || body.code === "calibrating" || body.code === "file_needs_remux") return body.code;
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
  } finally {
    ctrl.abort();
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
  // Exposed so useCastSession can re-negotiate the URL against the RECEIVER's
  // capabilities via buildCastStreamUrl instead of casting the phone-negotiated
  // streamUrl as-is.
  serverUrl: string | null;
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
export async function fetchStreamToken(mediaId: string, opts?: { cast?: boolean }): Promise<string> {
  // Native player (expo-video/ExoPlayer/AVPlayer): like a Chromecast or a web
  // native <video>, it fetches segments itself and can't re-mint the short
  // sliding token mid-stream, so a pause would 401 on the 5-minute default.
  // purpose=native returns the long-lived (12h) window instead.
  //
  // opts.cast requests the CAST-marked variant of that same long-lived
  // window: the server's cast progress derivation (cast_observe.go) only
  // treats a token with Cast=true as a signal, so casting the native token
  // makes the receiver's fetches invisible to server-side progress tracking.
  const purpose = opts?.cast ? "cast" : "native";
  const res = await eveyFetch<StreamTokenResponse>({
    url: `/api/v1/media/${encodeURIComponent(mediaId)}/stream-token?purpose=${purpose}`,
    method: "POST",
  });
  return res.token;
}

function appendCodecParams(params: URLSearchParams, maxAudioChannels?: number): void {
  params.set("videoCodecs", VIDEO_CODECS.join(","));
  // Audio codecs come from the device's real decoder list (MediaCodecList via
  // the MediaCaps native module) - a hardcoded list with ac3/eac3 direct-played
  // Dolby tracks on devices with no Dolby decoder: video without sound.
  params.set("audioCodecs", getDeviceAudioCodecs().join(","));
  params.set("containers", CONTAINERS.join(","));
  // What the device takes muxed in fMP4, NOT what it decodes - see
  // getFmp4AudioPassthrough. Lets the server COPY an MP3 track (old DVD rips)
  // instead of cascading it through its AAC encoder. Omitted when empty so the
  // URL of every other platform stays byte-identical to before.
  const passthrough = getFmp4AudioPassthrough();
  if (passthrough.length > 0) params.set("fmp4AudioPassthrough", passthrough.join(","));
  // hdr=1 lets the decision keep the video COPY rung for HDR10/HLG sources on
  // an HDR panel (exact colors + on-demand seek) instead of tonemapping to SDR.
  if (getDeviceHdrSupport()) params.set("hdr", "1");
  // Channel ceiling of the OUTPUT, not of the decoder - see lib/outputChannels.
  // Omitted when the device cannot answer, which leaves the server profile's
  // own default in place: an unknown output must not silently lose surround.
  if (maxAudioChannels != null) params.set("maxAudioChannels", String(maxAudioChannels));
  // H.264 level ceiling of the hardware decoder: above it the server
  // transcodes instead of direct-playing a stream the device cannot open.
  const level = getDeviceMaxH264Level();
  if (level) params.set("maxH264Level", String(level));
}

// audioTrack/subtitleTrack are 0-based indices AMONG the audio/subtitle
// streams (what the server's clampAudioTrack expects), NOT the absolute
// media-stream index. They MUST land in the HLS master URL: the manifest's
// audio is chosen by ?audioTrack= (transcode_handler_vod.go), and - just as
// important - without them the URL is identical for every track, so switching
// language rebuilt the same URL and the native player never reloaded, leaving
// the audio on the default track. This mirrors the web buildTranscodeHlsUrl.
export function buildStreamUrl(
  serverUrl: string,
  mediaId: string,
  streamInfo: V1StreamInfo,
  token: string,
  audioTrack?: number,
  subtitleTrack?: number | null,
  maxAudioChannels?: number,
  startSec = 0,
  maxHeight?: number,
): string {
  if (streamInfo.playMethod === "PLAY_METHOD_DIRECT_PLAY") {
    return `${serverUrl}/api/v1/media/${encodeURIComponent(mediaId)}/stream?token=${token}`;
  }
  // DIRECT_STREAM (container-only mismatch, both streams copy-eligible) falls
  // through to the HLS master on purpose - it is NOT a separate URL shape.
  // /transcode?remux=1 pipes one ffmpeg's stdout per GET: chunked, no
  // Content-Length, `Accept-Ranges: none` (transcode_handler.go). ExoPlayer/
  // AVPlayer therefore mark that source UNSEEKABLE, and everything that is a
  // seek underneath dies with it - resume-at-position (starts at 0:00 instead),
  // dragging the timeline, and the double-tap skip. The HLS copy rung serves the
  // same copied bytes as fMP4 segments over a complete VOD manifest, so the
  // whole timeline is addressable. This is what the server itself answers for
  // direct_stream (playback_handler.go returns TranscodeUrl = the HLS master)
  // and what the web player does (HLSPlayer's transcodeMode covers every
  // non-direct-play method).
  const params = new URLSearchParams({ token });
  appendCodecParams(params, maxAudioChannels);
  if (audioTrack != null && audioTrack >= 0) params.set("audioTrack", String(audioTrack));
  if (subtitleTrack != null && subtitleTrack >= 0) params.set("subtitleTrack", String(subtitleTrack));
  // Server publishes a complete VOD manifest from t=0 (no live-edge snap), so
  // the player seeks natively on the absolute timeline. startSec is a PRE-WARM
  // hint only: the server opens the encoder at that position instead of 0
  // (a resume used to pay an encoder nobody read); it never shapes the manifest
  // and the client must not offset its clock by it.
  if (startSec > 0) params.set("startSec", String(Math.floor(startSec)));
  // The quality the viewer picked. It went to GetStreamInfo alone, which only
  // changed the stream-info verdict: the master never saw it and served the
  // same ladder whatever the menu said.
  if (maxHeight && maxHeight > 0) params.set("maxHeight", String(maxHeight));
  return `${serverUrl}/api/v1/media/${encodeURIComponent(mediaId)}/hls/master.m3u8?${params.toString()}`;
}

// The 0-based position of an absolute media-stream index among streams of one
// type (audio/subtitle). Returns undefined when the index is unset (<=0) or
// not found, so buildStreamUrl omits the param and the server keeps its default.
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

// Tracks chosen on the media page BEFORE playback (absolute stream indices):
// the first negotiation carries them, so the manifest opens on that audio.
export interface PlaySessionInitial {
  audioStreamIndex?: number;
  subtitleStreamIndex?: number;
}

export function usePlaySession(mediaId: string, initial?: PlaySessionInitial): PlaySessionResult {
  const initialRef = useRef(initial);
  const [streamUrl, setStreamUrl] = useState<string | null>(null);
  const [streamToken, setStreamToken] = useState<string | null>(null);
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [streamInfo, setStreamInfo] = useState<V1StreamInfo | null>(null);
  const [serverUrl, setServerUrl] = useState<string | null>(null);
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
  // Which negotiation is the current one. Two taps on the track picker start
  // two startStream() calls that both run to completion, and whichever answers
  // LAST used to win - so if the first server round-trip was the slower one,
  // the player silently went back to the track the user had just left, with no
  // error to explain it. stoppedRef does not cover this: neither call was
  // stopped, they simply raced. The web player already guards its equivalent
  // rebuild (useTranscodePlayback's `if (hls !== hlsRef.current) return`).
  const generationRef = useRef(0);

  const startStream = useCallback(
    async (audioStreamIndex?: number, subtitleStreamIndex?: number, maxHeight?: number) => {
      const generation = ++generationRef.current;
      const superseded = () => generation !== generationRef.current;
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

        const [info, wp, maxAudioChannels] = await Promise.all([
          playbackServiceGetStreamInfo(mediaId, {
            supportedCodecs: VIDEO_CODECS,
            supportedAudioCodecs: getDeviceAudioCodecs(),
            supportedContainers: CONTAINERS,
            // Same declaration the HLS master URL carries, so the stream-info
            // panel describes the bytes the segments will actually hold.
            fmp4AudioPassthrough: getFmp4AudioPassthrough(),
            supportsHdr: getDeviceHdrSupport(),
            maxH264Level: getDeviceMaxH264Level(),
            maxHeight: explicitMaxHeight,
            audioStreamIndex,
            subtitleStreamIndex,
            networkType,
          }),
          eveyFetch<V1WatchProgress>({
            url: `/api/v1/playback/${encodeURIComponent(mediaId)}/progress`,
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
        setStreamToken(token);
        // Convert the absolute stream indices the caller passed (from the
        // picker's s.index) to the 0-based per-type indices the manifest URL
        // needs, so a language switch produces a DIFFERENT url that reloads.
        const audioTrack = typeRelativeIndex(info, "MEDIA_STREAM_TYPE_AUDIO", audioStreamIndex);
        const subtitleTrack = typeRelativeIndex(info, "MEDIA_STREAM_TYPE_SUBTITLE", subtitleStreamIndex);
        const resumeSec = wp?.positionMs && !wp.played ? Number(wp.positionMs) / 1000 : 0;
        const url = buildStreamUrl(serverUrl, mediaId, info, token, audioTrack, subtitleTrack, maxAudioChannels, resumeSec, explicitMaxHeight);

        // Direct play streams the original file with no orchestrator involved,
        // so it cannot be shed - only probe the transcode/HLS paths.
        if (info.playMethod !== "PLAY_METHOD_DIRECT_PLAY") {
          const shed = await detectShed(url);
          if (shed) {
            if (!stoppedRef.current && !superseded()) setShedReason(shed);
            return;
          }
        }
        // A newer selection has been made while we were negotiating: this
        // answer is about a track the user has already left. Registering a
        // play session for it would also leave a stray session behind.
        if (superseded()) return;

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

        if (!stoppedRef.current && !superseded()) {
          streamInfoRef.current = info;
          setStreamInfo(info);
          setServerUrl(serverUrl);
          setStreamUrl(url);
          if (session.id) {
            sessionIdRef.current = session.id;
            setSessionId(session.id);
          }
        }
      } catch (err) {
        if (superseded()) return; // a superseded attempt must not fail the live one
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
    startStream(initialRef.current?.audioStreamIndex, initialRef.current?.subtitleStreamIndex);
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
      // Forget the cached stream token: a retry that reuses the token which
      // just 401'd rebuilds the same dead URL and fails identically - the
      // retry button was provably unable to repair the one failure it exists
      // for. Same fix the TV received (see tv usePlaySession).
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
  // the whole timeline - the player seeks natively (see seekTo/seekBy), which is
  // both instant and free of the offset bookkeeping that reported doubled positions.

  useEffect(() => {
    return () => stopSession();
  }, [stopSession]);

  return { streamUrl, streamToken, sessionId, streamInfo, serverUrl, isLoading, isError, shedReason, stopSession, restartWithTracks, progress };
}
