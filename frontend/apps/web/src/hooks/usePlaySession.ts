import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  usePlaybackServiceGetWatchProgress,
  usePlaybackServiceStopStream,
  usePlaybackServiceGetStreamInfo,
  playbackServiceStartStream,
} from "@evey/api";
import type { V1StreamInfo, V1WatchProgress } from "@evey/api/src/generated/playback.schemas";
import { useAuthStore } from "@evey/store";
import { getBrowserCapabilities } from "@/utils/codec";

export interface PlaySessionResult {
  streamInfo: V1StreamInfo | null;
  sessionId: string | null;
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
  const token = useAuthStore.getState().accessToken;

  const caps = useMemo(() => getBrowserCapabilities(), []);
  const streamInfoParams = useMemo(() => ({
    supportedCodecs: caps.videoCodecs,
    supportedContainers: caps.containers,
    maxWidth: window.screen.width,
    maxHeight: window.screen.height,
  }), [caps]);

  const { data: progress } = usePlaybackServiceGetWatchProgress(mediaId, undefined, {
    query: { retry: false },
  });
  const { data: rawStreamInfo } = usePlaybackServiceGetStreamInfo(mediaId, streamInfoParams);

  const streamInfo: V1StreamInfo | undefined = rawStreamInfo
    ? (() => {
        const isDirectPlay =
          rawStreamInfo.playMethod === "PLAY_METHOD_DIRECT_PLAY" ||
          rawStreamInfo.playMethod === "PLAY_METHOD_DIRECT_STREAM";
        const url = isDirectPlay
          ? (token
              ? `${rawStreamInfo.directPlayUrl ?? `/api/v1/media/${mediaId}/stream`}?token=${token}`
              : rawStreamInfo.directPlayUrl)
          : (token
              ? `/api/v1/media/${mediaId}/transcode?token=${token}`
              : rawStreamInfo.transcodeUrl);
        return { ...rawStreamInfo, directPlayUrl: url };
      })()
    : undefined;

  const { mutate: stopStreamMutate } = usePlaybackServiceStopStream();

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

    playbackServiceStartStream({
      mediaItemId: mediaId,
      playMethod,
      deviceId: "web-browser",
      deviceName: "Web Browser",
      clientName: "Evey Web",
      startPositionMs: "0",
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
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mediaId]);

  const stopSession = useCallback(() => {
    if (stopped.current) return;
    stopped.current = true;
    const sid = sessionIdRef.current;
    if (sid) {
      stopStreamMutate({ data: { sessionId: sid } });
    }
  }, [stopStreamMutate]);

  useEffect(() => {
    return () => stopSession();
  }, [stopSession]);

  return {
    streamInfo: streamInfo ?? null,
    sessionId,
    isLoading: starting || (!rawStreamInfo && !startError),
    isError: startError,
    progress: progress ?? null,
    stopSession,
  };
}
