import { useState, useEffect, useRef, useCallback } from "react";
import { Platform } from "react-native";
import {
  playbackServiceGetStreamInfo,
  playbackServiceStartStream,
  playbackServiceStopStream,
  eveyFetch,
} from "@evey/api";
import type { V1StreamInfo, V1WatchProgress } from "@evey/api";
import { getDeviceAudioCodecs, getDeviceMaxH264Level, getFmp4AudioPassthrough } from "./deviceCodecs";

// The play session of the two native players (phone and television). They
// kept a copy each until 2026-09-24, and the copies had drifted apart: the TV
// probed the unseekable /transcode pipe without aborting it and let the slower
// of two track switches win, the phone showed an expired session as a stream
// failure. What differs between the devices is passed in as PlaySessionDevice.

export const VIDEO_CODECS = ["h264", "hevc", "vp9"];
// Android's player (ExoPlayer/AndroidXMedia3) demuxes Matroska/WebM natively, so
// a compatible-codec .mkv (the bulk of a self-hosted library) DIRECT-PLAYS
// instead of being transcoded for nothing - which is what breaks in-timeline
// seeking and shifts colors. Measured 2026-09-19 on the living-room box: with
// mp4 alone, a 1.1 Mbps H.264 + AAC stereo film went through ffmpeg, audio
// drifting off the picture. AVPlayer (iPhone, Apple TV) reads neither
// container, so Apple keeps mp4/hls and lets the server remux.
export const CONTAINERS = Platform.OS === "android" ? ["mp4", "mkv", "webm", "hls"] : ["mp4", "hls"];

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
export type ShedReason = "too_many_streams" | "file_not_ready" | "calibrating" | "file_needs_remux" | null;

function isShedCode(code: string | null | undefined): code is Exclude<ShedReason, null> {
  return code === "too_many_streams" || code === "file_not_ready" || code === "calibrating" || code === "file_needs_remux";
}

// expo-video's PlayerError is just `{ message: string }` - ExoPlayer/AVPlayer
// give no structured HTTP status or response body, so the shed code cannot be
// read off a native player error. The manifest URL is probed directly before
// the player gets it: the server returns the code both as an `X-Evey-Error`
// header and a JSON body (transcode_handler_vod.go's writeShed). The player's
// own GET of the same manifest right after is idempotent, like a manifest
// reload on seek, and does not consume a second capacity slot.
//
// ONLY the HLS master is probeable: /transcode spawns one ffmpeg PER GET and
// streams its stdout, so probing it started a phantom remux of the whole film
// that nothing ever read, while its refusal is a 429 this 503 contract never
// matched anyway. The abort is part of the same guarantee: the probe wants a
// status line, never a body, and closing the connection tells the server the
// probe is done.
export async function detectShed(url: string): Promise<ShedReason> {
  if (!url.includes("master.m3u8")) return null;
  const ctrl = new AbortController();
  try {
    const res = await fetch(url, { signal: ctrl.signal });
    if (res.status !== 503) return null;
    const headerCode = res.headers.get("X-Evey-Error");
    if (isShedCode(headerCode)) return headerCode;
    try {
      const body = (await res.json()) as { code?: string };
      if (isShedCode(body.code)) return body.code;
    } catch {
      // Not parseable JSON - fall through to the default below.
    }
    // No usable code: the more common cause, whose "try again shortly" copy
    // reads fine either way.
    return "too_many_streams";
  } catch {
    // The preflight itself failed (offline, DNS, ...) - let the native player
    // attempt playback and surface its own error if it also fails.
    return null;
  } finally {
    ctrl.abort();
  }
}

// mediaId reaches these builders straight from the route params, which a deep
// link controls. Every id is percent-encoded so a crafted "..%2Fadmin" id
// cannot pivot the authenticated request onto another API path.
export async function fetchStreamToken(mediaId: string, opts?: { cast?: boolean }): Promise<string> {
  // A native player fetches segments itself and can't re-mint the short
  // sliding token mid-stream, so a pause would 401 on the 5-minute default.
  // purpose=native returns the long-lived (12h) window instead.
  //
  // opts.cast requests the CAST-marked variant of that same window: the
  // server's cast progress derivation (cast_observe.go) only treats a token
  // with Cast=true as a signal.
  const purpose = opts?.cast ? "cast" : "native";
  const res = await eveyFetch<StreamTokenResponse>({
    url: `/api/v1/media/${encodeURIComponent(mediaId)}/stream-token?purpose=${purpose}`,
    method: "POST",
  });
  return res.token;
}

