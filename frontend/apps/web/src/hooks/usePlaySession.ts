import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  usePlaybackServiceGetWatchProgress,
  usePlaybackServiceStopStream,
  usePlaybackServiceGetStreamInfo,
  playbackServiceStartStream,
} from "@evey/api";
import type { V1StreamInfo, V1WatchProgress } from "@evey/api";
import { usePlayerStore } from "@evey/store";
import { getBrowserCapabilities } from "@/utils/codec";
import { useStreamToken } from "./useStreamToken";

export interface PlaySessionResult {
  streamInfo: V1StreamInfo | null;
  sessionId: string | null;
  streamToken: string | null;
  fetchStreamToken: (mediaId: string) => Promise<string>;
  isLoading: boolean;
  isError: boolean;
  progress: V1WatchProgress | null;
  stopSession: () => void;
}

export function usePlaySession(mediaId: string): PlaySessionResult {
  const [sessionId, setSessionId] = useState<string | null>(null);
  const sessionIdRef = useRef<string | null>(null);
  const stopped = useRef(false);
  const startedRef = useRef(false);
  const [starting, setStarting] = useState(false);
  const [startError, setStartError] = useState(false);
  const { streamToken, fetchStreamToken } = useStreamToken();
  const streamTokenRef = useRef<string | null>(null);

  const caps = useMemo(() => getBrowserCapabilities(), []);
  const streamInfoParams = useMemo(() => ({
    supportedCodecs: caps.videoCodecs,
    supportedAudioCodecs: caps.audioCodecs,
    supportedContainers: caps.containers,
    maxWidth: window.screen.width,
    maxHeight: window.screen.height,
  }), [caps]);

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
        } else if (rawStreamInfo.playMethod === "PLAY_METHOD_DIRECT_STREAM") {
          url = currentStreamToken
            ? `/api/v1/media/${mediaId}/transcode?token=${currentStreamToken}&remux=1`
            : rawStreamInfo.transcodeUrl;
        } else {
          url = currentStreamToken
            ? `/api/v1/media/${mediaId}/hls/master.m3u8?token=${currentStreamToken}`
            : rawStreamInfo.transcodeUrl;
        }
        return { ...rawStreamInfo, directPlayUrl: url };
      })()
    : undefined;

  const { mutate: stopStreamMutate } = usePlaybackServiceStopStream();
  const stopStreamRef = useRef(stopStreamMutate);
  stopStreamRef.current = stopStreamMutate;

  useEffect(() => {
    stopped.current = false;
    if (!mediaId || startedRef.current) return;
    startedRef.current = true;
    setStarting(true);

    const playMethod = rawStreamInfo?.playMethod === "PLAY_METHOD_DIRECT_PLAY"
      ? "PLAY_METHOD_DIRECT_PLAY"
      : rawStreamInfo?.playMethod === "PLAY_METHOD_DIRECT_STREAM"
        ? "PLAY_METHOD_DIRECT_STREAM"
        : "PLAY_METHOD_TRANSCODE";

    fetchStreamToken(mediaId).then((token) => {
      streamTokenRef.current = token;
      return playbackServiceStartStream({
        mediaItemId: mediaId,
        playMethod,
        deviceId: "web-browser",
        deviceName: "Web Browser",
        clientName: "Evey Web",
        startPositionMs: "0",
      });
    }).then((session) => {
      if (session.id) {
        sessionIdRef.current = session.id;
        setSessionId(session.id);
      }
    }).catch(() => {
      setStartError(true);
    }).finally(() => {
      setStarting(false);
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps -- fire-once per mediaId via startedRef guard; rawStreamInfo may be undefined on first run (defaults to TRANSCODE, server resolves actual method)
  }, [mediaId]);

  const stopSession = useCallback(() => {
    if (stopped.current) return;
    stopped.current = true;
    const sid = sessionIdRef.current;
    if (sid) {
      stopStreamRef.current({ data: { sessionId: sid } });
    }
  }, []);

  useEffect(() => {
    return () => {
      const mode = usePlayerStore.getState().mode;
      if (mode !== "mini") stopSession();
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
