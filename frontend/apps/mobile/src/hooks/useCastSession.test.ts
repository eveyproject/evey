import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";

const showCastDialog = vi.fn();
let castState = 0; // NOT_CONNECTED
let castClientInstance: { loadMedia: ReturnType<typeof vi.fn> } | null = null;

vi.mock("./useChromecast", () => ({
  CastContext: { showCastDialog: () => showCastDialog() },
  CastStateEnum: { CONNECTED: 4 },
  useCastStateHook: () => castState,
  useRemoteMediaClientHook: () => castClientInstance,
  useMediaStatusHook: () => null,
}));

const setAudioBoost = vi.fn();
vi.mock("../lib/audioBoost", () => ({ setAudioBoost: (v: number) => setAudioBoost(v) }));

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

function mount(player: ReturnType<typeof fakePlayer>, streamUrl = "https://example.invalid/s.m3u8") {
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
    }),
  );
}

beforeEach(() => {
  castState = 0;
  castClientInstance = null;
  showCastDialog.mockReset();
  setAudioBoost.mockReset();
  eveyFetchMock.mockReset();
});

describe("useCastSession", () => {
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
});
