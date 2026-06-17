import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  usePlaybackServiceGetWatchProgress,
  usePlaybackServiceGetStreamInfo,
  playbackServiceStartStream,
} from "@evey/api";
import type { V1StreamInfo, V1WatchProgress } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { getBrowserCapabilities } from "@/utils/codec";
import { appendCodecParams } from "@/features/playback/hooks/hlsPlayerUtils";
import { useStreamToken } from "./useStreamToken";

// postStopStream tears down the server-side play session with a raw keepalive
// fetch (not eveyFetch / react-query). It must fire reliably during teardown:
// page unload (pagehide), route unmount, AND from the mini-player after the
// player route has already unmounted — a react-query mutation bound to an
// unmounted component is not a dependable transport here, which is exactly why
// closing the player via the browser back button (player -> mini -> close) used
// to leave the transcode session running in the admin panel. The active
// session token is fresh; a 401 is silently dropped (the session is gone).
function postStopStream(sessionId: string) {
  const token = useAuthStore.getState().accessToken;
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  if (token) headers["Authorization"] = `Bearer ${token}`;
  const csrfMatch = typeof document !== "undefined"
    ? document.cookie.match(/(?:^|;\s*)_csrf=([^;]*)/)
    : null;
  if (csrfMatch) headers["X-CSRF-Token"] = csrfMatch[1];
  void fetch("/api/v1/playback/stop", {
    method: "POST",
    credentials: "include",
    headers,
    body: JSON.stringify({ sessionId }),
    keepalive: true,
  }).catch((err) => console.warn("playback stop failed", err));
}

export interface PlaySessionResult {
  streamInfo: V1StreamInfo | null;
  sessionId: string | null;
  streamToken: string | null;
  fetchStreamToken: (mediaId: string, opts?: { cast?: boolean }) => Promise<string>;
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
  // No maxWidth/maxHeight: the screen size is NOT a decode capability. Sending
  // window.screen dims made the server flag resolution_exceeded and transcode
  // a 4K title for a 1366px laptop that decodes it natively (Jellyfin
  // direct-plays there). The browser downscales for display at zero cost;
  // bandwidth stays governed by the profile's bitrate prefs, and the transcode
  // fallback remains codec/HDR-driven.
  const streamInfoParams = useMemo(() => ({
    supportedCodecs: caps.videoCodecs,
    supportedAudioCodecs: caps.audioCodecs,
    supportedContainers: caps.containers,
    supportsHdr: caps.hdr,
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
          // Report the bitrate we'll actually pull (from the StreamInfo we just
          // resolved) so the admin dashboard shows the served rate, not source.
          servedBitrateBps: rawStreamInfo.bitrate ?? "0",
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
    if (sid) postStopStream(sid);
  }, []);

  useEffect(() => {
    const handleUnload = () => {
      const sid = sessionIdRef.current;
      if (sid && !stopped.current) {
        stopped.current = true;
        postStopStream(sid);
      }
    };
    window.addEventListener("pagehide", handleUnload);
    return () => {
      window.removeEventListener("pagehide", handleUnload);
      // Leaving the player (back button, route change, tab close) always ends
      // the session — there is no mini-player handoff that keeps it alive.
      stopSession();
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
