import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  getBaseURL,
  getCredentialsMode,
  usePlaybackServiceGetWatchProgress,
  usePlaybackServiceGetStreamInfo,
  playbackServiceStartStream,
} from "@evey/api";
import type { V1StreamInfo, V1WatchProgress } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { getBrowserCapabilities, canWasmDecodeAC3 } from "./codec";
import { appendCodecParams, prefersNativeHLS, getFmp4AudioPassthrough } from "./hlsCodecParams";
import { useStreamToken } from "../hooks/useStreamToken";

// postStopStream tears down the server-side play session with a raw keepalive
// fetch (not eveyFetch / react-query). It must fire reliably during teardown:
// page unload (pagehide), route unmount, AND from the mini-player after the
// player route has already unmounted - a react-query mutation bound to an
// unmounted component is not a dependable transport here, which is exactly why
// closing the player via the browser back button (player -> mini -> close) used
// to leave the transcode session running in the admin panel. The active
// session token is fresh; a 401 is silently dropped (the session is gone).
// The base URL is the fetcher's: empty on the web app (same origin), the
// chosen server on a packaged TV client.
function postStopStream(sessionId: string) {
  const token = useAuthStore.getState().accessToken;
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  if (token) headers["Authorization"] = `Bearer ${token}`;
  const csrfMatch = typeof document !== "undefined"
    ? document.cookie.match(/(?:^|;\s*)_csrf=([^;]*)/)
    : null;
  if (csrfMatch) headers["X-CSRF-Token"] = csrfMatch[1];
  void fetch(`${getBaseURL()}/api/v1/playback/stop`, {
    method: "POST",
    credentials: getCredentialsMode(),
    headers,
    body: JSON.stringify({ sessionId }),
    keepalive: true,
  }).catch((err) => console.warn("playback stop failed", err));
}

// servedCodecLabel builds the per-track codec label persisted on the session
// for the admin panel: the source codec on a copy (source == served), or a
// "source->target" form on a transcode. Language-neutral tokens; the admin UI
// adds the localized "copié / transcodé" wording around it.
function servedCodecLabel(decision: string | undefined, source: string, target: string | undefined): string {
  if (decision === "transcode" && target) return source ? `${source}→${target}` : target;
  return source;
}

function sourceCodec(streams: V1StreamInfo["mediaStreams"], kind: "VIDEO" | "AUDIO"): string {
  const s = (streams ?? []).find((x) => x.type === `MEDIA_STREAM_TYPE_${kind}`);
  return s?.codec ?? "";
}

export interface PlaySessionDevice {
  deviceId: string;
  deviceName: string;
  clientName: string;
  /**
   * Whether this player runs the ac3go wasm engine for AC-3/E-AC-3. The web
   * app does and says so when the browser cannot decode them natively; a
   * client without the engine must declare false, or the server serves an
   * out-of-band audio rendition nothing will decode.
   */
  wasmAudio?: boolean;
}

const WEB_DEVICE: PlaySessionDevice = { deviceId: "web-browser", deviceName: "Web Browser", clientName: "Evey Web" };

export interface PlaySessionResult {
  streamInfo: V1StreamInfo | null;
  sessionId: string | null;
  streamToken: string | null;
  fetchStreamToken: (mediaId: string, opts?: { cast?: boolean }) => Promise<string>;
  isLoading: boolean;
  isError: boolean;
  progress: V1WatchProgress | null;
  stopSession: () => void;
}

/**
 * Negotiates a play session for a browser player: declares what this
 * browser decodes, resolves the stream URL the server chose, starts the
 * session and ends it on every way out. `device` names the client in the
 * admin's Now Playing panel; the web app is the default, a television
 * browser says so.
 */
