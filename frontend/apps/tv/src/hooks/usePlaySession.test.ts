import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

const eveyFetchMock = vi.fn();
const getStreamInfoMock = vi.fn();
const startStreamMock = vi.fn();
const stopStreamMock = vi.fn();

vi.mock("@evey/api", () => ({
  eveyFetch: (...args: unknown[]) => eveyFetchMock(...args),
  playbackServiceGetStreamInfo: (...args: unknown[]) => getStreamInfoMock(...args),
  playbackServiceStartStream: (...args: unknown[]) => startStreamMock(...args),
  playbackServiceStopStream: (...args: unknown[]) => stopStreamMock(...args),
}));

vi.mock("../lib/server-config", () => ({
  getServerUrl: vi.fn().mockResolvedValue("http://localhost:8080"),
}));

const getAudioProbeMock = vi.fn();
// The hardware H.264 ceiling this fake box answers at module load.
const getMaxH264LevelMock = vi.fn(async () => 42);
// HDR formats this fake panel renders. Null = the module isn't in the build,
// which is the shape a box without the probe answers with.
const getDisplayHdrTypesMock = vi.fn<() => Promise<string[] | null>>(async () => null);
vi.mock("@evey/media-caps", async () => {
  // The declaration RULE is not mocked: it is the thing under test on the TV
  // side too, and a hand-rolled copy here would let the two drift.
  const real = await vi.importActual<typeof import("@evey/media-caps")>("@evey/media-caps");
  return {
    getAudioProbe: () => getAudioProbeMock(),
    getMaxH264Level: () => getMaxH264LevelMock(),
    getDisplayHdrTypes: () => getDisplayHdrTypesMock(),
    displaySupportsHdr: real.displaySupportsHdr,
    declarableAudioCodecs: real.declarableAudioCodecs,
    // No ac3go in these cases: the module answers nothing.
    getPlayerAudioDecoders: async () => [],
    checkPlaybackDecoder: async () => null,
  };
});

// Minimal shape of the native audio probe; only the two fields the channel
// ceiling is derived from vary between cases.
function audioProbe(over: Record<string, unknown>) {
  return {
    apiLevel: 34,
    isTv: true,
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

beforeEach(() => {
  eveyFetchMock.mockReset();
  getStreamInfoMock.mockReset();
  startStreamMock.mockReset();
  stopStreamMock.mockReset();
  getAudioProbeMock.mockReset();
  getAudioProbeMock.mockResolvedValue(null);
});

// The channel ceiling of the OUTPUT, which no playback request used to carry:
// the server profile pinned MaxAudioChannels = 6 and the box folded 5.1 onto
// a 2.0 television without normalisation - voices fine, music and effects
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

  it("declares the stereo ceiling of a 2.0 television", async () => {
    getAudioProbeMock.mockResolvedValue(
      audioProbe({ hdmi: { available: true, plugState: 1, maxChannelCount: 2 }, outputDeviceTypes: [9] }),
    );
    expect(new URL(await negotiatedUrl()).searchParams.get("maxAudioChannels")).toBe("2");
  });

  it("leaves a 5.1 amplifier at 5.1", async () => {
    getAudioProbeMock.mockResolvedValue(
      audioProbe({ hdmi: { available: true, plugState: 1, maxChannelCount: 6 }, outputDeviceTypes: [10] }),
    );
    expect(new URL(await negotiatedUrl()).searchParams.get("maxAudioChannels")).toBe("6");
  });

  it("omits the param when the box cannot answer, keeping the server default", async () => {
    getAudioProbeMock.mockResolvedValue(null);
    expect(new URL(await negotiatedUrl()).searchParams.has("maxAudioChannels")).toBe(false);
  });

  // The HDR twin of the channel ceiling: a RESTITUTION capability, probed at
  // every negotiation and carried on the same URL. GetStreamInfo must see the
  // same answer as the master, or the stream panel describes a playback the
  // server never serves - which is exactly the divergence that made
  // /playback-info lie about the channel ceiling.
  it("declares hdr=1 for an HDR panel, on the master URL and to GetStreamInfo", async () => {
    getAudioProbeMock.mockResolvedValue(null);
    getDisplayHdrTypesMock.mockResolvedValue(["hdr10", "hlg"]);
    expect(new URL(await negotiatedUrl()).searchParams.get("hdr")).toBe("1");
    expect(getStreamInfoMock).toHaveBeenLastCalledWith("m1", expect.objectContaining({ supportsHdr: true }));
  });

  // His living room: an Android TV box on an old SDR panel, supportedHdrTypes=[] on every
  // mode. The tone-map must stay, and the URL must not grow a param.
  it("stays SDR for an SDR panel and for a box that cannot answer", async () => {
    getAudioProbeMock.mockResolvedValue(null);
    getDisplayHdrTypesMock.mockResolvedValue([]);
    expect(new URL(await negotiatedUrl()).searchParams.has("hdr")).toBe(false);
    expect(getStreamInfoMock).toHaveBeenLastCalledWith("m1", expect.objectContaining({ supportsHdr: false }));

    getDisplayHdrTypesMock.mockResolvedValue(null);
    expect(new URL(await negotiatedUrl()).searchParams.has("hdr")).toBe(false);
  });
});

