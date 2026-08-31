import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";

const showCastDialog = vi.fn();
let castState = 0; // NOT_CONNECTED
let castClientInstance: { loadMedia: ReturnType<typeof vi.fn> } | null = null;
let castDeviceModelName: string | null = null;
let castDeviceFriendlyName: string | null = null;
let sessionDeviceModelName: string | null = null;
let castMediaStatus: { streamPosition?: number } | null = null;

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
vi.mock("@evey/api", () => ({
  eveyFetch: (...args: unknown[]) => eveyFetchMock(...args),
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
  opts: { streamInfo?: any; serverUrl?: string | null } = {},
) {
  return renderHook(() =>
    useCastSession({
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      player: player as any,
      volume: 1,
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
  ensureNotificationPermission.mockClear();
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

  // With streamInfo/serverUrl available, the URL is re-negotiated against the
  // RECEIVER's profile (via buildCastStreamUrl) instead of just swapping the
  // token into the phone-negotiated URL - a Chromecast gen 1-3 must never see
  // the hevc the phone itself advertised.
  it("negotiates the receiver's profile from the connected device's model name", async () => {
    castState = 4; // CONNECTED
    castDeviceModelName = "Chromecast"; // gen 1-3 floor: h264 only, no hevc
    castClientInstance = { loadMedia: vi.fn().mockResolvedValue(undefined) };
    eveyFetchMock.mockResolvedValue({ token: "cast-tok-999", mediaId: "m1" });

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
    player: player as any, volume: 1, streamUrl, mediaId: "m1", title: "A Film", currentTime: 0,
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
    player: player as any, volume: 1, streamUrl: `https://example.invalid/api/v1/media/${mediaId}/hls/master.m3u8?token=t`,
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
