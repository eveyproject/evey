import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";

const showCastDialog = vi.fn();
let castState = 0; // NOT_CONNECTED
let castClientInstance: { loadMedia: ReturnType<typeof vi.fn> } | null = null;
let castDeviceModelName: string | null = null;
let castDeviceFriendlyName: string | null = null;
let sessionDeviceModelName: string | null = null;
let castMediaStatus: { streamPosition?: number; playerState?: string; idleReason?: string | number | null } | null = null;

// The caps channel the connected receiver may answer on. A test scripts it:
// null = no channel (no session, not our receiver); a fake channel records
// sends, closes, and lets the test push answers (prompted or not).
type FakeCapsChannel = {
  sends: number; closed: boolean; unsubscribed: number;
  push(raw: unknown): void;
  channel: { transport: { send: (r: unknown) => Promise<void>; subscribe: (h: (raw: unknown) => void) => () => void }; close: () => Promise<void> };
};
function fakeCapsChannel(answerFor?: (sendIndex: number) => unknown): FakeCapsChannel {
  let handler: ((raw: unknown) => void) | null = null;
  const f: FakeCapsChannel = {
    sends: 0, closed: false, unsubscribed: 0,
    push(raw) { if (!f.closed) handler?.(raw); },
    channel: {
      transport: {
        send: async () => {
          f.sends += 1;
          const a = answerFor?.(f.sends);
          if (a !== undefined) queueMicrotask(() => { if (!f.closed) handler?.(a); });
        },
        subscribe: (h) => { handler = h; return () => { f.unsubscribed += 1; handler = null; }; },
      },
      close: async () => { f.closed = true; handler = null; },
    },
  };
  return f;
}
let capsChannels: Array<FakeCapsChannel | null> = [];
let openedChannels = 0;

vi.mock("./useChromecast", () => ({
  CastContext: { showCastDialog: () => showCastDialog() },
  CastStateEnum: { CONNECTED: 4 },
  useCastStateHook: () => castState,
  useRemoteMediaClientHook: () => castClientInstance,
  useMediaStatusHook: () => castMediaStatus,
  useCastDeviceHook: () =>
    castDeviceModelName || castDeviceFriendlyName
      ? { modelName: castDeviceModelName, friendlyName: castDeviceFriendlyName }
      : null,
  getConnectedDeviceModelName: async () => sessionDeviceModelName,
  openCapsChannel: async () => {
    const next = capsChannels[openedChannels] ?? null;
    openedChannels += 1;
    return next ? next.channel : null;
  },
}));

const setAudioBoost = vi.fn();
vi.mock("../lib/audioBoost", () => ({ setAudioBoost: (v: number) => setAudioBoost(v) }));

// The Cast media notification is the foreground service that keeps the sender
// alive with the screen off (2026-07-26 operator-box cut). The permission ask
// is mocked here; its own logic is covered in notificationPermission.test.ts.
const ensureNotificationPermission = vi.fn().mockResolvedValue(undefined);
vi.mock("../lib/notificationPermission", () => ({
  ensureNotificationPermission: () => ensureNotificationPermission(),
}));

const eveyFetchMock = vi.fn();
const getStreamInfoMock = vi.fn();
vi.mock("@evey/api", () => ({
  eveyFetch: (...args: unknown[]) => eveyFetchMock(...args),
  playbackServiceGetStreamInfo: (...args: unknown[]) => getStreamInfoMock(...args),
}));

// useCastSession pulls fetchStreamToken in from usePlaySession, which at
// import time also drags in server-config (expo-secure-store) - unmockable
// in this jsdom environment (ExpoGlobal is undefined here). Stub it the same
// way usePlaySession.test.ts does; nothing in these tests exercises it.
vi.mock("../lib/server-config", () => ({
  getServerUrl: vi.fn().mockResolvedValue("http://localhost:8080"),
}));

// deviceCodecs (also dragged in transitively) reads NativeModules.MediaCaps at
// import time - the globally-aliased react-native mock has no NativeModules.
vi.mock("react-native", () => ({
  Platform: { OS: "android", select: (obj: Record<string, unknown>) => obj.android },
  NativeModules: {},
}));

import { useCastSession } from "./useCastSession";

function fakePlayer() {
  return { pause: vi.fn(), play: vi.fn(), volume: 1 };
}

