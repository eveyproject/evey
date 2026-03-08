import { useCallback, useEffect, useRef, useState } from "react";
import { usePlaybackServiceGetWatchProgress, usePlaybackServiceStartStream, usePlaybackServiceStopStream, usePlaybackServiceGetStreamInfo, } from "@evey/api";
import { useAuthStore } from "@evey/store";
export function usePlaySession(mediaId) {
    const [sessionId, setSessionId] = useState(null);
    const sessionIdRef = useRef(null);
    const stopped = useRef(false);
    const token = useAuthStore.getState().accessToken;
    const { data: progress } = usePlaybackServiceGetWatchProgress(mediaId);
    const { data: rawStreamInfo } = usePlaybackServiceGetStreamInfo(mediaId);
    const streamInfo = rawStreamInfo
        ? {
            ...rawStreamInfo,
            directPlayUrl: token
                ? `/api/v1/media/${mediaId}/stream?token=${token}`
                : rawStreamInfo.directPlayUrl,
        }
        : undefined;
    const stopStream = usePlaybackServiceStopStream();
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
        if (!mediaId)
            return;
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
        if (stopped.current)
            return;
        stopped.current = true;
        const sid = sessionIdRef.current;
        if (sid) {
            stopStream.mutate({ data: { sessionId: sid } });
        }
    }, [stopStream]);
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
