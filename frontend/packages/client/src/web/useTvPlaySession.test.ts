import { renderHook, cleanup, act, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

const getStreamInfo = vi.fn();
const startStream = vi.fn();
const stopStream = vi.fn(() => Promise.resolve({}));
const eveyFetch = vi.fn();

vi.mock("@evey/api", () => ({
  getBaseURL: () => "http://tv.test",
  playbackServiceGetStreamInfo: (...a: unknown[]) => getStreamInfo(...a),
  playbackServiceStartStream: (...a: unknown[]) => startStream(...a),
  playbackServiceStopStream: (...a: unknown[]) => stopStream(...a),
  eveyFetch: (...a: unknown[]) => eveyFetch(...a),
}));
vi.mock("./codec", () => ({
  getBrowserCapabilities: () => ({ videoCodecs: ["h264"], audioCodecs: ["aac"], containers: ["mp4"], hdr: false }),
}));
vi.mock("./hlsCodecParams", () => ({
  appendCodecParams: (p: URLSearchParams) => p.set("videoCodecs", "h264"),
}));

import { useTvPlaySession, buildTvStreamUrl } from "./useTvPlaySession";

const DEVICE = { deviceId: "tv-browser", deviceName: "TV", clientName: "Evey TV", wasmAudio: false };
const INFO = {
  playMethod: "PLAY_METHOD_TRANSCODE",
  mediaStreams: [
    { type: "MEDIA_STREAM_TYPE_VIDEO", index: 0 },
    { type: "MEDIA_STREAM_TYPE_AUDIO", index: 1, language: "eng" },
    { type: "MEDIA_STREAM_TYPE_AUDIO", index: 2, language: "fra" },
    { type: "MEDIA_STREAM_TYPE_SUBTITLE", index: 3 },
    { type: "MEDIA_STREAM_TYPE_SUBTITLE", index: 4, codec: "dvd_subtitle" },
  ],
};

beforeEach(() => {
  getStreamInfo.mockReset().mockResolvedValue(INFO);
  startStream.mockReset().mockResolvedValue({ id: "sess-1" });
  stopStream.mockClear();
  eveyFetch.mockReset().mockImplementation(({ url }: { url: string }) =>
    url.includes("stream-token") ? Promise.resolve({ token: "tok-1" }) : Promise.resolve({ positionMs: "0" }),
  );
  vi.stubGlobal("fetch", vi.fn(() => Promise.resolve({ status: 200, headers: new Headers() } as Response)));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

describe("useTvPlaySession", () => {
  it("negotiates once, opens the HLS master and starts the server session", async () => {
    const { result } = renderHook(() => useTvPlaySession("m1", DEVICE));
    await waitFor(() => expect(result.current.streamUrl).not.toBeNull());
    expect(result.current.streamUrl).toContain("/api/v1/media/m1/hls/master.m3u8?token=tok-1");
    expect(result.current.sessionId).toBe("sess-1");
    expect(getStreamInfo).toHaveBeenCalledTimes(1);
    expect(startStream).toHaveBeenCalledWith(expect.objectContaining({ deviceId: "tv-browser", playMethod: "PLAY_METHOD_TRANSCODE" }));
  });

  // Seen on the dev server, 2026-09-24: under StrictMode the effects run,
  // clean up and run again; the cleanup marked the session stopped and the
  // second run never restarted it, so the answer was dropped and the player
  // stayed black with no stream.
  it("keeps its negotiation through StrictMode's mount, unmount and remount", async () => {
    const { result } = renderHook(() => useTvPlaySession("m1", DEVICE), { reactStrictMode: true });
    await waitFor(() => expect(result.current.streamUrl).not.toBeNull());
    expect(result.current.streamInfo).not.toBeNull();
  });

  // The viewer picks the French audio and a 480p ceiling: the running session
  // stops, a fresh token is minted, and the new master carries the type-relative
  // audio ordinal (the second audio track is audioTrack=1) and the ceiling.
  it("restartWithTracks stops the session and negotiates again with the picks", async () => {
    const { result } = renderHook(() => useTvPlaySession("m1", DEVICE));
    await waitFor(() => expect(result.current.sessionId).toBe("sess-1"));
    startStream.mockResolvedValue({ id: "sess-2" });
    act(() => result.current.restartWithTracks(2, undefined, 480));
    await waitFor(() => expect(result.current.sessionId).toBe("sess-2"));
    expect(stopStream).toHaveBeenCalledWith({ sessionId: "sess-1" });
    expect(getStreamInfo).toHaveBeenLastCalledWith("m1", expect.objectContaining({ audioStreamIndex: 2, maxHeight: 480 }));
    expect(result.current.streamUrl).toContain("audioTrack=1");
    expect(result.current.streamUrl).toContain("maxHeight=480");
    expect(eveyFetch.mock.calls.filter((c) => (c[0] as { url: string }).url.includes("stream-token")).length).toBe(2);
  });

  // The orchestrator answers 503 with an Evey code: a reason for the viewer
  // and a retry, not a broken title. No server session is opened meanwhile.
  it("surfaces a shed reason instead of a stream when the master answers 503", async () => {
    vi.stubGlobal("fetch", vi.fn(() => Promise.resolve({ status: 503, headers: new Headers({ "X-Evey-Error": "too_many_streams" }), json: async () => ({}) } as unknown as Response)));
    const { result } = renderHook(() => useTvPlaySession("m1", DEVICE));
    await waitFor(() => expect(result.current.shedReason).toBe("too_many_streams"));
    expect(result.current.streamUrl).toBeNull();
    expect(startStream).not.toHaveBeenCalled();
  });

  it("never probes direct play for shedding - the file streams with no orchestrator", async () => {
    getStreamInfo.mockResolvedValue({ playMethod: "PLAY_METHOD_DIRECT_PLAY", directPlayUrl: "/api/v1/media/m1/stream", mediaStreams: [] });
    const { result } = renderHook(() => useTvPlaySession("m1", DEVICE));
    await waitFor(() => expect(result.current.streamUrl).not.toBeNull());
    expect(result.current.streamUrl).toBe("http://tv.test/api/v1/media/m1/stream?token=tok-1");
    expect(fetch).not.toHaveBeenCalled();
  });
});

describe("buildTvStreamUrl", () => {
  it("carries the subtitle burn-in ordinal only when asked, and the resume hint", () => {
    const url = buildTvStreamUrl("m1", INFO as never, "t", { subtitleTrack: 1, startSec: 61.9 });
    expect(url).toContain("subtitleTrack=1");
    expect(url).toContain("startSec=61");
    expect(buildTvStreamUrl("m1", INFO as never, "t", {})).not.toContain("subtitleTrack");
  });
});
