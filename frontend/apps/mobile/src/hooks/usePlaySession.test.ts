import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

const eveyFetchMock = vi.fn();
const getStreamInfoMock = vi.fn();
const startStreamMock = vi.fn();
const stopStreamMock = vi.fn();

vi.mock("@evey/api", () => ({
  eveyFetch: (...args: unknown[]) => eveyFetchMock(...args),
  eveyFetchRaw: vi.fn(),
  playbackServiceGetStreamInfo: (...args: unknown[]) => getStreamInfoMock(...args),
  playbackServiceStartStream: (...args: unknown[]) => startStreamMock(...args),
  playbackServiceStopStream: (...args: unknown[]) => stopStreamMock(...args),
}));

vi.mock("../lib/server-config", () => ({
  getServerUrl: vi.fn().mockResolvedValue("http://localhost:8080"),
}));

const getAudioProbeMock = vi.fn();
// The hardware H.264 ceiling this fake device answers at module load.
const getMaxH264LevelMock = vi.fn(async () => 41);
vi.mock("@evey/media-caps", () => ({
  getAudioProbe: () => getAudioProbeMock(),
  getMaxH264Level: () => getMaxH264LevelMock(),
}));

// Minimal shape of the native audio probe; only the two fields the channel
// ceiling is derived from vary between cases.
function audioProbe(over: Record<string, unknown>) {
  return {
    apiLevel: 34,
    isTv: false,
    deviceModel: "test",
    decoders: [],
    directPlayback: {},
    directProfiles: [],
    hdmi: { available: false },
    surroundOutputRaw: -1,
    outputDeviceTypes: [],
    ...over,
  };
}

vi.mock("react-native", () => ({
  Dimensions: {
    get: () => ({ width: 360, height: 800, scale: 2 }),
  },
  Platform: { OS: "android" },
  // deviceCodecs reads NativeModules.MediaCaps at import time; absent = the
  // conservative baseline audio codec list.
  NativeModules: {},
}));

beforeEach(() => {
  eveyFetchMock.mockReset();
  getStreamInfoMock.mockReset();
  startStreamMock.mockReset();
  stopStreamMock.mockReset();
  getAudioProbeMock.mockReset();
  getAudioProbeMock.mockResolvedValue(null);
});

// The channel ceiling of the OUTPUT, which no playback request used to carry:
// the server profile pinned MaxAudioChannels = 6 and the device folded 5.1
// onto two speakers without normalisation - voices fine, music and effects
// clipping.
describe("maxAudioChannels reaches the stream URL", () => {
  const realFetch = global.fetch;
  const fetchMock = vi.fn();

  beforeEach(() => {
    global.fetch = fetchMock as unknown as typeof fetch;
    fetchMock.mockReset();
    fetchMock.mockResolvedValue({ status: 200, headers: { get: () => null }, json: async () => ({}) });
    getStreamInfoMock.mockResolvedValue({ playMethod: "PLAY_METHOD_TRANSCODE" });
    startStreamMock.mockResolvedValue({ id: "sess1" });
  });

  afterEach(() => {
    global.fetch = realFetch;
  });

  async function negotiatedUrl(): Promise<string> {
    const { renderHook, waitFor } = await import("@testing-library/react");
    const { usePlaySession } = await import("./usePlaySession");
    eveyFetchMock
      .mockResolvedValueOnce(null)
      .mockResolvedValueOnce({ token: "t1", mediaId: "m1" });
    const { result } = renderHook(() => usePlaySession("m1"));
    await waitFor(() => expect(result.current.streamUrl).not.toBeNull());
    return result.current.streamUrl as string;
  }

  it("declares the stereo ceiling of a phone (speaker + headphones + Bluetooth)", async () => {
    getAudioProbeMock.mockResolvedValue(audioProbe({ outputDeviceTypes: [2, 4, 8] }));
    expect(new URL(await negotiatedUrl()).searchParams.get("maxAudioChannels")).toBe("2");
  });

  it("leaves a plugged 5.1 HDMI sink at 5.1", async () => {
    getAudioProbeMock.mockResolvedValue(
      audioProbe({ hdmi: { available: true, plugState: 1, maxChannelCount: 6 }, outputDeviceTypes: [9] }),
    );
    expect(new URL(await negotiatedUrl()).searchParams.get("maxAudioChannels")).toBe("6");
  });

  it("omits the param when the device cannot answer, keeping the server default", async () => {
    getAudioProbeMock.mockResolvedValue(null);
    expect(new URL(await negotiatedUrl()).searchParams.has("maxAudioChannels")).toBe(false);
  });
});

