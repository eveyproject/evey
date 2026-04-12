import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  usePlaybackServiceGetWatchProgress,
  usePlaybackServiceStopStream,
  usePlaybackServiceGetStreamInfo,
  playbackServiceStartStream,
} from "@evey/api";
import type { V1StreamInfo, V1WatchProgress } from "@evey/api";
import { usePlayerStore, useAuthStore } from "@evey/store";
import { getBrowserCapabilities } from "@/utils/codec";
import { appendCodecParams } from "@/features/playback/hooks/hlsPlayerUtils";
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
          if (currentStreamToken) {
            const p = new URLSearchParams({ token: currentStreamToken, remux: "1" });
            appendCodecParams(p);
            url = `/api/v1/media/${mediaId}/transcode?${p.toString()}`;
          } else {
            url = rawStreamInfo.transcodeUrl;
          }
        } else {
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

  const { mutate: stopStreamMutate } = usePlaybackServiceStopStream();
  const stopStreamRef = useRef(stopStreamMutate);
  stopStreamRef.current = stopStreamMutate;

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
        const token = await fetchStreamToken(mediaId);
        streamTokenRef.current = token;
        const session = await playbackServiceStartStream({
          mediaItemId: mediaId,
          playMethod,
          deviceId: "web-browser",
          deviceName: "Web Browser",
          clientName: "Evey Web",
          startPositionMs: "0",
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
    if (sid) {
      stopStreamRef.current({ data: { sessionId: sid } });
    }
  }, []);

  useEffect(() => {
    const handleUnload = () => {
      const sid = sessionIdRef.current;
      if (sid && !stopped.current) {
        stopped.current = true;
        // NOTE: raw fetch (not eveyFetch) because keepalive:true is required for
        // pagehide/beforeunload — eveyFetch doesn't propagate keepalive, and its
        // auto-refresh path is useless here (page is unloading). Token is already
        // fresh from the active session; if it 401s, we silently drop (expected).
        const token = useAuthStore.getState().accessToken;
        const headers: Record<string, string> = { "Content-Type": "application/json" };
        if (token) headers["Authorization"] = `Bearer ${token}`;
        const csrfMatch = typeof document !== "undefined"
          ? document.cookie.match(/(?:^|;\s*)_csrf=([^;]*)/)
          : null;
        if (csrfMatch) headers["X-CSRF-Token"] = csrfMatch[1];
        fetch("/api/v1/playback/stop", {
          method: "POST",
          credentials: "include",
          headers,
          body: JSON.stringify({ sessionId: sid }),
          keepalive: true,
        }).catch((err) => console.warn("playback stop failed", err));
      }
    };
    window.addEventListener("pagehide", handleUnload);
    return () => {
      window.removeEventListener("pagehide", handleUnload);
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