export interface StreamUrlOptions {
  // 0-based indices AMONG the audio/subtitle streams (what the server's
  // clampAudioTrack expects), NOT the absolute media-stream index.
  audioTrack?: number;
  subtitleTrack?: number | null;
  maxAudioChannels?: number;
  startSec?: number;
  maxHeight?: number;
  screenHeight?: number;
  supportsHdr?: boolean;
}

function appendCodecParams(params: URLSearchParams, opts: StreamUrlOptions): void {
  params.set("videoCodecs", VIDEO_CODECS.join(","));
  // The device's real decoder list (MediaCodecList via the MediaCaps module):
  // a hardcoded list with ac3/eac3 direct-played Dolby tracks on devices with
  // no Dolby decoder - video without sound.
  params.set("audioCodecs", getDeviceAudioCodecs().join(","));
  params.set("containers", CONTAINERS.join(","));
  // What the device takes muxed in fMP4, NOT what it decodes - see
  // getFmp4AudioPassthrough. Omitted when empty so the URL of every other
  // platform stays byte-identical to before.
  const passthrough = getFmp4AudioPassthrough();
  if (passthrough.length > 0) params.set("fmp4AudioPassthrough", passthrough.join(","));
  // hdr=1 keeps the video COPY rung for HDR10/HLG sources on an HDR panel
  // instead of tonemapping to SDR. Omitted when false.
  if (opts.supportsHdr) params.set("hdr", "1");
  // Channel ceiling of the OUTPUT, not of the decoder - see outputChannels.
  // Omitted when the device cannot answer: an unknown output must not
  // silently lose surround.
  if (opts.maxAudioChannels != null) params.set("maxAudioChannels", String(opts.maxAudioChannels));
  // H.264 level ceiling of the hardware decoder: above it the server
  // transcodes instead of direct-playing a stream the device cannot open.
  const level = getDeviceMaxH264Level();
  if (level) params.set("maxH264Level", String(level));
}

// The tracks MUST land in the HLS master URL: the manifest's audio is picked
// by ?audioTrack= (transcode_handler_vod.go), a bitmap subtitle is burnt in
// only when ?subtitleTrack= names it, and without them the URL is identical
// for every track, so a language switch never reloaded the native player.
export function buildStreamUrl(
  serverUrl: string,
  mediaId: string,
  streamInfo: V1StreamInfo,
  token: string,
  opts: StreamUrlOptions = {},
): string {
  const id = encodeURIComponent(mediaId);
  if (streamInfo.playMethod === "PLAY_METHOD_DIRECT_PLAY") {
    return `${serverUrl}/api/v1/media/${id}/stream?token=${token}`;
  }
  // DIRECT_STREAM falls through to the HLS master on purpose. /transcode?remux=1
  // pipes one ffmpeg's stdout per GET with `Accept-Ranges: none`, so the native
  // player marks it UNSEEKABLE and resume, timeline drag and skip all die. The
  // HLS copy rung serves the same copied bytes over a complete VOD manifest -
  // what the server itself answers for direct_stream and what the web does.
  const params = new URLSearchParams({ token });
  appendCodecParams(params, opts);
  if (opts.audioTrack != null && opts.audioTrack >= 0) params.set("audioTrack", String(opts.audioTrack));
  if (opts.subtitleTrack != null && opts.subtitleTrack >= 0) params.set("subtitleTrack", String(opts.subtitleTrack));
  // A PRE-WARM hint only: the encoder opens at the resume position; the
  // manifest still starts at 0 and the client must not offset its clock by it.
  const startSec = opts.startSec ?? 0;
  if (startSec > 0) params.set("startSec", String(Math.floor(startSec)));
  // The quality the viewer picked: sent to GetStreamInfo alone, the master
  // served the same ladder whatever the menu said.
  if (opts.maxHeight && opts.maxHeight > 0) params.set("maxHeight", String(opts.maxHeight));
  // The panel's height trims the transcode ladder (no 2160p rung for a 1080p
  // panel) without turning a copy-playable title into a transcode.
  if (opts.screenHeight && opts.screenHeight > 0) params.set("screenHeight", String(opts.screenHeight));
  return `${serverUrl}/api/v1/media/${id}/hls/master.m3u8?${params.toString()}`;
}