describe("buildStreamUrl", () => {
  it("percent-encodes a deep-link traversal id so the stream URL cannot pivot to another API path", async () => {
    const { buildStreamUrl } = await import("./usePlaySession");
    const url = buildStreamUrl("http://srv", "../admin/api-keys", { playMethod: "PLAY_METHOD_DIRECT_PLAY" }, "tok");
    expect(url).toBe("http://srv/api/v1/media/..%2Fadmin%2Fapi-keys/stream?token=tok");
  });

  it("returns direct stream URL for PLAY_METHOD_DIRECT_PLAY (no codec params)", async () => {
    const { buildStreamUrl } = await import("./usePlaySession");
    const url = buildStreamUrl("http://srv", "m1", { playMethod: "PLAY_METHOD_DIRECT_PLAY" }, "tok");
    expect(url).toBe("http://srv/api/v1/media/m1/stream?token=tok");
  });

  // /transcode?remux=1 is a piped ffmpeg stdout: chunked, no Content-Length,
  // `Accept-Ranges: none`. A native player marks that source UNSEEKABLE, which
  // takes down every seek at once - resume-at-position (the episode restarts at
  // 0:00), dragging the timeline, and the double-tap skip. The HLS copy rung
  // serves the same copied bytes over a complete VOD manifest.
  it("routes PLAY_METHOD_DIRECT_STREAM through the seekable HLS master, never the unseekable remux pipe", async () => {
    const { buildStreamUrl } = await import("./usePlaySession");
    const url = buildStreamUrl("http://srv", "m1", { playMethod: "PLAY_METHOD_DIRECT_STREAM" }, "tok");
    expect(url).toMatch(/^http:\/\/srv\/api\/v1\/media\/m1\/hls\/master\.m3u8\?/);
    expect(url).not.toContain("remux=1");
    expect(url).toContain("token=tok");
    expect(url).toContain("videoCodecs=");
    expect(url).toContain("audioCodecs=");
    expect(url).toContain("containers=");
  });

  // The passthrough opt-in must reach the MASTER URL, not just the gRPC
  // stream-info call: the HLS handler builds its plan from these query params,
  // and without it the server re-encodes the MP3 the panel said it would copy
  // (prod 2026-09-06). Android only - see getFmp4AudioPassthrough.
  it("carries the fMP4 audio-passthrough opt-in on the HLS master", async () => {
    const { buildStreamUrl } = await import("./usePlaySession");
    const url = buildStreamUrl("http://srv", "m1", { playMethod: "PLAY_METHOD_TRANSCODE" }, "tok");
    expect(url).toContain("fmp4AudioPassthrough=mp3");
  });

  // A direct_stream that carries a hand-picked audio/subtitle track must reach
  // the manifest with those params, exactly like a transcode - the remux branch
  // used to build its own URL shape and this went untested.
  it("carries the picked audio and subtitle tracks on a DIRECT_STREAM manifest", async () => {
    const { buildStreamUrl } = await import("./usePlaySession");
    const url = buildStreamUrl("http://srv", "m1", { playMethod: "PLAY_METHOD_DIRECT_STREAM" }, "tok", { audioTrack: 1, subtitleTrack: 0 });
    expect(url).toContain("audioTrack=1");
    expect(url).toContain("subtitleTrack=0");
  });

  it("returns HLS URL for PLAY_METHOD_TRANSCODE with codec params", async () => {
    const { buildStreamUrl } = await import("./usePlaySession");
    const url = buildStreamUrl("http://srv", "m1", { playMethod: "PLAY_METHOD_TRANSCODE" }, "tok");
    expect(url).toMatch(/^http:\/\/srv\/api\/v1\/media\/m1\/hls\/master\.m3u8\?/);
    expect(url).toContain("token=tok");
    expect(url).toContain("videoCodecs=");
  });

  // The language-switch bug: the manifest's audio is chosen by ?audioTrack=
  // (0-based among audio streams, clampAudioTrack), and the native player only
  // reloads when the URL changes. Omitting the param rebuilt the SAME url for
  // every track, so the audio stayed on the default. These pin both the param
  // and that a different track yields a different url.
  it("carries the 0-based audioTrack/subtitleTrack in the HLS URL", async () => {
    const { buildStreamUrl } = await import("./usePlaySession");
    const url = buildStreamUrl("http://srv", "m1", { playMethod: "PLAY_METHOD_TRANSCODE" }, "tok", { audioTrack: 1, subtitleTrack: 0 });
    expect(url).toContain("audioTrack=1");
    expect(url).toContain("subtitleTrack=0");
  });

  it("produces a DIFFERENT URL per audio track so the player reloads on switch", async () => {
    const { buildStreamUrl } = await import("./usePlaySession");
    const info = { playMethod: "PLAY_METHOD_TRANSCODE" } as const;
    const track0 = buildStreamUrl("http://srv", "m1", info, "tok", { audioTrack: 0 });
    const track1 = buildStreamUrl("http://srv", "m1", info, "tok", { audioTrack: 1 });
    expect(track0).not.toBe(track1);
    // No track selected -> no param (server keeps its default).
    expect(buildStreamUrl("http://srv", "m1", info, "tok")).not.toContain("audioTrack=");
  });

  it("declares mkv/webm containers on Android so a compatible MKV direct-plays", async () => {
    const { buildStreamUrl } = await import("./usePlaySession");
    const url = buildStreamUrl("http://srv", "m1", { playMethod: "PLAY_METHOD_TRANSCODE" }, "tok");
    // Without mkv here the server flags container_incompatible and transcodes
    // every .mkv - the root cause of broken seeking + shifted colors.
    expect(decodeURIComponent(url)).toContain("containers=mp4,mkv,webm,hls");
  });

  // startSec is a pre-warm hint the server reads (transcode_start_hint.go):
  // the encoder opens at the resume position, the manifest still starts at 0.
  it("sends the resume position as a whole-second hint, and nothing from the start", async () => {
    const { buildStreamUrl } = await import("./usePlaySession");
    const info = { playMethod: "PLAY_METHOD_TRANSCODE" } as const;
    expect(buildStreamUrl("http://srv", "m1", info, "tok", { startSec: 42.7 })).toContain("startSec=42");
    expect(buildStreamUrl("http://srv", "m1", info, "tok")).not.toContain("startSec");
  });

  it("carries the hardware H.264 level ceiling the device probed at load", async () => {
    const { buildStreamUrl } = await import("./usePlaySession");
    // The probe is async at module load; let it land before reading the URL.
    await new Promise((r) => setTimeout(r, 0));
    const url = buildStreamUrl("http://srv", "m1", { playMethod: "PLAY_METHOD_TRANSCODE" }, "tok");
    expect(url).toContain("maxH264Level=41");
    expect(getMaxH264LevelMock).toHaveBeenCalled();
  });

  it("carries the probed channel ceiling, and nothing when there is none", async () => {
    const { buildStreamUrl } = await import("./usePlaySession");
    const info = { playMethod: "PLAY_METHOD_TRANSCODE" };
    expect(buildStreamUrl("http://srv", "m1", info, "tok", { maxAudioChannels: 2 }))
      .toContain("maxAudioChannels=2");
    expect(buildStreamUrl("http://srv", "m1", info, "tok", { maxAudioChannels: 6 }))
      .toContain("maxAudioChannels=6");
    expect(buildStreamUrl("http://srv", "m1", info, "tok")).not.toContain("maxAudioChannels");
  });

  // The quality menu used to reach GetStreamInfo alone: the verdict changed,
  // the master served the same ladder. The pick rides the master URL now.
  it("carries the picked quality as maxHeight, nothing on Auto", async () => {
    const { buildStreamUrl } = await import("./usePlaySession");
    const info = { playMethod: "PLAY_METHOD_TRANSCODE" };
    expect(buildStreamUrl("http://srv", "m1", info, "tok", { maxHeight: 720 })).toContain("maxHeight=720");
    expect(buildStreamUrl("http://srv", "m1", info, "tok", {})).not.toContain("maxHeight");
  });

  it("falls back to HLS URL for unknown play method", async () => {
    const { buildStreamUrl } = await import("./usePlaySession");
    const url = buildStreamUrl("http://srv", "m1", { playMethod: "WHATEVER" }, "tok");
    expect(url).toMatch(/^http:\/\/srv\/api\/v1\/media\/m1\/hls\/master\.m3u8\?/);
  });
});