describe("usePlaySession resume", () => {
  it("fetches watch progress and passes its position to startStream", async () => {
    const { renderHook, waitFor } = await import("@testing-library/react");
    const { usePlaySession } = await import("./usePlaySession");

    getStreamInfoMock.mockResolvedValue({ playMethod: "PLAY_METHOD_DIRECT_PLAY" });
    eveyFetchMock
      .mockResolvedValueOnce({ positionMs: "42000", played: false }) // progress
      .mockResolvedValueOnce({ token: "t1", mediaId: "m1" }); // stream-token
    startStreamMock.mockResolvedValue({ id: "sess1" });

    const { result } = renderHook(() => usePlaySession("m1"));

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(eveyFetchMock).toHaveBeenCalledWith({
      url: "/api/v1/playback/m1/progress",
      method: "GET",
    });
    expect(startStreamMock).toHaveBeenCalledWith(
      expect.objectContaining({ startPositionMs: "42000" }),
    );
    expect(result.current.progress).toEqual({ positionMs: "42000", played: false });
  });

  it("falls back to position 0 when the item is already fully watched", async () => {
    const { renderHook, waitFor } = await import("@testing-library/react");
    const { usePlaySession } = await import("./usePlaySession");

    getStreamInfoMock.mockResolvedValue({ playMethod: "PLAY_METHOD_DIRECT_PLAY" });
    eveyFetchMock
      .mockResolvedValueOnce({ positionMs: "999000", played: true })
      .mockResolvedValueOnce({ token: "t1", mediaId: "m1" });
    startStreamMock.mockResolvedValue({ id: "sess1" });

    const { result } = renderHook(() => usePlaySession("m1"));

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(startStreamMock).toHaveBeenCalledWith(
      expect.objectContaining({ startPositionMs: "0" }),
    );
  });

  // The session starts where the player will (resolveStartPositionMs and
  // the one-time hints), so the encoder is pre-warmed at the right place.
  it.each([
    ["a title stopped in its last seconds", { positionMs: "89000", durationMs: "90000", played: false }, undefined, "0"],
    ["a viewer starting over", { positionMs: "42000", durationMs: "90000", played: false }, { restart: true }, "0"],
    ["a cast hand-over", { positionMs: "42000", durationMs: "90000", played: false }, { startSec: 61 }, "61000"],
  ])("starts the session where the player starts: %s", async (_label, wp, initial, expected) => {
    const { renderHook, waitFor } = await import("@testing-library/react");
    const { usePlaySession } = await import("./usePlaySession");

    getStreamInfoMock.mockResolvedValue({ playMethod: "PLAY_METHOD_DIRECT_PLAY" });
    eveyFetchMock
      .mockResolvedValueOnce(wp)
      .mockResolvedValueOnce({ token: "t1", mediaId: "m1" });
    startStreamMock.mockResolvedValue({ id: "sess1" });

    const { result } = renderHook(() => usePlaySession("m1", initial));

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(startStreamMock).toHaveBeenCalledWith(
      expect.objectContaining({ startPositionMs: expected }),
    );
  });

  it("falls back to position 0 when the progress fetch fails", async () => {
    const { renderHook, waitFor } = await import("@testing-library/react");
    const { usePlaySession } = await import("./usePlaySession");

    getStreamInfoMock.mockResolvedValue({ playMethod: "PLAY_METHOD_DIRECT_PLAY" });
    eveyFetchMock
      .mockRejectedValueOnce(new Error("network")) // progress fetch fails
      .mockResolvedValueOnce({ token: "t1", mediaId: "m1" }); // stream-token
    startStreamMock.mockResolvedValue({ id: "sess1" });

    const { result } = renderHook(() => usePlaySession("m1"));

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(startStreamMock).toHaveBeenCalledWith(
      expect.objectContaining({ startPositionMs: "0" }),
    );
    expect(result.current.progress).toBeNull();
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

  // A title whose file was deleted from disk: the server refuses it with
  // MEDIA_FILE_MISSING. It is a title that is gone, not a failure to retry.
  it("reads a deleted file as file_missing, not as an error", async () => {
    const { renderHook, waitFor } = await import("@testing-library/react");
    const { usePlaySession } = await import("./usePlaySession");

    getStreamInfoMock.mockRejectedValue(Object.assign(new Error("gone"), { httpStatus: 404, errorCode: "media_file_missing" }));
    eveyFetchMock.mockResolvedValue({ positionMs: "0" });

    const { result } = renderHook(() => usePlaySession("m3"));

    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.shedReason).toBe("file_missing");
    expect(result.current.isError).toBe(false);
  });

  // The title already left the catalogue (the scan pruned it while a list
  // still showed it): a plain 404, read as unavailable rather than as an
  // error to retry. Plain, because a parental block answers the same 404 and
  // must not be told apart from a title that does not exist.
  it("reads a plain 404 as media_unavailable, not as an error", async () => {
    const { renderHook, waitFor } = await import("@testing-library/react");
    const { usePlaySession } = await import("./usePlaySession");

    getStreamInfoMock.mockRejectedValue(Object.assign(new Error("Média introuvable"), { httpStatus: 404 }));
    eveyFetchMock.mockResolvedValue({ positionMs: "0" });

    const { result } = renderHook(() => usePlaySession("m4"));

    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.shedReason).toBe("media_unavailable");
    expect(result.current.isError).toBe(false);
  });
});

