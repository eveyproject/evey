import { useCallback, useEffect, useRef, useState } from "react";
import {
  eveyFetch,
  getBaseURL,
  playbackServiceGetStreamInfo,
  playbackServiceStartStream,
  playbackServiceStopStream,
} from "@evey/api";
import type { V1StreamInfo, V1WatchProgress } from "@evey/api";
import { getBrowserCapabilities } from "./codec";
import { appendCodecParams } from "./hlsCodecParams";
import type { PlaySessionDevice } from "./usePlaySession";

// The television-browser play session: the same shape as the Android TV
// one (apps/tv/src/hooks/usePlaySession.ts) so the player screen can be
// written once against both - a stream url, the negotiated stream info,
// a restart that carries the viewer's audio, burn-in subtitle and quality
// picks, and the transcoder's "not now" answers surfaced as a reason rather
// than a dead player.

export type ShedReason = "too_many_streams" | "file_not_ready" | "calibrating" | "file_needs_remux" | null;

export interface TvPlaySessionResult {
  streamUrl: string | null;
  sessionId: string | null;
  streamInfo: V1StreamInfo | null;
  isLoading: boolean;
  isError: boolean;
  shedReason: ShedReason;
  progress: V1WatchProgress | null;
  stopSession: () => void;
  /** Tear the session down and negotiate again with these absolute stream indices and ceiling. */
  restartWithTracks: (audioIndex?: number, subtitleIndex?: number, maxHeight?: number) => void;
}

// A 503 with an Evey code is the orchestrator saying "not now" (all encoder
// slots busy, the file still being prepared or calibrated): shown to the
// viewer as such, with a retry, not as a broken title.
async function detectShed(url: string, token: string | null): Promise<ShedReason> {
  try {
    const res = await fetch(url, { headers: token ? { Authorization: `Bearer ${token}` } : undefined });
    if (res.status !== 503) return null;
    const known = new Set(["too_many_streams", "file_not_ready", "calibrating", "file_needs_remux"]);
    const header = res.headers.get("X-Evey-Error");
    if (header && known.has(header)) return header as ShedReason;
    try {
      const body = (await res.json()) as { code?: string };
      if (body.code && known.has(body.code)) return body.code as ShedReason;
    } catch { /* no body */ }
    return "too_many_streams";
  } catch {
    return null;
  }
}

// The master carries subtitle/audio ordinals RELATIVE to their type, the
// stream info carries absolute indices: translate before building the url.
function typeRelativeIndex(
  info: V1StreamInfo,
  type: "MEDIA_STREAM_TYPE_AUDIO" | "MEDIA_STREAM_TYPE_SUBTITLE",
  absoluteIndex: number | undefined,
): number | undefined {
  if (absoluteIndex == null || absoluteIndex <= 0) return undefined;
  const ofType = (info.mediaStreams ?? []).filter((s) => s.type === type);
  const pos = ofType.findIndex((s) => (s.index ?? -1) === absoluteIndex);
  return pos >= 0 ? pos : undefined;
}

export function buildTvStreamUrl(
  mediaId: string,
  info: V1StreamInfo,
  token: string,
  opts: { audioTrack?: number; subtitleTrack?: number; startSec?: number; maxHeight?: number; screenHeight?: number },
): string {
  const base = getBaseURL();
  if (info.playMethod === "PLAY_METHOD_DIRECT_PLAY") {
    return `${base}${info.directPlayUrl ?? `/api/v1/media/${mediaId}/stream`}?token=${token}`;
  }
  // DIRECT_STREAM and TRANSCODE both play the HLS master (the copy rung serves
  // the remux as fMP4 segments): there is no progressive pipe on a TV.
  const params = new URLSearchParams({ token });
  appendCodecParams(params);
  if (opts.audioTrack != null && opts.audioTrack >= 0) params.set("audioTrack", String(opts.audioTrack));
  if (opts.subtitleTrack != null && opts.subtitleTrack >= 0) params.set("subtitleTrack", String(opts.subtitleTrack));
  // A pre-warm hint for the encoder only: the manifest covers the whole
  // timeline and the player seeks natively.
  if (opts.startSec && opts.startSec > 0) params.set("startSec", String(Math.floor(opts.startSec)));
  if (opts.maxHeight && opts.maxHeight > 0) params.set("maxHeight", String(opts.maxHeight));
  if (opts.screenHeight && opts.screenHeight > 0) params.set("screenHeight", String(opts.screenHeight));
  return `${base}/api/v1/media/${mediaId}/hls/master.m3u8?${params.toString()}`;
}

export interface TvPlaySessionInitial {
  audioStreamIndex?: number;
  subtitleStreamIndex?: number;
}