describe("fetchStreamToken", () => {
  it("requests the long-lived native-purpose token and returns it", async () => {
    eveyFetchMock.mockResolvedValue({ token: "abc123", mediaId: "m1" });
    const { fetchStreamToken } = await import("./usePlaySession");

    const token = await fetchStreamToken("m1");

    expect(token).toBe("abc123");
    // Plan B P3: the native player can't re-mint mid-stream, so it must ask for
    // the 12h window (purpose=native), not the 5-minute default.
    expect(eveyFetchMock).toHaveBeenCalledWith({
      url: "/api/v1/media/m1/stream-token?purpose=native",
      method: "POST",
    });
  });
});

describe("usePlaySession integration", () => {
  it("calls getStreamInfo with supported codecs", async () => {
    const { renderHook } = await import("@testing-library/react");
    const { usePlaySession } = await import("./usePlaySession");

    getStreamInfoMock.mockResolvedValue({
      playMethod: "PLAY_METHOD_DIRECT_PLAY",
    });
    eveyFetchMock
      .mockResolvedValueOnce(null) // progress
      .mockResolvedValueOnce({ token: "t1", mediaId: "m1" }); // stream-token
    startStreamMock.mockResolvedValue({ id: "sess1" });

    const { result } = renderHook(() => usePlaySession("m1"));

    // Wait for loading to finish
    const { waitFor } = await import("@testing-library/react");
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(getStreamInfoMock).toHaveBeenCalledWith("m1", expect.objectContaining({
      supportedCodecs: ["h264", "hevc", "vp9"],
      // Baseline list: no MediaCaps native module in the test env, and never
      // ac3/eac3 without a probed Dolby decoder (silent direct-play otherwise).
      supportedAudioCodecs: ["aac", "mp3", "opus", "flac", "vorbis"],
      supportedContainers: ["mp4", "mkv", "webm", "hls"],
    }));
  });

  it("sets isError on failure", async () => {
    const { renderHook, waitFor } = await import("@testing-library/react");
    const { usePlaySession } = await import("./usePlaySession");

    getStreamInfoMock.mockRejectedValue(new Error("network"));
    eveyFetchMock.mockRejectedValue(new Error("network"));

    const { result } = renderHook(() => usePlaySession("m2"));

    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.isError).toBe(true);
  });
});