// The 0-based position of an absolute media-stream index among streams of one
// type. Undefined when the index is unset (<=0) or not found, so the param is
// omitted and the server keeps its default.
export function typeRelativeIndex(
  streamInfo: V1StreamInfo | undefined,
  type: "MEDIA_STREAM_TYPE_AUDIO" | "MEDIA_STREAM_TYPE_SUBTITLE",
  absoluteIndex: number | undefined,
): number | undefined {
  if (absoluteIndex == null || absoluteIndex <= 0) return undefined;
  const ofType = (streamInfo?.mediaStreams ?? []).filter((s) => s.type === type);
  const pos = ofType.findIndex((s) => (s.index ?? -1) === absoluteIndex);
  return pos >= 0 ? pos : undefined;
}

// What one native app brings. The probes are passed in rather than imported so
// each app keeps its own module paths (its tests double them there).
export interface PlaySessionDevice {
  id: string;
  name: string;
  getServerUrl: () => Promise<string | null>;
  // Re-probed at every negotiation, not memoised: the output changes
  // mid-session (headphones, dock, amplifier, HDMI re-plugged).
  probeMaxOutputChannels: () => Promise<number | undefined>;
  probeHdr: () => Promise<boolean> | boolean;
  // Pixel height used to trim the transcode ladder; omit to leave it whole.
  screenHeight?: () => number | undefined;
  // For the profile's cellular bitrate cap; omit where there is no cellular.
  networkType?: () => Promise<"wifi" | "cellular">;
  warn?: (...args: unknown[]) => void;
}

export interface PlaySessionResult {
  streamUrl: string | null;
  // Exposed so the native player can send it as an Authorization: Bearer
  // header on the source - the server accepts cookie/Bearer/query alike.
  streamToken: string | null;
  sessionId: string | null;
  streamInfo: V1StreamInfo | null;
  // Exposed so a cast can re-negotiate the URL against the RECEIVER.
  serverUrl: string | null;
  isLoading: boolean;
  isError: boolean;
  // Set instead of isError when the session is no longer valid: nothing is
  // wrong with the stream, the viewer has to sign in again. Collapsing it into
  // isError put "Failed to load stream" on screen for an expired token.
  isAuthError: boolean;
  // Set instead of isError when the server refused to start THIS request.
  shedReason: ShedReason;
  stopSession: () => void;
  restartWithTracks: (audioIndex?: number, subtitleIndex?: number, maxHeight?: number) => void;
  progress: V1WatchProgress | null;
}

// Tracks chosen on the media page BEFORE playback (absolute stream indices):
// the first negotiation carries them, so the manifest opens on that audio.
export interface PlaySessionInitial {
  audioStreamIndex?: number;
  subtitleStreamIndex?: number;
}

