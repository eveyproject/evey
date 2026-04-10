import { useState, useEffect, useRef, useCallback } from "react";
import { Dimensions } from "react-native";
import {
  playbackServiceGetStreamInfo,
  playbackServiceStartStream,
  playbackServiceStopStream,
  eveyFetch,
} from "@evey/api";
import type { V1StreamInfo } from "@evey/api";
import { getServerUrl } from "../lib/server-config";

const DEVICE_ID = "evey-tv";
const DEVICE_NAME = "Evey TV";

const VIDEO_CODECS = ["h264", "hevc", "vp9"];
const AUDIO_CODECS = ["aac", "mp3", "opus", "flac", "ac3", "eac3"];
const CONTAINERS = ["mp4", "hls"];

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
  const [isLoading, setIsLoading] = useState(true);
  const [isError, setIsError] = useState(false);
  const [transcodeOffset, setTranscodeOffset] = useState(0);
  const sessionIdRef = useRef<string | null>(null);
  const startedRef = useRef(false);
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
        const screenH = Math.round(screen.height * (screen.scale ?? 1));

        const info = await playbackServiceGetStreamInfo(mediaId, {
          supportedCodecs: VIDEO_CODECS,
          supportedAudioCodecs: AUDIO_CODECS,
          supportedContainers: CONTAINERS,
          maxWidth: Math.round(screen.width * (screen.scale ?? 1)),
          maxHeight: maxHeight && maxHeight > 0 ? Math.min(maxHeight, screenH) : screenH,
          audioStreamIndex,
          subtitleStreamIndex,
        });

        let token = tokenRef.current;
        if (!token) {
          token = await fetchStreamToken(mediaId);
          tokenRef.current = token;
        }
        const url = buildStreamUrl(serverUrl, mediaId, info, token, startSec);

        const startPosMs = startSec ? String(Math.floor(startSec * 1000)) : "0";
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
      } catch {
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
      setIsLoading(true);
      startStream(undefined, undefined, undefined, positionSec);
    },
    [startStream],
  );

  useEffect(() => {
    return () => stopSession();
  }, [stopSession]);

  return { streamUrl, sessionId, streamInfo, isLoading, isError, stopSession, restartWithTracks, restartAtPosition, transcodeOffset };
}