// Server-side load shed (HTTP 503): expo-video's native player only exposes a
// free-text error message, so usePlaySession probes the manifest URL itself
// before handing it to the player - this is the only place that can read the
// server's structured shed code.
describe("usePlaySession: server load-shed (503)", () => {
  const realFetch = global.fetch;
  const fetchMock = vi.fn();

  beforeEach(() => {
    global.fetch = fetchMock as unknown as typeof fetch;
    fetchMock.mockReset();
  });

  afterEach(() => {
    global.fetch = realFetch;
  });

  function mockHappyNegotiation() {
    getStreamInfoMock.mockResolvedValue({ playMethod: "PLAY_METHOD_TRANSCODE" });
    eveyFetchMock
      .mockResolvedValueOnce(null) // progress
      .mockResolvedValueOnce({ token: "t1", mediaId: "m1" }); // stream-token
  }

  it("reads the shed code off the X-Evey-Error header and does not start a play session", async () => {
    const { renderHook, waitFor } = await import("@testing-library/react");
    const { usePlaySession } = await import("./usePlaySession");

    mockHappyNegotiation();
    fetchMock.mockResolvedValue({
      status: 503,
      headers: { get: (name: string) => (name === "X-Evey-Error" ? "too_many_streams" : null) },
      json: async () => ({ code: "too_many_streams", retryAfterSec: 5, message: "shed: too many streams" }),
    });

    const { result } = renderHook(() => usePlaySession("m1"));
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.shedReason).toBe("too_many_streams");
    expect(result.current.isError).toBe(false);
    expect(result.current.streamUrl).toBeNull();
    // Never register a play session for a request the server refused.
    expect(startStreamMock).not.toHaveBeenCalled();
  });

  it("reads file_not_ready from the JSON body when the header is absent", async () => {
    const { renderHook, waitFor } = await import("@testing-library/react");
    const { usePlaySession } = await import("./usePlaySession");

    mockHappyNegotiation();
    fetchMock.mockResolvedValue({
      status: 503,
      headers: { get: () => null },
      json: async () => ({ code: "file_not_ready", retryAfterSec: 10, message: "shed: index building" }),
    });

    const { result } = renderHook(() => usePlaySession("m1"));
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.shedReason).toBe("file_not_ready");
  });

  it("defaults to too_many_streams when the 503 carries no usable header or body", async () => {
    const { renderHook, waitFor } = await import("@testing-library/react");
    const { usePlaySession } = await import("./usePlaySession");

    mockHappyNegotiation();
    fetchMock.mockResolvedValue({
      status: 503,
      headers: { get: () => null },
      json: async () => { throw new Error("not json"); },
    });

    const { result } = renderHook(() => usePlaySession("m1"));
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.shedReason).toBe("too_many_streams");
  });

  it("does not probe for PLAY_METHOD_DIRECT_PLAY (nothing to shed)", async () => {
    const { renderHook, waitFor } = await import("@testing-library/react");
    const { usePlaySession } = await import("./usePlaySession");

    getStreamInfoMock.mockResolvedValue({ playMethod: "PLAY_METHOD_DIRECT_PLAY" });
    eveyFetchMock
      .mockResolvedValueOnce(null)
      .mockResolvedValueOnce({ token: "t1", mediaId: "m1" });
    startStreamMock.mockResolvedValue({ id: "sess1" });

    const { result } = renderHook(() => usePlaySession("m1"));
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(fetchMock).not.toHaveBeenCalled();
    expect(result.current.shedReason).toBeNull();
    expect(result.current.streamUrl).not.toBeNull();
  });

  it("proceeds normally when the preflight succeeds (2xx)", async () => {
    const { renderHook, waitFor } = await import("@testing-library/react");
    const { usePlaySession } = await import("./usePlaySession");

    mockHappyNegotiation();
    startStreamMock.mockResolvedValue({ id: "sess1" });
    fetchMock.mockResolvedValue({ status: 200, headers: { get: () => null }, json: async () => ({}) });

    const { result } = renderHook(() => usePlaySession("m1"));
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.shedReason).toBeNull();
    expect(result.current.streamUrl).not.toBeNull();
    expect(startStreamMock).toHaveBeenCalled();
  });
});