export function useTvPlaySession(mediaId: string, device: PlaySessionDevice, initial?: TvPlaySessionInitial): TvPlaySessionResult {
  const initialRef = useRef(initial);
  const [streamUrl, setStreamUrl] = useState<string | null>(null);
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [streamInfo, setStreamInfo] = useState<V1StreamInfo | null>(null);
  const [progress, setProgress] = useState<V1WatchProgress | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isError, setIsError] = useState(false);
  const [shedReason, setShedReason] = useState<ShedReason>(null);
  const sessionIdRef = useRef<string | null>(null);
  const startedRef = useRef(false);
  const stoppedRef = useRef(false);
  const tokenRef = useRef<string | null>(null);
  const prevMediaIdRef = useRef(mediaId);
  if (prevMediaIdRef.current !== mediaId) {
    prevMediaIdRef.current = mediaId;
    startedRef.current = false;
  }

  const startStream = useCallback(async (audioStreamIndex?: number, subtitleStreamIndex?: number, maxHeight?: number) => {
    try {
      const caps = getBrowserCapabilities();
      const screenHeight = Math.round(Math.min(window.screen.height, window.screen.width) * (window.devicePixelRatio || 1));
      const [info, wp] = await Promise.all([
        playbackServiceGetStreamInfo(mediaId, {
          supportedCodecs: caps.videoCodecs,
          supportedAudioCodecs: caps.audioCodecs,
          supportedContainers: caps.containers,
          supportsHdr: caps.hdr,
          wasmAudio: device.wasmAudio ?? false,
          maxHeight: maxHeight && maxHeight > 0 ? maxHeight : undefined,
          audioStreamIndex,
          subtitleStreamIndex,
        }),
        eveyFetch<V1WatchProgress>({ url: `/api/v1/playback/${mediaId}/progress`, method: "GET" }).catch(() => null),
      ]);
      if (wp) setProgress(wp);

      let token = tokenRef.current;
      if (!token) {
        const res = await eveyFetch<{ token: string }>({ url: `/api/v1/media/${encodeURIComponent(mediaId)}/stream-token?purpose=native`, method: "POST" });
        token = res.token;
        tokenRef.current = token;
      }
      const resumeSec = wp?.positionMs && !wp.played ? Number(wp.positionMs) / 1000 : 0;
      const url = buildTvStreamUrl(mediaId, info, token, {
        audioTrack: typeRelativeIndex(info, "MEDIA_STREAM_TYPE_AUDIO", audioStreamIndex),
        subtitleTrack: typeRelativeIndex(info, "MEDIA_STREAM_TYPE_SUBTITLE", subtitleStreamIndex),
        startSec: resumeSec,
        maxHeight,
        screenHeight,
      });

      // Direct play streams the file with no orchestrator involved: only the
      // transcode/HLS paths can be shed.
      if (info.playMethod !== "PLAY_METHOD_DIRECT_PLAY") {
        const shed = await detectShed(url, null);
        if (shed) {
          if (!stoppedRef.current) setShedReason(shed);
          return;
        }
      }

      const session = await playbackServiceStartStream({
        mediaItemId: mediaId,
        playMethod: info.playMethod ?? "PLAY_METHOD_TRANSCODE",
        deviceId: device.deviceId,
        deviceName: device.deviceName,
        clientName: device.clientName,
        startPositionMs: wp?.positionMs && !wp.played ? wp.positionMs : "0",
        audioStreamIndex,
        subtitleStreamIndex,
        servedBitrateBps: info.bitrate ?? "0",
        videoDecision: info.videoDecision ?? "",
        audioDecision: info.audioDecision ?? "",
        resolution: info.resolution ?? "",
      });
      if (!stoppedRef.current) {
        setStreamInfo(info);
        setStreamUrl(url);
        if (session.id) {
          sessionIdRef.current = session.id;
          setSessionId(session.id);
        }
      }
    } catch {
      setIsError(true);
    } finally {
      setIsLoading(false);
    }
  }, [mediaId, device.deviceId, device.deviceName, device.clientName, device.wasmAudio]);

  useEffect(() => {
    if (!mediaId || startedRef.current) return;
    startedRef.current = true;
    stoppedRef.current = false;
    void startStream(initialRef.current?.audioStreamIndex, initialRef.current?.subtitleStreamIndex);
  }, [mediaId, startStream]);

  const stopSession = useCallback(() => {
    if (stoppedRef.current) return;
    stoppedRef.current = true;
    const sid = sessionIdRef.current;
    if (sid) playbackServiceStopStream({ sessionId: sid }).catch(() => {});
  }, []);

  const restartWithTracks = useCallback((audioIndex?: number, subtitleIndex?: number, maxHeight?: number) => {
    const sid = sessionIdRef.current;
    if (sid) {
      playbackServiceStopStream({ sessionId: sid }).catch(() => {});
      sessionIdRef.current = null;
    }
    stoppedRef.current = false;
    // A fresh token on every restart: retrying with the one that just died
    // rebuilds the same dead url.
    tokenRef.current = null;
    setIsError(false);
    setShedReason(null);
    setIsLoading(true);
    void startStream(audioIndex, subtitleIndex, maxHeight);
  }, [startStream]);

  useEffect(() => {
    const onHide = () => stopSession();
    window.addEventListener("pagehide", onHide);
    return () => {
      window.removeEventListener("pagehide", onHide);
      stopSession();
    };
  }, [stopSession]);

  return { streamUrl, sessionId, streamInfo, isLoading, isError, shedReason, progress, stopSession, restartWithTracks };
}