function mount(
  player: ReturnType<typeof fakePlayer>,
  streamUrl = "https://example.invalid/s.m3u8",
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  opts: { streamInfo?: any; serverUrl?: string | null; restoreLocalVolume?: () => void } = {},
) {
  return renderHook(() =>
    useCastSession({
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      player: player as any,
      restoreLocalVolume: opts.restoreLocalVolume ?? (() => {}),
      streamUrl,
      mediaId: "m1",
      title: "A Film",
      currentTime: 0,
      fallbackDurationSec: 120,
      streamInfo: opts.streamInfo ?? null,
      serverUrl: opts.serverUrl ?? null,
    }),
  );
}

beforeEach(() => {
  castState = 0;
  castClientInstance = null;
  castDeviceModelName = null;
  castDeviceFriendlyName = null;
  sessionDeviceModelName = null;
  castMediaStatus = null;
  showCastDialog.mockReset();
  setAudioBoost.mockReset();
  eveyFetchMock.mockReset();
  getStreamInfoMock.mockReset();
  getStreamInfoMock.mockResolvedValue({ playMethod: "PLAY_METHOD_TRANSCODE" });
  ensureNotificationPermission.mockClear();
  capsChannels = [];
  openedChannels = 0;
  vi.useRealTimers();
});

describe("useCastSession", () => {
  // The progress reporter forwards this name to the server; a named cast is
  // what makes the server leave admin commands for the TV's beacon instead of
  // handing them to the phone.
  it("exposes the receiver's friendly name while connected", () => {
    castState = 4;
    castDeviceFriendlyName = "Salon TV";
    const { result } = mount(fakePlayer());
    expect(result.current.castDeviceName).toBe("Salon TV");
  });

  it("falls back to a generic name when the device has not been delivered yet", () => {
    castState = 4;
    const { result } = mount(fakePlayer());
    expect(result.current.castDeviceName).toBe("Chromecast");
  });

  it("exposes an empty cast device name when not connected", () => {
    const { result } = mount(fakePlayer());
    expect(result.current.castDeviceName).toBe("");
  });

  // The unmute ran on every volume change, not only when a cast ended: it
  // depended on the level, and it wrote the raw level over the player's trim,
  // so under 100% the phone played the square of what the bar showed.
  it("restores the local volume only when a cast ends", async () => {
    const restoreLocalVolume = vi.fn();
    const { rerender } = mount(fakePlayer(), undefined, { restoreLocalVolume });
    rerender();
    expect(restoreLocalVolume).not.toHaveBeenCalled();
    await act(async () => { castState = 4; rerender(); });
    await act(async () => { castState = 0; rerender(); });
    rerender();
    expect(restoreLocalVolume).toHaveBeenCalledTimes(1);
  });

  // Tapping cast used to pause the local player and set volume to 0 BEFORE
  // opening the picker. Dismissing the picker changes no cast state, so no
  // effect re-ran and nothing undid it: the film sat paused and silent, and the
  // volume only came back on the next swipe. Muting belongs at the moment the
  // receiver actually takes over, not at the moment we ask.
  it("does not silence the local player just for opening the picker", async () => {
    const player = fakePlayer();
    const { result } = mount(player);

    await act(async () => {
      await result.current.handleCast();
    });

    expect(showCastDialog).toHaveBeenCalledTimes(1);
    expect(player.pause).not.toHaveBeenCalled();
    expect(player.volume).toBe(1);
  });

  // Without POST_NOTIFICATIONS the Cast media notification (foreground
  // service) never shows on Android 13+: Android freezes the app ~60s after
  // screen-off and some operator boxes stop playback when its sender vanishes
  // (prod logs 2026-07-26 18:52:05, ReportProgress dying with no StopStream).
  // The tap on the cast button is the user-activation moment to ask.
  it("asks for the notification permission when the user initiates a cast", async () => {
    const { result } = mount(fakePlayer());

    await act(async () => {
      await result.current.handleCast();
    });

    expect(ensureNotificationPermission).toHaveBeenCalled();
  });

  it("reports the cast state it is given", () => {
    const { result } = mount(fakePlayer());
    expect(result.current.castConnected).toBe(false);
  });

  // Server-side cast progress derivation (internal/adapters/driving/http/
  // cast_observe.go) only activates for a token minted with purpose=cast. The
  // stream URL carries the local player's purpose=native token, which the
  // derivation ignores - casting that URL as-is left the receiver's fetches
  // invisible to the tracker (frozen progress, false "not watched" on resume).
  it("mints a purpose=cast token and swaps it into the casted URL, keeping other params", async () => {
    castState = 4; // CONNECTED
    castClientInstance = { loadMedia: vi.fn().mockResolvedValue(undefined) };
    eveyFetchMock.mockResolvedValue({ token: "cast-tok-999", mediaId: "m1" });

    const { result } = mount(
      fakePlayer(),
      "https://example.invalid/api/v1/media/m1/hls/master.m3u8?token=native-tok&videoCodecs=h264",
    );

    await act(async () => {
      await result.current.handleCast();
    });

    expect(eveyFetchMock).toHaveBeenCalledWith({
      url: "/api/v1/media/m1/stream-token?purpose=cast",
      method: "POST",
    });
    // Mounting with castConnected already true also fires the auto-load
    // effect once on top of the explicit handleCast() call above - both hit
    // the same code path, so asserting on the LAST call is enough here.
    expect(castClientInstance!.loadMedia).toHaveBeenCalled();
    const calls = castClientInstance!.loadMedia.mock.calls;
    const call = calls[calls.length - 1][0];
    const castedUrl = new URL(call.mediaInfo.contentUrl);
    expect(castedUrl.searchParams.get("token")).toBe("cast-tok-999");
    expect(castedUrl.searchParams.get("videoCodecs")).toBe("h264");
  });

  // With streamInfo/serverUrl available, the SERVER is asked for the
  // RECEIVER's verdict (GetStreamInfo with the profile derived from the
  // connected device's model name) instead of just swapping the token into
  // the phone-negotiated URL - a Chromecast gen 1-3 must never see the hevc
  // the phone itself advertised, and the phone keeps no direct-play rule of
  // its own: a server verdict of direct_stream/transcode is the HLS master.
  it("negotiates the receiver's profile from the connected device's model name", async () => {
    castState = 4; // CONNECTED
    castDeviceModelName = "Chromecast"; // gen 1-3 floor: h264 only, no hevc
    castClientInstance = { loadMedia: vi.fn().mockResolvedValue(undefined) };
    eveyFetchMock.mockResolvedValue({ token: "cast-tok-999", mediaId: "m1" });
    getStreamInfoMock.mockResolvedValue({ playMethod: "PLAY_METHOD_DIRECT_STREAM" });

    const { result } = mount(fakePlayer(), "https://example.invalid/s.m3u8", {
      serverUrl: "https://example.invalid",
      streamInfo: {
        playMethod: "PLAY_METHOD_DIRECT_PLAY",
        container: "mkv",
        mediaStreams: [
          { type: "MEDIA_STREAM_TYPE_VIDEO", codec: "hevc", bitDepth: 10, height: 2160 },
          { type: "MEDIA_STREAM_TYPE_AUDIO", codec: "aac", isDefault: true },
        ],
      },
    });

    await act(async () => {
      await result.current.handleCast();
    });

    const calls = castClientInstance!.loadMedia.mock.calls;
    const call = calls[calls.length - 1][0];
    const castedUrl = new URL(call.mediaInfo.contentUrl);
    expect(castedUrl.pathname).toBe("/api/v1/media/m1/hls/master.m3u8");
    expect(castedUrl.searchParams.get("videoCodecs")).toBe("h264");
    expect(castedUrl.searchParams.get("maxH264Height")).toBe("1080");
    expect(castedUrl.searchParams.get("token")).toBe("cast-tok-999");
    // The verdict was asked with the RECEIVER's profile, not the phone's caps.
    expect(getStreamInfoMock).toHaveBeenCalledWith("m1", expect.objectContaining({
      supportedCodecs: ["h264"], supportedContainers: ["mp4"], supportsHdr: false, maxHeight: 1080, maxH264Height: 1080,
    }));
  });

  // The server says the receiver can play the raw file: the cast loads it.
  it("hands the receiver the raw file when the server's receiver verdict is direct play", async () => {
    castState = 4;
    castDeviceModelName = "Chromecast";
    castClientInstance = { loadMedia: vi.fn().mockResolvedValue(undefined) };
    eveyFetchMock.mockResolvedValue({ token: "cast-tok-999", mediaId: "m1" });
    getStreamInfoMock.mockResolvedValue({ playMethod: "PLAY_METHOD_DIRECT_PLAY" });

    const { result } = mount(fakePlayer(), "https://example.invalid/s.m3u8", {
      serverUrl: "https://example.invalid",
      streamInfo: { playMethod: "PLAY_METHOD_DIRECT_PLAY", container: "mp4", mediaStreams: [] },
    });
    await act(async () => {
      await result.current.handleCast();
    });

    const calls = castClientInstance!.loadMedia.mock.calls;
    const castedUrl = new URL(calls[calls.length - 1][0].mediaInfo.contentUrl);
    expect(castedUrl.pathname).toBe("/api/v1/media/m1/stream");
    expect(castedUrl.searchParams.get("token")).toBe("cast-tok-999");
  });
});

  // The per-model profile table maps SDK modelName substrings that could NOT
  // be verified online for the 2024 devices (Chromecast HD, Google TV
  // Streamer) - nobody publishes the exact md= strings. This beacon is how
  // the real strings reach the admin logs from the FIRST real-device
  // connection, so the table can be corrected on evidence instead of folklore.
  it("reports the receiver's raw modelName through the player-event beacon on load", async () => {
    castState = 4;
    const loadMedia = vi.fn().mockResolvedValue(undefined);
    castClientInstance = { loadMedia };
    castDeviceModelName = "Chromecast Ultra";
    eveyFetchMock.mockResolvedValue({ token: "cast-tok", mediaId: "m1" });

    const { result } = mount(fakePlayer());
    await act(async () => { await result.current.handleCast(); });

    const beacon = eveyFetchMock.mock.calls
      .map((c) => c[0] as { url: string; data?: { type?: string; details?: string } })
      .find((c) => c.url.includes("/player-event"));
    expect(beacon).toBeDefined();
    expect(beacon!.data!.type).toBe("cast_receiver_model");
    expect(beacon!.data!.details).toContain("model=Chromecast Ultra");
  });

  // First real-device test (a series episode, 2026-07-25 00:51): title and controls
  // showed but the TV spun forever and NEVER fetched from the server. Two
  // causes, both fixed here and locked by these tests:
  // - the Default Media Receiver plays contentId; we sent the raw media UUID
  //   there (the working web path sends the URL as contentId);
  // - our HLS is fMP4: without the hlsVideoSegmentFormat=FMP4 hint the
  //   receiver assumes MPEG2-TS and "loads forever with no picture" (the
  //   exact web fix validated on this TV on 2026-07-22, never ported).
  it("loads media with the URL as contentId and fMP4 hints for HLS", async () => {
    castState = 4;
    const loadMedia = vi.fn().mockResolvedValue(undefined);
    castClientInstance = { loadMedia };
    eveyFetchMock.mockResolvedValue({ token: "cast-tok", mediaId: "m1" });

    const { result } = mount(fakePlayer());
    await act(async () => { await result.current.handleCast(); });

    // Auto-load effect + explicit handleCast both fire (same code path);
    // assert on the last call like the other tests here.
    expect(loadMedia).toHaveBeenCalled();
    const info = loadMedia.mock.calls[loadMedia.mock.calls.length - 1][0].mediaInfo;
    expect(info.contentId).toBe(info.contentUrl);
    expect(info.contentUrl).toContain("m3u8");
    expect(info.hlsVideoSegmentFormat).toBe("FMP4");
    expect(info.hlsSegmentFormat).toBe("FMP4");
  });

  // The useCastDevice hook delivers the device on a later render than the
  // auto-load, so reading it at load time reported model=unknown for a real
  // TV and silently floored the capability profile. The session is the
  // authoritative source at that instant.
  it("reads the receiver model from the live session, not the stale hook", async () => {
    castState = 4;
    const loadMedia = vi.fn().mockResolvedValue(undefined);
    castClientInstance = { loadMedia };
    castDeviceModelName = null; // hook has not delivered yet
    sessionDeviceModelName = "Chromecast Ultra";
    eveyFetchMock.mockResolvedValue({ token: "cast-tok", mediaId: "m1" });

    const { result } = mount(fakePlayer());
    await act(async () => { await result.current.handleCast(); });

    const beacon = eveyFetchMock.mock.calls
      .map((c) => c[0] as { url: string; data?: { details?: string } })
      .find((c) => c.url.includes("/player-event"));
    expect(beacon!.data!.details).toContain("model=Chromecast Ultra");
  
});