export function usePlaySession(mediaId: string, device: PlaySessionDevice = WEB_DEVICE): PlaySessionResult {
  const [sessionId, setSessionId] = useState<string | null>(null);
  const sessionIdRef = useRef<string | null>(null);
  const stopped = useRef(false);
  const startedRef = useRef(false);
  const prevMediaIdRef = useRef(mediaId);
  if (prevMediaIdRef.current !== mediaId) {
    prevMediaIdRef.current = mediaId;
    startedRef.current = false;
  }
  const [starting, setStarting] = useState(false);
  const [startError, setStartError] = useState(false);
  const { streamToken, fetchStreamToken } = useStreamToken();
  const streamTokenRef = useRef<string | null>(null);

  const caps = useMemo(() => getBrowserCapabilities(), []);
  // No maxWidth/maxHeight: the screen size is NOT a decode capability. Sending
  // window.screen dims made the server flag resolution_exceeded and transcode
  // a 4K title for a 1366px laptop that decodes it natively (it should
  // direct-play there). The browser downscales for display at zero cost;
  // bandwidth stays governed by the profile's bitrate prefs, and the transcode
  // fallback remains codec/HDR-driven.
  const streamInfoParams = useMemo(() => ({
    supportedCodecs: caps.videoCodecs,
    supportedAudioCodecs: caps.audioCodecs,
    supportedContainers: caps.containers,
    supportsHdr: caps.hdr,
    // Deliberately not in supportedAudioCodecs - see canWasmDecodeAC3. Without
    // it the server calls the audio "transcode" and serves it untouched.
    wasmAudio: device.wasmAudio ?? canWasmDecodeAC3(),
    // Same declaration appendCodecParams puts on the HLS master URL, so the
    // stream-info panel describes the bytes the segments will actually carry.
    // Sending one and not the other is how the panel came to claim a copy the
    // packager was re-encoding (2026-09-06).
    fmp4AudioPassthrough: getFmp4AudioPassthrough(),
  }), [caps, device.wasmAudio]);

  const { data: progress } = usePlaybackServiceGetWatchProgress(mediaId, undefined, {
    query: { retry: false },
  });
  const { data: rawStreamInfo } = usePlaybackServiceGetStreamInfo(mediaId, streamInfoParams);

  const currentStreamToken = streamTokenRef.current;
  const streamInfo: V1StreamInfo | undefined = rawStreamInfo
    ? (() => {
        let url: string | undefined;
        if (rawStreamInfo.playMethod === "PLAY_METHOD_DIRECT_PLAY") {
          url = currentStreamToken
            ? `${rawStreamInfo.directPlayUrl ?? `/api/v1/media/${mediaId}/stream`}?token=${currentStreamToken}`
            : rawStreamInfo.directPlayUrl;
        } else {
          // DIRECT_STREAM (container-only remux) and TRANSCODE both play the
          // HLS master: the copy rung serves the remux as fMP4 segments, the
          // same URL shape as a transcode. There is no progressive pipe.
          if (currentStreamToken) {
            const p = new URLSearchParams({ token: currentStreamToken });
            appendCodecParams(p);
            url = `/api/v1/media/${mediaId}/hls/master.m3u8?${p.toString()}`;
          } else {
            url = rawStreamInfo.transcodeUrl;
          }
        }
        return { ...rawStreamInfo, directPlayUrl: url };
      })()
    : undefined;

  useEffect(() => {
    stopped.current = false;
    if (!mediaId || startedRef.current || !rawStreamInfo) return;
    startedRef.current = true;
    setStarting(true);

    const playMethod = rawStreamInfo.playMethod === "PLAY_METHOD_DIRECT_PLAY"
      ? "PLAY_METHOD_DIRECT_PLAY"
      : rawStreamInfo.playMethod === "PLAY_METHOD_DIRECT_STREAM"
        ? "PLAY_METHOD_DIRECT_STREAM"
        : "PLAY_METHOD_TRANSCODE";

    (async () => {
      try {
        // A native <video> HLS session (Safari/iOS) can't re-mint the sliding
        // token mid-stream, so it gets the long-lived window up front; the
        // httpOnly evey_hls cookie then carries it across segments/pauses.
        const token = await fetchStreamToken(
          mediaId,
          prefersNativeHLS() ? { native: true } : undefined,
        );
        streamTokenRef.current = token;
        const session = await playbackServiceStartStream({
          mediaItemId: mediaId,
          playMethod,
          deviceId: device.deviceId,
          deviceName: device.deviceName,
          clientName: device.clientName,
          startPositionMs: "0",
          // Report the bitrate we'll actually pull (from the StreamInfo we just
          // resolved) so the admin dashboard shows the served rate, not source.
          servedBitrateBps: rawStreamInfo.bitrate ?? "0",
          // Per-track serving decision + served codecs/resolution so the admin
          // "Now Playing" panel can distinguish a copy rung from a real transcode.
          videoDecision: rawStreamInfo.videoDecision ?? "",
          audioDecision: rawStreamInfo.audioDecision ?? "",
          videoCodec: servedCodecLabel(rawStreamInfo.videoDecision, sourceCodec(rawStreamInfo.mediaStreams, "VIDEO"), rawStreamInfo.videoCodec),
          audioCodec: servedCodecLabel(rawStreamInfo.audioDecision, sourceCodec(rawStreamInfo.mediaStreams, "AUDIO"), rawStreamInfo.audioCodec),
          resolution: rawStreamInfo.resolution ?? "",
        });
        if (session.id) {
          sessionIdRef.current = session.id;
          setSessionId(session.id);
        }
      } catch {
        setStartError(true);
      } finally {
        setStarting(false);
      }
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps -- fire-once per mediaId via startedRef guard; waits for rawStreamInfo to determine correct playMethod
  }, [mediaId, rawStreamInfo]);

  const stopSession = useCallback(() => {
    if (stopped.current) return;
    stopped.current = true;
    const sid = sessionIdRef.current;
    if (sid) postStopStream(sid);
  }, []);

  useEffect(() => {
    const handleUnload = () => {
      const sid = sessionIdRef.current;
      if (sid && !stopped.current) {
        stopped.current = true;
        postStopStream(sid);
      }
    };
    window.addEventListener("pagehide", handleUnload);
    return () => {
      window.removeEventListener("pagehide", handleUnload);
      // Leaving the player (back button, route change, tab close) always ends
      // the session - there is no mini-player handoff that keeps it alive.
      stopSession();
    };
  }, [stopSession]);

  return {
    streamInfo: streamInfo ?? null,
    sessionId,
    streamToken,
    fetchStreamToken,
    isLoading: starting || (!rawStreamInfo && !startError),
    isError: startError,
    progress: progress ?? null,
    stopSession,
  };
}