export function usePlaySession(mediaId: string, device: PlaySessionDevice, initial?: PlaySessionInitial): PlaySessionResult {
  const initialRef = useRef(initial);
  const deviceRef = useRef(device);
  deviceRef.current = device;
  const [streamUrl, setStreamUrl] = useState<string | null>(null);
  const [streamToken, setStreamToken] = useState<string | null>(null);
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [streamInfo, setStreamInfo] = useState<V1StreamInfo | null>(null);
  const [serverUrl, setServerUrl] = useState<string | null>(null);
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
  // Which negotiation is the current one. Two taps on the track picker start
  // two startStream() calls that both run to completion, and whichever
  // answered LAST used to win - the player silently went back to the track the
  // viewer had just left. The web guards its equivalent rebuild the same way.
  const generationRef = useRef(0);

  const startStream = useCallback(
    async (audioStreamIndex?: number, subtitleStreamIndex?: number, maxHeight?: number) => {
      const generation = ++generationRef.current;
      const superseded = () => generation !== generationRef.current;
      const dev = deviceRef.current;
      try {
        const serverUrl = serverUrlRef.current || ((await dev.getServerUrl()) ?? "");
        serverUrlRef.current = serverUrl;
        // Screen size is NOT a decode capability (the display downscales for
        // free): declaring it to GetStreamInfo flagged resolution_exceeded and
        // transcoded titles the device plays natively. Only an explicit quality
        // pick is a ceiling; the screen rides the master URL as ?screenHeight.
        const explicitMaxHeight = maxHeight && maxHeight > 0 ? maxHeight : undefined;

        // The device probes run BEFORE the negotiation: GetStreamInfo has to
        // describe the same client the master URL will declare. They are local
        // native calls, the wait is not a network one.
        const [maxAudioChannels, supportsHdr, networkType] = await Promise.all([
          dev.probeMaxOutputChannels(),
          dev.probeHdr(),
          dev.networkType?.(),
        ]);

        const [info, wp] = await Promise.all([
          playbackServiceGetStreamInfo(mediaId, {
            supportedCodecs: VIDEO_CODECS,
            supportedAudioCodecs: getDeviceAudioCodecs(),
            supportedContainers: CONTAINERS,
            // Same declaration the HLS master URL carries, so the stream-info
            // panel describes the bytes the segments will actually hold.
            fmp4AudioPassthrough: getFmp4AudioPassthrough(),
            supportsHdr,
            maxH264Level: getDeviceMaxH264Level(),
            maxHeight: explicitMaxHeight,
            audioStreamIndex,
            subtitleStreamIndex,
            ...(networkType ? { networkType } : {}),
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
        // The picker hands absolute stream indices; the manifest counts per
        // type, so a language switch produces a DIFFERENT url that reloads.
        const resumeSec = wp?.positionMs && !wp.played ? Number(wp.positionMs) / 1000 : 0;
        const url = buildStreamUrl(serverUrl, mediaId, info, token, {
          audioTrack: typeRelativeIndex(info, "MEDIA_STREAM_TYPE_AUDIO", audioStreamIndex),
          subtitleTrack: typeRelativeIndex(info, "MEDIA_STREAM_TYPE_SUBTITLE", subtitleStreamIndex),
          maxAudioChannels,
          startSec: resumeSec,
          maxHeight: explicitMaxHeight,
          screenHeight: dev.screenHeight?.(),
          supportsHdr,
        });

        // Direct play streams the original file with no orchestrator involved,
        // so it cannot be shed - only probe the HLS path.
        if (info.playMethod !== "PLAY_METHOD_DIRECT_PLAY") {
          const shed = await detectShed(url);
          if (shed) {
            if (!stoppedRef.current && !superseded()) setShedReason(shed);
            return;
          }
        }
        // A newer selection was made while this one negotiated: registering a
        // play session for it would leave a stray session behind.
        if (superseded()) return;

        const startPosMs = wp?.positionMs && !wp.played ? wp.positionMs : "0";
        const session = await playbackServiceStartStream({
          mediaItemId: mediaId,
          playMethod: info.playMethod ?? "PLAY_METHOD_TRANSCODE",
          deviceId: dev.id,
          deviceName: dev.name,
          clientName: dev.name,
          startPositionMs: startPosMs,
          audioStreamIndex,
          subtitleStreamIndex,
        });

        if (!stoppedRef.current && !superseded()) {
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
        (dev.warn ?? console.warn)("[player] startStream failed:", err);
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
    // Mounted again: a rehearsal unmount (StrictMode) ran stopSession while
    // the first negotiation was in flight, which then dropped its answer.
    stoppedRef.current = false;
    if (!mediaId || startedRef.current) return;
    startedRef.current = true;
    startStream(initialRef.current?.audioStreamIndex, initialRef.current?.subtitleStreamIndex);
  }, [mediaId, startStream]);

  const stopSession = useCallback(() => {
    if (stoppedRef.current) return;
    stoppedRef.current = true;
    const sid = sessionIdRef.current;
    if (sid) {
      playbackServiceStopStream({ sessionId: sid }).catch((err) =>
        (deviceRef.current.warn ?? console.warn)("stop session failed:", err),
      );
    }
  }, []);

  const restartWithTracks = useCallback(
    (audioIndex?: number, subtitleIndex?: number, maxHeight?: number) => {
      const sid = sessionIdRef.current;
      if (sid) {
        playbackServiceStopStream({ sessionId: sid }).catch((err) =>
          (deviceRef.current.warn ?? console.warn)("stop before restart failed:", err),
        );
        sessionIdRef.current = null;
      }
      stoppedRef.current = false;
      // Forget the cached stream token: a retry that reuses the token which
      // just 401'd rebuilds the same dead URL and fails identically.
      tokenRef.current = null;
      setIsError(false);
      setIsAuthError(false);
      setShedReason(null);
      setIsLoading(true);
      startStream(audioIndex, subtitleIndex, maxHeight);
    },
    [startStream],
  );

  // No restartAtPosition: the VOD manifest covers the whole timeline and the
  // player seeks natively, free of the offset bookkeeping that doubled positions.

  useEffect(() => {
    return () => stopSession();
  }, [stopSession]);

  return { streamUrl, streamToken, sessionId, streamInfo, serverUrl, isLoading, isError, isAuthError, shedReason, stopSession, restartWithTracks, progress };
}