// The receiver's OBSERVED capabilities (its caps channel) drive the
// negotiation: they see the display behind the box, which no model name can
// (a 4K box on a 1080p screen - prod 2026-09-19). observed > model > floor,
// and the profile is settled BEFORE the server is asked for its verdict, so a
// table entry richer than the real screen never buys a raw HEVC/4K file.
describe("useCastSession - receiver caps handshake", () => {
  const observed = { videoCodecs: ["h264", "vp9"], maxHeight: 1080, hdr: false, nativeDolby: false, maxAudioChannels: 2, maxH264Level: 42, h264MaxHeight: 1080 };
  const report = (caps = observed) => ({ type: "caps", caps, userAgent: "CrKey/1.56" });
  const richModel = "Google TV Streamer"; // table: h264/hevc/vp9/av1, 4K, HDR
  const hevcDirectPlaySource = {
    playMethod: "PLAY_METHOD_DIRECT_PLAY",
    container: "mp4",
    mediaStreams: [
      { type: "MEDIA_STREAM_TYPE_VIDEO", codec: "hevc", bitDepth: 10, height: 2160 },
      { type: "MEDIA_STREAM_TYPE_AUDIO", codec: "aac", isDefault: true },
    ],
  };
  function connected() {
    castState = 4;
    castClientInstance = { loadMedia: vi.fn().mockResolvedValue(undefined) };
    eveyFetchMock.mockResolvedValue({ token: "cast-tok", mediaId: "m1" });
  }
  const mountRich = () => mount(fakePlayer(), "https://example.invalid/s.m3u8", { serverUrl: "https://example.invalid", streamInfo: hevcDirectPlaySource });
  const lastCastedUrl = () => { const calls = castClientInstance!.loadMedia.mock.calls; return new URL(calls[calls.length - 1][0].mediaInfo.contentUrl); };
  const capsBeacon = () => eveyFetchMock.mock.calls.map((c) => c[0] as { url: string; data?: { type?: string; details?: string } }).find((c) => c.data?.type === "cast_receiver_caps")?.data?.details;

  it("an immediate answer: observed wins over a richer model, and the server is asked with it BEFORE the verdict", async () => {
    connected();
    sessionDeviceModelName = richModel;
    const ch = fakeCapsChannel(() => report());
    capsChannels = [ch];
    // The server, asked with the RICH table profile, would say direct play
    // (a raw 4K HEVC file). Asked with the observed one it must not.
    getStreamInfoMock.mockImplementation(async (_id: string, params: { supportedCodecs: string[] }) =>
      ({ playMethod: params.supportedCodecs.includes("hevc") ? "PLAY_METHOD_DIRECT_PLAY" : "PLAY_METHOD_TRANSCODE" }));

    const { result } = mountRich();
    await act(async () => { await result.current.handleCast(); });

    expect(getStreamInfoMock).toHaveBeenCalledWith("m1", expect.objectContaining({
      supportedCodecs: ["h264", "vp9"], supportsHdr: false, maxHeight: 1080, maxH264Height: 1080,
    }));
    const url = lastCastedUrl();
    expect(url.pathname, "no raw file for a screen that cannot show it").toBe("/api/v1/media/m1/hls/master.m3u8");
    expect(url.searchParams.get("videoCodecs")).toBe("h264,vp9");
    expect(url.searchParams.get("maxAudioChannels")).toBe("2");
    expect(url.searchParams.has("hdr")).toBe(false);
    expect(capsBeacon()).toContain("answered=1 codecs=h264/vp9 maxHeight=1080");
    expect(ch.closed, "channel closed after the ask").toBe(true);
    expect(ch.unsubscribed).toBe(1);
  });

  it("a late answer (the app still launching) is taken on a later ask", async () => {
    vi.useFakeTimers();
    connected();
    sessionDeviceModelName = richModel;
    const ch = fakeCapsChannel((n) => (n >= 3 ? report() : undefined));
    capsChannels = [ch];
    const { result } = mountRich();
    const timersBefore = vi.getTimerCount(); // the hook's own stall tracker interval
    await act(async () => {
      const p = result.current.handleCast();
      await vi.advanceTimersByTimeAsync(1500);
      await p;
    });
    expect(ch.sends).toBe(3);
    expect(lastCastedUrl().searchParams.get("videoCodecs")).toBe("h264,vp9");
    expect(ch.closed).toBe(true);
    expect(vi.getTimerCount(), "no handshake timer left behind").toBe(timersBefore);
  });

  it("the receiver's unprompted READY broadcast is accepted", async () => {
    vi.useFakeTimers();
    connected();
    sessionDeviceModelName = richModel;
    const ch = fakeCapsChannel();
    capsChannels = [ch];
    const { result } = mountRich();
    await act(async () => {
      const p = result.current.handleCast();
      await vi.advanceTimersByTimeAsync(800);
      ch.push(JSON.stringify(report()));
      await vi.advanceTimersByTimeAsync(50);
      await p;
    });
    expect(lastCastedUrl().searchParams.get("videoCodecs")).toBe("h264,vp9");
    expect(ch.closed).toBe(true);
  });

  it("no answer within the budget: the model table, then the verdict", async () => {
    vi.useFakeTimers();
    connected();
    sessionDeviceModelName = "Chromecast Ultra"; // table: hevc 4K HDR, h264 held at 1080
    const ch = fakeCapsChannel();
    capsChannels = [ch];
    const { result } = mountRich();
    const timersBefore = vi.getTimerCount();
    await act(async () => {
      const p = result.current.handleCast();
      await vi.advanceTimersByTimeAsync(5100);
      await p;
    });
    expect(ch.sends).toBeGreaterThanOrEqual(10);
    expect(getStreamInfoMock).toHaveBeenCalledWith("m1", expect.objectContaining({ supportedCodecs: ["h264", "hevc", "vp9"], maxH264Height: 1080, maxHeight: 2160 }));
    expect(capsBeacon()).toBe("answered=0");
    expect(ch.closed).toBe(true);
    expect(vi.getTimerCount()).toBe(timersBefore);
  });

  it("no answer and no model: the floor", async () => {
    vi.useFakeTimers();
    connected();
    sessionDeviceModelName = null;
    castDeviceModelName = null;
    capsChannels = [null]; // not even a channel (a Default Media Receiver)
    const { result } = mountRich();
    await act(async () => { await result.current.handleCast(); });
    expect(getStreamInfoMock).toHaveBeenCalledWith("m1", expect.objectContaining({ supportedCodecs: ["h264"], supportsHdr: false, maxHeight: 1080, maxH264Height: 1080 }));
    expect(lastCastedUrl().searchParams.get("videoCodecs")).toBe("h264");
  });

  it("a session change mid-handshake drops that load; the old session's late answer is ignored", async () => {
    vi.useFakeTimers();
    connected();
    sessionDeviceModelName = richModel;
    const first = fakeCapsChannel();
    const second = fakeCapsChannel(() => report({ ...observed, videoCodecs: ["h264"] }));
    capsChannels = [first, second];
    const { result, rerender } = mountRich();
    let firstLoad: Promise<void> | undefined;
    await act(async () => {
      firstLoad = result.current.handleCast();
      await vi.advanceTimersByTimeAsync(1000);
    });
    // The receiver goes away, then a new one connects: the auto-load runs
    // the handshake again on the NEW session's channel.
    await act(async () => { castState = 0; rerender(); });
    await act(async () => { castState = 4; rerender(); await vi.advanceTimersByTimeAsync(100); });
    // The old session finally answers - on a channel already closed, and for
    // a generation that is gone: nothing may come of it.
    await act(async () => {
      first.push(report({ ...observed, videoCodecs: ["h264", "hevc", "vp9"], maxHeight: 2160, hdr: true }));
      await vi.advanceTimersByTimeAsync(5100);
      await firstLoad;
    });
    expect(first.closed).toBe(true);
    expect(second.closed).toBe(true);
    const loads = castClientInstance!.loadMedia.mock.calls;
    expect(loads, "one load: the new session's; the stale one was dropped").toHaveLength(1);
    expect(new URL(loads[0][0].mediaInfo.contentUrl).searchParams.get("videoCodecs")).toBe("h264");
    expect(getStreamInfoMock.mock.calls.every(([, p]: [string, { supportedCodecs: string[] }]) => !p.supportedCodecs.includes("hevc"))).toBe(true);
  });

  it("observed caps that contradict the model table win, HDR and Dolby included", async () => {
    connected();
    sessionDeviceModelName = "Chromecast Ultra"; // table: no Dolby, HDR, hevc
    capsChannels = [fakeCapsChannel(() => report({ ...observed, videoCodecs: ["h264", "hevc"], hdr: true, nativeDolby: true, maxHeight: 2160, h264MaxHeight: 2160 }))];
    const { result } = mountRich();
    await act(async () => { await result.current.handleCast(); });
    expect(getStreamInfoMock).toHaveBeenCalledWith("m1", expect.objectContaining({
      supportedCodecs: ["h264", "hevc"], supportedAudioCodecs: ["aac", "mp3", "opus", "flac", "eac3", "ac3"], supportsHdr: true, maxHeight: 2160, maxH264Height: 2160,
    }));
  });
});

