import { useState, useEffect, useRef, useCallback } from "react";
import { Dimensions } from "react-native";
import * as Network from "expo-network";
import {
  playbackServiceGetStreamInfo,
  playbackServiceStartStream,
  playbackServiceStopStream,
  eveyFetch,
} from "@evey/api";
import type { V1StreamInfo, V1WatchProgress } from "@evey/api";
import { getServerUrl } from "../lib/server-config";

const DEVICE_ID = "evey-mobile";
const DEVICE_NAME = "Evey Mobile";

const VIDEO_CODECS = ["h264", "hevc", "vp9"];
const AUDIO_CODECS = ["aac", "mp3", "opus", "flac", "ac3", "eac3"];
const CONTAINERS = ["mp4", "hls"];

// Probe the connection state via expo-network so the server can apply the
// per-profile max_bitrate_cellular_kbps cap when relevant. Anything that
// isn't explicitly cellular is reported as "wifi" — wired/ethernet/unknown
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

export interface PlaySessionResult {
  streamUrl: string | null;
  sessionId: string | null;
  streamInfo: V1StreamInfo | null;
  isLoading: boolean;
  isError: boolean;
  stopSession: () => void;
  restartWithTracks: (audioIndex?: number, subtitleIndex?: number, maxHeight?: number) => void;
  restartAtPosition: (positionSec: number) => void;
  transcodeOffset: number;
  progress: V1WatchProgress | null;
}

async function fetchStreamToken(mediaId: string): Promise<string> {
  const res = await eveyFetch<StreamTokenResponse>({
    url: `/api/v1/media/${mediaId}/stream-token`,
    method: "POST",
  });
  return res.token;
}

function appendCodecParams(params: URLSearchParams): void {
  params.set("videoCodecs", VIDEO_CODECS.join(","));
  params.set("audioCodecs", AUDIO_CODECS.join(","));
  params.set("containers", CONTAINERS.join(","));
}

function buildStreamUrl(
  serverUrl: string,
  mediaId: string,
  streamInfo: V1StreamInfo,
  token: string,
  startSec?: number,
): string {
  if (streamInfo.playMethod === "PLAY_METHOD_DIRECT_PLAY") {
    return `${serverUrl}/api/v1/media/${mediaId}/stream?token=${token}`;
  }
  if (streamInfo.playMethod === "PLAY_METHOD_DIRECT_STREAM") {
    const p = new URLSearchParams({ token, remux: "1" });
    appendCodecParams(p);
    return `${serverUrl}/api/v1/media/${mediaId}/transcode?${p.toString()}`;
  }
  const params = new URLSearchParams({ token });
  if (startSec && startSec > 0) params.set("startSec", String(Math.floor(startSec)));
  appendCodecParams(params);
  return `${serverUrl}/api/v1/media/${mediaId}/hls/master.m3u8?${params.toString()}`;
}

export function usePlaySession(mediaId: string): PlaySessionResult {
  const [streamUrl, setStreamUrl] = useState<string | null>(null);
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [streamInfo, setStreamInfo] = useState<V1StreamInfo | null>(null);
  const [progress, setProgress] = useState<V1WatchProgress | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isError, setIsError] = useState(false);
  const [transcodeOffset, setTranscodeOffset] = useState(0);
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
    async (audioStreamIndex?: number, subtitleStreamIndex?: number, maxHeight?: number, startSec?: number) => {
      try {
        const serverUrl = serverUrlRef.current || ((await getServerUrl()) ?? "");
        serverUrlRef.current = serverUrl;
        const screen = Dimensions.get("screen");
        // Video plays rotated to landscape, so the long screen edge ends up
        // along the video's width. Send (long, short) as (maxWidth, maxHeight)
        // — otherwise the server sees source.width=1920 > client.maxWidth=1080
        // (portrait short edge), triggers resolution_exceeded, and transcodes
        // a 1080p source down to 1080p again with quality loss.
        const w = Math.round(screen.width * (screen.scale ?? 1));
        const h = Math.round(screen.height * (screen.scale ?? 1));
        const longEdge = Math.max(w, h);
        const shortEdge = Math.min(w, h);
        const cappedMaxHeight = maxHeight && maxHeight > 0 ? Math.min(maxHeight, shortEdge) : shortEdge;
        const networkType = await detectNetworkType();

        const [info, wp] = await Promise.all([
          playbackServiceGetStreamInfo(mediaId, {
            supportedCodecs: VIDEO_CODECS,
            supportedAudioCodecs: AUDIO_CODECS,
            supportedContainers: CONTAINERS,
            maxWidth: longEdge,
            maxHeight: cappedMaxHeight,
            audioStreamIndex,
            subtitleStreamIndex,
            networkType,
          }),
          eveyFetch<V1WatchProgress>({
            url: `/api/v1/playback/${mediaId}/progress`,
            method: "GET",
          }).catch(() => null),
        ]);

        if (wp) setProgress(wp);

        let token = tokenRef.current;
        if (!token) {
          token = await fetchStreamToken(mediaId);
          tokenRef.current = token;
        }
        const url = buildStreamUrl(serverUrl, mediaId, info, token, startSec);

        const startPosMs = startSec ? String(Math.floor(startSec * 1000)) : (wp?.positionMs && !wp.played ? wp.positionMs : "0");
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
          setTranscodeOffset(startSec ?? 0);
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
      setIsLoading(true);
      startStream(audioIndex, subtitleIndex, maxHeight);
    },
    [startStream],
  );

  const restartAtPosition = useCallback(
    (positionSec: number) => {
      if (streamInfoRef.current?.playMethod === "PLAY_METHOD_DIRECT_PLAY") return;
      const sid = sessionIdRef.current;
      if (sid) {
        playbackServiceStopStream({ sessionId: sid }).catch((err) =>
          console.warn("stop before seek restart failed:", err),
        );
        sessionIdRef.current = null;
      }
      stoppedRef.current = false;
      setStreamUrl(null);
      setIsError(false);
      setIsLoading(true);
      startStream(undefined, undefined, undefined, positionSec);
    },
    [startStream],
  );

  useEffect(() => {
    return () => stopSession();
  }, [stopSession]);

  return { streamUrl, sessionId, streamInfo, isLoading, isError, stopSession, restartWithTracks, restartAtPosition, transcodeOffset, progress };
}
