import { useCallback, useEffect, useRef, useState } from "react";
import {
  usePlaybackServiceGetWatchProgress,
  usePlaybackServiceStopStream,
  usePlaybackServiceGetStreamInfo,
  playbackServiceStartStream,
} from "@evey/api";
import type { V1StreamInfo, V1WatchProgress } from "@evey/api/src/generated/playback.schemas";
import { useAuthStore } from "@evey/store";
import { canPlayHevc } from "@/utils/codec";

export interface PlaySessionResult {
  streamInfo: V1StreamInfo | null;
  sessionId: string | null;
  isLoading: boolean;
  progress: V1WatchProgress | null;
  stopSession: () => void;
}

export function usePlaySession(mediaId: string): PlaySessionResult {
  const [sessionId, setSessionId] = useState<string | null>(null);
  const sessionIdRef = useRef<string | null>(null);
  const stopped = useRef(false);
  const startedRef = useRef(false);
  const [starting, setStarting] = useState(false);
  const token = useAuthStore.getState().accessToken;

  const { data: progress } = usePlaybackServiceGetWatchProgress(mediaId, undefined, {
    query: { retry: false },
  });
  const { data: rawStreamInfo } = usePlaybackServiceGetStreamInfo(mediaId);

  const streamInfo: V1StreamInfo | undefined = rawStreamInfo
    ? {
        ...rawStreamInfo,
        directPlayUrl: token
          ? `/api/v1/media/${mediaId}/transcode?token=${token}${canPlayHevc ? "&remux=1" : ""}`
          : rawStreamInfo.directPlayUrl,
      }
    : undefined;

  const { mutate: stopStreamMutate } = usePlaybackServiceStopStream();

  useEffect(() => {
    stopped.current = false;
    if (!mediaId || startedRef.current) return;
    startedRef.current = true;
    setStarting(true);

    playbackServiceStartStream({
      mediaItemId: mediaId,
      playMethod: "PLAY_METHOD_TRANSCODE",
      deviceId: "web-browser",
      deviceName: "Web Browser",
      clientName: "Evey Web",
      startPositionMs: "0",
    }).then((session) => {
      if (session.id) {
        sessionIdRef.current = session.id;
        setSessionId(session.id);
      }
    }).catch((err) => {
      console.error("Failed to start stream:", err);
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
    isLoading: starting || !rawStreamInfo,
    progress: progress ?? null,
    stopSession,
  };
}