// Live, 2026-08-31, 54 minutes into a film: a token refresh was refused,
// the app logged out, the saved credentials logged it back in, StartStream
// re-issued the stream URL - and the auto-load, keyed on that URL, re-LOADed
// the receiver, which interrupted the running film and restarted it from 0.
it("does not re-load the receiver when the stream URL is re-minted for the same media", async () => {
  castState = 4; // CONNECTED
  castClientInstance = { loadMedia: vi.fn().mockResolvedValue(undefined) };
  eveyFetchMock.mockResolvedValue({ token: "cast-tok", mediaId: "m1" });
  const player = fakePlayer();
  const args = (streamUrl: string) => ({
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    player: player as any, restoreLocalVolume: () => {}, streamUrl, mediaId: "m1", title: "A Film", currentTime: 0,
    fallbackDurationSec: 120, streamInfo: null, serverUrl: null,
  });
  const { rerender } = renderHook((p: ReturnType<typeof args>) => useCastSession(p), {
    initialProps: args("https://example.invalid/api/v1/media/m1/hls/master.m3u8?token=tok-1"),
  });
  await act(async () => {});
  expect(castClientInstance!.loadMedia).toHaveBeenCalledTimes(1);

  await act(async () => {
    rerender(args("https://example.invalid/api/v1/media/m1/hls/master.m3u8?token=tok-2"));
  });
  expect(castClientInstance!.loadMedia).toHaveBeenCalledTimes(1);
});