// Server-side load shed (HTTP 503): expo-video's native player only exposes a
// free-text error message, so usePlaySession probes the manifest URL itself
// before handing it to the player - this is the only place that can read the
// server's structured shed code.
describe("buildStreamUrl", () => {
  // startSec was dead on arrival: no HLS handler parses it (transcode_handler.go
  // reads only maxWidth/maxHeight/maxBitrate/videoCodecs/audioCodecs/containers/
  // hdr/fp). The server always publishes the VOD manifest from t=0, so emitting
  // it made the client believe the timeline was shifted: it set
  // transcodeOffset=startSec and added that to every timeUpdate - and on TV to
  // the reported duration as well - over a playlist that still began at 0.
  it("carries the hardware H.264 level ceiling the box probed at load", async () => {
    const { buildStreamUrl } = await import("./usePlaySession");
    // The probe is async at module load; let it land before reading the URL.
    await new Promise((r) => setTimeout(r, 0));
    const url = buildStreamUrl("http://srv", "m1", { playMethod: "PLAY_METHOD_TRANSCODE" }, "tok");
    expect(url).toContain("maxH264Level=42");
    expect(getMaxH264LevelMock).toHaveBeenCalled();
  });

  it("carries the probed channel ceiling, and nothing when there is none", async () => {
    const { buildStreamUrl } = await import("./usePlaySession");
    const info = { playMethod: "PLAY_METHOD_TRANSCODE" } as const;
    expect(buildStreamUrl("http://srv", "m1", info, "tok", { maxAudioChannels: 2 })).toContain("maxAudioChannels=2");
    expect(buildStreamUrl("http://srv", "m1", info, "tok", { maxAudioChannels: 6 })).toContain("maxAudioChannels=6");
    expect(buildStreamUrl("http://srv", "m1", info, "tok")).not.toContain("maxAudioChannels");
  });

  // The TV declared no HDR capability at all, so the server saw an SDR client
  // on every title: measured 2026-09-19, a 2160p HDR10 HEVC that direct-plays
  // with hdr=1 was going out re-encoded for want of this one param. Absent
  // rather than hdr=0 when the panel is SDR, so an SDR box's URL - and the
  // cache key behind it - stays byte-identical to before.
  it("declares hdr=1 only when the panel renders HDR", async () => {
    const { buildStreamUrl } = await import("./usePlaySession");
    const info = { playMethod: "PLAY_METHOD_TRANSCODE" } as const;
    expect(buildStreamUrl("http://srv", "m1", info, "tok", { supportsHdr: true })).toContain("hdr=1");
    expect(buildStreamUrl("http://srv", "m1", info, "tok", { supportsHdr: false })).not.toContain("hdr");
    expect(buildStreamUrl("http://srv", "m1", info, "tok", {})).not.toContain("hdr");
  });

  // startSec is a pre-warm hint the server reads (transcode_start_hint.go).
  it("sends the resume position as a whole-second hint, and nothing from the start", async () => {
    const { buildStreamUrl } = await import("./usePlaySession");
    const info = { playMethod: "PLAY_METHOD_TRANSCODE" } as const;
    expect(buildStreamUrl("http://srv", "m1", info, "tok", { startSec: 42.7 })).toContain("startSec=42");
    expect(buildStreamUrl("http://srv", "m1", info, "tok")).not.toContain("startSec");
  });

  // The quality menu used to reach GetStreamInfo alone: the verdict changed,
  // the master served the same ladder. Both ceilings ride the master URL now.
  it("carries the picked quality as maxHeight and the panel as screenHeight", async () => {
    const { buildStreamUrl } = await import("./usePlaySession");
    const info = { playMethod: "PLAY_METHOD_TRANSCODE" } as const;
    const url = buildStreamUrl("http://srv", "m1", info, "tok", { maxHeight: 720, screenHeight: 1080 });
    expect(url).toContain("maxHeight=720");
    expect(url).toContain("screenHeight=1080");
    const auto = buildStreamUrl("http://srv", "m1", info, "tok", { screenHeight: 1080 });
    expect(auto).not.toContain("maxHeight");
    expect(auto).toContain("screenHeight=1080");
  });

  // The TV told the gRPC calls which tracks it wanted and then asked for a URL
  // that named neither, so the manifest kept serving the default audio and never
  // burnt a selected bitmap subtitle. Worse, the URL was byte-identical for
  // every track, so the native player had no reason to reload at all.
  it("names the chosen audio and subtitle tracks", async () => {
    const { buildStreamUrl } = await import("./usePlaySession");
    const info = { playMethod: "PLAY_METHOD_TRANSCODE" } as const;
    const url = buildStreamUrl("http://srv", "m1", info, "tok", { maxAudioChannels: 2, audioTrack: 1, subtitleTrack: 0 });
    expect(url).toContain("audioTrack=1");
    expect(url).toContain("subtitleTrack=0");
  });

  it("differs between two track choices - an identical URL never reloads", async () => {
    const { buildStreamUrl } = await import("./usePlaySession");
    const info = { playMethod: "PLAY_METHOD_TRANSCODE" } as const;
    expect(buildStreamUrl("http://srv", "m1", info, "tok", { maxAudioChannels: 2, audioTrack: 0 }))
      .not.toBe(buildStreamUrl("http://srv", "m1", info, "tok", { maxAudioChannels: 2, audioTrack: 1 }));
  });

  it("omits the params when no track was chosen, leaving the server's own default", async () => {
    const { buildStreamUrl } = await import("./usePlaySession");
    const url = buildStreamUrl("http://srv", "m1", { playMethod: "PLAY_METHOD_TRANSCODE" }, "tok", { maxAudioChannels: 2 });
    expect(url).not.toContain("audioTrack");
    expect(url).not.toContain("subtitleTrack");
  });

  // Direct play streams the original file: no manifest, so no track params to
  // put anywhere. Pinned so nobody "fixes" this by appending them there too.
  it("leaves the direct-play URL alone", async () => {
    const { buildStreamUrl } = await import("./usePlaySession");
    const url = buildStreamUrl("http://srv", "m1", { playMethod: "PLAY_METHOD_DIRECT_PLAY" }, "tok", { maxAudioChannels: 2, audioTrack: 1, subtitleTrack: 0 });
    expect(url).not.toContain("audioTrack");
    expect(url).toContain("/stream?token=tok");
  });
});

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