// GET /media/{id}/transcode spawns one ffmpeg PER REQUEST and streams its
// stdout. The shed preflight used to probe that URL too, with a body it never
// consumed: every DIRECT_STREAM playback start left a phantom remux of the
// whole film running server-side - including when the player then chose a
// LOCAL downloaded file as its source. Only the HLS master (idempotent,
// spawn-free) is probeable.
describe("usePlaySession: shed preflight scope", () => {
  const realFetch = global.fetch;
  const fetchMock = vi.fn();

  beforeEach(() => {
    global.fetch = fetchMock as unknown as typeof fetch;
    fetchMock.mockReset();
  });

  afterEach(() => {
    global.fetch = realFetch;
  });

  it("never sends a DIRECT_STREAM playback to the one-ffmpeg-per-GET /transcode endpoint", async () => {
    const { renderHook, waitFor } = await import("@testing-library/react");
    const { usePlaySession } = await import("./usePlaySession");

    getStreamInfoMock.mockResolvedValue({ playMethod: "PLAY_METHOD_DIRECT_STREAM" });
    eveyFetchMock
      .mockResolvedValueOnce(null)
      .mockResolvedValueOnce({ token: "t1", mediaId: "m1" });
    startStreamMock.mockResolvedValue({ id: "sess1" });
    fetchMock.mockResolvedValue({ status: 200, headers: { get: () => null } });

    const { result } = renderHook(() => usePlaySession("m1"));
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.streamUrl).toContain("/hls/master.m3u8?");
    expect(result.current.streamUrl).not.toContain("/transcode?");
    // Now that direct_stream rides the master, it earns the shed preflight too -
    // and the probe still only ever touches the spawn-free manifest.
    for (const [url] of fetchMock.mock.calls) {
      expect(String(url)).toContain("/hls/master.m3u8");
    }
  });

  it("probes the HLS master with an abortable request (status wanted, never a body)", async () => {
    const { renderHook, waitFor } = await import("@testing-library/react");
    const { usePlaySession } = await import("./usePlaySession");

    getStreamInfoMock.mockResolvedValue({ playMethod: "PLAY_METHOD_TRANSCODE" });
    eveyFetchMock
      .mockResolvedValueOnce(null)
      .mockResolvedValueOnce({ token: "t1", mediaId: "m1" });
    startStreamMock.mockResolvedValue({ id: "sess1" });
    fetchMock.mockResolvedValue({ status: 200, headers: { get: () => null }, json: async () => ({}) });

    const { result } = renderHook(() => usePlaySession("m1"));
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining("master.m3u8"),
      expect.objectContaining({ signal: expect.anything() }),
    );
    expect(result.current.streamUrl).not.toBeNull();
  });
});