it("re-loads when the MEDIA changes, and resumes a re-cast of the same title where the television is", async () => {
  castState = 4; // CONNECTED
  castClientInstance = { loadMedia: vi.fn().mockResolvedValue(undefined) };
  eveyFetchMock.mockResolvedValue({ token: "cast-tok", mediaId: "m1" });
  const player = fakePlayer();
  const args = (mediaId: string) => ({
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    player: player as any, restoreLocalVolume: () => {}, streamUrl: `https://example.invalid/api/v1/media/${mediaId}/hls/master.m3u8?token=t`,
    mediaId, title: "A Film", currentTime: 0, fallbackDurationSec: 120, streamInfo: null, serverUrl: null,
  });
  const { result, rerender } = renderHook((p: ReturnType<typeof args>) => useCastSession(p), { initialProps: args("m1") });
  await act(async () => {});
  expect(castClientInstance!.loadMedia).toHaveBeenCalledTimes(1);

  // The receiver has reached 54 minutes; the user presses cast again.
  castMediaStatus = { streamPosition: 3240 };
  await act(async () => {
    rerender(args("m1"));
  });
  await act(async () => {
    await result.current.handleCast();
  });
  const calls = castClientInstance!.loadMedia.mock.calls;
  expect(calls).toHaveLength(2);
  expect(calls[1][0].startTime).toBe(3240);

  // A different title is a new load, from the local player's position (0).
  await act(async () => {
    rerender(args("m2"));
  });
  expect(castClientInstance!.loadMedia).toHaveBeenCalledTimes(3);
  expect(castClientInstance!.loadMedia.mock.calls[2][0].startTime).toBe(0);
});