// The TV player is a native segment fetcher (expo-video/ExoPlayer): it cannot
// re-mint the 5-minute sliding token mid-stream, so a living-room pause longer
// than the default TTL 401'd the next segment. Mobile got purpose=native long
// ago; the TV had been left behind.
describe("usePlaySession stream token", () => {
  it("requests the long-lived native token, not the 5-minute default", async () => {
    const { renderHook, waitFor } = await import("@testing-library/react");
    const { usePlaySession } = await import("./usePlaySession");

    getStreamInfoMock.mockResolvedValue({ playMethod: "PLAY_METHOD_DIRECT_PLAY" });
    eveyFetchMock
      .mockResolvedValueOnce({ positionMs: "0", played: false })
      .mockResolvedValueOnce({ token: "t1", mediaId: "m1" });
    startStreamMock.mockResolvedValue({ id: "sess1" });

    const { result } = renderHook(() => usePlaySession("m1"));
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(eveyFetchMock).toHaveBeenCalledWith(
      expect.objectContaining({ url: expect.stringContaining("stream-token?purpose=native") }),
    );
  });

  it("mints a fresh token on restart instead of replaying the dead one", async () => {
    const { renderHook, waitFor, act } = await import("@testing-library/react");
    const { usePlaySession } = await import("./usePlaySession");

    getStreamInfoMock.mockResolvedValue({ playMethod: "PLAY_METHOD_DIRECT_PLAY" });
    eveyFetchMock
      .mockResolvedValueOnce({ positionMs: "0", played: false })
      .mockResolvedValueOnce({ token: "t-dead", mediaId: "m1" })
      .mockResolvedValue({ token: "t-fresh", mediaId: "m1" });
    startStreamMock.mockResolvedValue({ id: "sess1" });
    stopStreamMock.mockResolvedValue({});

    const { result } = renderHook(() => usePlaySession("m1"));
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    act(() => result.current.restartWithTracks());
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    // Retry exists to recover from a 401: reusing the cached token rebuilds
    // the same dead URL and fails identically, trapping the viewer.
    const tokenCalls = eveyFetchMock.mock.calls.filter(
      (c: unknown[]) => String((c[0] as { url?: string })?.url ?? "").includes("stream-token"),
    );
    expect(tokenCalls.length).toBeGreaterThanOrEqual(2);
  });
});