// Two taps on the track picker start two negotiations that both run to
// completion. Whichever ANSWERS last used to win, so when the first
// round-trip was the slower one the player silently went back to the track
// the user had just left - no error, no retry, just the wrong audio.
// The web player has guarded its equivalent rebuild for a while; this is the
// same guard on the mobile side.
describe("usePlaySession: concurrent track switches", () => {
  const realFetch = global.fetch;
  const fetchMock = vi.fn();

  beforeEach(() => {
    global.fetch = fetchMock as unknown as typeof fetch;
    fetchMock.mockReset();
    fetchMock.mockResolvedValue({ status: 200, headers: { get: () => null }, json: async () => ({}) });
    stopStreamMock.mockResolvedValue(undefined);
  });
  afterEach(() => {
    global.fetch = realFetch;
  });

  it("keeps the LAST requested track when an earlier negotiation answers after it", async () => {
    const { renderHook, waitFor } = await import("@testing-library/react");
    const { usePlaySession } = await import("./usePlaySession");

    // Two audio tracks: absolute index 1 is the first (relative 0), absolute
    // index 2 the second (relative 1) - the URL carries the RELATIVE one.
    const streams = [
      { type: "MEDIA_STREAM_TYPE_AUDIO", index: 1 },
      { type: "MEDIA_STREAM_TYPE_AUDIO", index: 2 },
    ];
    // Track A resolves slowly, track B quickly: B is asked for second but
    // answers first, so A's late answer must be discarded.
    let resolveSlow: ((v: unknown) => void) | null = null;
    getStreamInfoMock
      .mockResolvedValueOnce({ playMethod: "PLAY_METHOD_TRANSCODE", mediaStreams: streams }) // mount
      .mockImplementationOnce(() => new Promise((res) => { resolveSlow = res; }))            // A (slow)
      .mockResolvedValueOnce({ playMethod: "PLAY_METHOD_TRANSCODE", mediaStreams: streams }); // B (fast)
    eveyFetchMock.mockResolvedValue({ token: "t1", mediaId: "m1" });
    startStreamMock.mockImplementation(async () => ({ id: `sess-${startStreamMock.mock.calls.length}` }));

    const { result } = renderHook(() => usePlaySession("m1"));
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    result.current.restartWithTracks(1); // A (relative 0) - will stall
    result.current.restartWithTracks(2); // B (relative 1) - overtakes it
    await waitFor(() => expect(result.current.streamUrl).toContain("audioTrack=1"));

    // A finally answers, late. It must not overwrite B.
    resolveSlow?.({ playMethod: "PLAY_METHOD_TRANSCODE", mediaStreams: streams });
    await new Promise((r) => setTimeout(r, 30));

    expect(result.current.streamUrl).toContain("audioTrack=1");
    expect(result.current.streamUrl).not.toContain("audioTrack=0");
    expect(result.current.isError).toBe(false);
  });
});

// The shared hook reports an expired session apart (isAuthError); the phone's
// player has no screen for it, so it must keep showing the stream error.
describe("usePlaySession: expired session on the phone", () => {
  it("still raises isError when the fetcher gives up with a 401", async () => {
    const { renderHook, waitFor } = await import("@testing-library/react");
    const { usePlaySession } = await import("./usePlaySession");

    getStreamInfoMock.mockRejectedValue(Object.assign(new Error("unauthorized"), { httpStatus: 401 }));
    eveyFetchMock.mockResolvedValue(null);

    const { result } = renderHook(() => usePlaySession("m1"));
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.isError).toBe(true);
    expect(result.current.isAuthError).toBe(true);
  });
});