// Stopping a cast hands the film back to the phone where the television got to:
// the local player sat parked at the cast's start, and resuming from there
// rewound the viewer by everything they had just watched.
it("puts the local player at the television's position when the cast ends", async () => {
  castState = 4; // CONNECTED
  castClientInstance = { loadMedia: vi.fn().mockResolvedValue(undefined) };
  eveyFetchMock.mockResolvedValue({ token: "cast-tok", mediaId: "m1" });
  const player = { ...fakePlayer(), currentTime: 0 };
  const { rerender } = mount(player);
  await act(async () => {});

  castMediaStatus = { streamPosition: 1830 };
  await act(async () => { rerender(); });
  expect(player.currentTime).toBe(0);

  castState = 0; // the user stopped the cast
  castClientInstance = null;
  castMediaStatus = null;
  await act(async () => { rerender(); });
  expect(player.currentTime).toBe(1830);

  // Once handed back, the position is spent: a later disconnect-shaped render
  // must not yank the viewer back to 1830 after they moved on locally.
  player.currentTime = 1900;
  await act(async () => { rerender(); });
  expect(player.currentTime).toBe(1900);
});

// A receiver that stops answering must leave a trace from the SENDER's side:
// the 2026-08-31 death left the phone spinning and the logs silent.
describe("castIsPlaying", () => {
  // react-native-google-cast serialises playerState as a string; comparing it
  // to the Cast SDK's integer left the controller on "play" for a whole film.
  it("follows the receiver's string player state", () => {
    eveyFetchMock.mockResolvedValue({ token: "cast-tok", mediaId: "m1" });
    castState = 4;
    castClientInstance = { loadMedia: vi.fn().mockResolvedValue(undefined) };
    castMediaStatus = { streamPosition: 10, playerState: "playing" };
    const { result, rerender } = mount(fakePlayer());
    expect(result.current.castIsPlaying).toBe(true);
    castMediaStatus = { streamPosition: 10, playerState: "paused" };
    rerender();
    expect(result.current.castIsPlaying).toBe(false);
  });
});