// Fixes the phone had and the television's copy lacked, until both players
// shared one play session (2026-09-24).
describe("usePlaySession: what the phone's copy already guarded", () => {
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

  it("percent-encodes a deep-link traversal id so the stream URL cannot pivot to another API path", async () => {
    const { buildStreamUrl } = await import("./usePlaySession");
    const url = buildStreamUrl("http://srv", "../admin/api-keys", { playMethod: "PLAY_METHOD_DIRECT_PLAY" }, "tok");
    expect(url).toBe("http://srv/api/v1/media/..%2Fadmin%2Fapi-keys/stream?token=tok");
  });

  it("probes the HLS master with an abortable request (status wanted, never a body)", async () => {
    const { renderHook, waitFor } = await import("@testing-library/react");
    const { usePlaySession } = await import("./usePlaySession");

    getStreamInfoMock.mockResolvedValue({ playMethod: "PLAY_METHOD_DIRECT_STREAM" });
    eveyFetchMock
      .mockResolvedValueOnce(null)
      .mockResolvedValueOnce({ token: "t1", mediaId: "m1" });
    startStreamMock.mockResolvedValue({ id: "sess1" });

    const { result } = renderHook(() => usePlaySession("m1"));
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining("/hls/master.m3u8?"),
      expect.objectContaining({ signal: expect.anything() }),
    );
  });

  it("keeps the LAST requested track when an earlier negotiation answers after it", async () => {
    const { renderHook, waitFor } = await import("@testing-library/react");
    const { usePlaySession } = await import("./usePlaySession");

    const streams = [
      { type: "MEDIA_STREAM_TYPE_AUDIO", index: 1 },
      { type: "MEDIA_STREAM_TYPE_AUDIO", index: 2 },
    ];
    let resolveSlow: (v: unknown) => void = () => {};
    getStreamInfoMock
      .mockResolvedValueOnce({ playMethod: "PLAY_METHOD_TRANSCODE", mediaStreams: streams })
      .mockImplementationOnce(() => new Promise((res) => { resolveSlow = res; }))
      .mockResolvedValueOnce({ playMethod: "PLAY_METHOD_TRANSCODE", mediaStreams: streams });
    eveyFetchMock.mockResolvedValue({ token: "t1", mediaId: "m1" });
    startStreamMock.mockImplementation(async () => ({ id: `sess-${startStreamMock.mock.calls.length}` }));

    const { result } = renderHook(() => usePlaySession("m1"));
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    result.current.restartWithTracks(1);
    result.current.restartWithTracks(2);
    await waitFor(() => expect(result.current.streamUrl).toContain("audioTrack=1"));

    resolveSlow({ playMethod: "PLAY_METHOD_TRANSCODE", mediaStreams: streams });
    await new Promise((r) => setTimeout(r, 30));

    expect(result.current.streamUrl).toContain("audioTrack=1");
    expect(result.current.streamUrl).not.toContain("audioTrack=0");
  });
});
