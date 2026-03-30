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
  restartWithTracks: (audioIndex?: number, subtitleIndex?: number) => void;
}

async function fetchStreamToken(mediaId: string): Promise<string> {
  const res = await eveyFetch<StreamTokenResponse>({
    url: `/api/v1/media/${mediaId}/stream-token`,
    method: "POST",
  });
  return res.token;
}

function buildStreamUrl(
  serverUrl: string,
  mediaId: string,
  streamInfo: V1StreamInfo,
  token: string,
): string {
  if (streamInfo.playMethod === "PLAY_METHOD_DIRECT_PLAY") {
    return `${serverUrl}/api/v1/media/${mediaId}/stream?token=${token}`;
  }
  if (streamInfo.playMethod === "PLAY_METHOD_DIRECT_STREAM") {
    return `${serverUrl}/api/v1/media/${mediaId}/transcode?token=${token}&remux=1`;
  }
  return `${serverUrl}/api/v1/media/${mediaId}/hls/master.m3u8?token=${token}`;
}

export function usePlaySession(mediaId: string): PlaySessionResult {
  const [streamUrl, setStreamUrl] = useState<string | null>(null);
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [streamInfo, setStreamInfo] = useState<V1StreamInfo | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isError, setIsError] = useState(false);
  const sessionIdRef = useRef<string | null>(null);
  const startedRef = useRef(false);
  const stoppedRef = useRef(false);
  const tokenRef = useRef<string | null>(null);
  const serverUrlRef = useRef<string>("");

  const startStream = useCallback(
    async (audioStreamIndex?: number, subtitleStreamIndex?: number) => {
      try {
        const serverUrl = serverUrlRef.current || ((await getServerUrl()) ?? "");
        serverUrlRef.current = serverUrl;
        const screen = Dimensions.get("screen");

        const info = await playbackServiceGetStreamInfo(mediaId, {
          supportedCodecs: ["h264", "hevc", "aac"],
          supportedContainers: ["mp4", "hls"],
          maxWidth: Math.round(screen.width * (screen.scale ?? 1)),
          maxHeight: Math.round(screen.height * (screen.scale ?? 1)),
          audioStreamIndex,
          subtitleStreamIndex,
        });

        let token = tokenRef.current;
        if (!token) {
          token = await fetchStreamToken(mediaId);
          tokenRef.current = token;
        }
        const url = buildStreamUrl(serverUrl, mediaId, info, token);

        const session = await playbackServiceStartStream({
          mediaItemId: mediaId,
          playMethod: info.playMethod ?? "PLAY_METHOD_TRANSCODE",
          deviceId: "evey-mobile",
          deviceName: "Evey Mobile",
          clientName: "Evey Mobile",
          startPositionMs: "0",
          audioStreamIndex,
          subtitleStreamIndex,
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
      playbackServiceStopStream({ sessionId: sid }).catch(() => {});
    }
  }, []);

  const restartWithTracks = useCallback(
    (audioIndex?: number, subtitleIndex?: number) => {
      const sid = sessionIdRef.current;
      if (sid) {
        playbackServiceStopStream({ sessionId: sid }).catch(() => {});
        sessionIdRef.current = null;
      }
      stoppedRef.current = false;
      setIsLoading(true);
      startStream(audioIndex, subtitleIndex);
    },
    [startStream],
  );

  useEffect(() => {
    return () => stopSession();
  }, [stopSession]);

  return { streamUrl, sessionId, streamInfo, isLoading, isError, stopSession, restartWithTracks };
}