describe("cast_sender_stall beacon", () => {
  function stallBeacons() {
    return eveyFetchMock.mock.calls
      .map((c) => c[0] as { data?: { type?: string; details?: string } })
      .filter((r) => r.data?.type === "cast_sender_stall")
      .map((r) => r.data?.details);
  }

  it("reports once when a PLAYING position stays frozen for the stall window", () => {
    vi.useFakeTimers();
    try {
      eveyFetchMock.mockResolvedValue({ token: "cast-tok", mediaId: "m1" });
      castState = 4;
      castClientInstance = { loadMedia: vi.fn().mockResolvedValue(undefined) };
      castMediaStatus = { streamPosition: 3266, playerState: "playing" };
      const player = fakePlayer();
      mount(player);
      act(() => { vi.advanceTimersByTime(15_000); });
      expect(stallBeacons()).toEqual([]);
      act(() => { vi.advanceTimersByTime(30_000); });
      expect(stallBeacons()).toHaveLength(1);
      expect(stallBeacons()[0]).toMatch(/^state=playing frozen=\d+s pos=3266$/);
      act(() => { vi.advanceTimersByTime(60_000); });
      expect(stallBeacons()).toHaveLength(1);
    } finally {
      vi.useRealTimers();
    }
  });

  it("reports an IDLE receiver with its reason", () => {
    eveyFetchMock.mockResolvedValue({ token: "cast-tok", mediaId: "m1" });
    castState = 4;
    castClientInstance = { loadMedia: vi.fn().mockResolvedValue(undefined) };
    castMediaStatus = { streamPosition: 120, playerState: "idle", idleReason: "ERROR" };
    mount(fakePlayer());
    expect(stallBeacons()).toEqual(["state=idle reason=ERROR pos=120"]);
  });

  it("stays silent while the position moves", () => {
    vi.useFakeTimers();
    try {
      eveyFetchMock.mockResolvedValue({ token: "cast-tok", mediaId: "m1" });
      castState = 4;
      castClientInstance = { loadMedia: vi.fn().mockResolvedValue(undefined) };
      castMediaStatus = { streamPosition: 10, playerState: "playing" };
      const { rerender } = mount(fakePlayer());
      for (let i = 1; i <= 10; i++) {
        castMediaStatus = { streamPosition: 10 + i * 5, playerState: "playing" };
        act(() => { vi.advanceTimersByTime(5_000); });
        rerender();
      }
      expect(stallBeacons()).toEqual([]);
    } finally {
      vi.useRealTimers();
    }
  });
});
