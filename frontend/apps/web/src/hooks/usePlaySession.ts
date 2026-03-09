import { useCallback, useEffect, useRef, useState } from "react";
import {
  usePlaybackServiceGetWatchProgress,
  usePlaybackServiceStartStream,
  usePlaybackServiceStopStream,
  usePlaybackServiceGetStreamInfo,
} from "@evey/api";
import type { V1StreamInfo, V1WatchProgress } from "@evey/api/src/generated/playback.schemas";
import { useAuthStore } from "@evey/store";

const canPlayHevc = (() => {
  const v = document.createElement("video");
  return !!(v.canPlayType('video/mp4; codecs="hvc1"') || v.canPlayType('video/mp4; codecs="hev1"'));
})();

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
  const startStream = usePlaybackServiceStartStream({
    mutation: {
      onSuccess: (session) => {
        if (session.id) {
          sessionIdRef.current = session.id;
          setSessionId(session.id);
        }
      },
    },
  });

  useEffect(() => {
    if (!mediaId) return;
    stopped.current = false;
    startStream.mutate({
      data: {
        mediaItemId: mediaId,
        playMethod: "PLAY_METHOD_TRANSCODE",
        deviceId: "web-browser",
        deviceName: "Web Browser",
        clientName: "Evey Web",
        startPositionMs: "0",
      },
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
    isLoading: startStream.isPending || !rawStreamInfo,
    progress: progress ?? null,
    stopSession,
  };
}
