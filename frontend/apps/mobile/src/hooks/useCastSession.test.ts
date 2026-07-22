import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";

const showCastDialog = vi.fn();
let castState = 0; // NOT_CONNECTED

vi.mock("./useChromecast", () => ({
  CastContext: { showCastDialog: () => showCastDialog() },
  CastStateEnum: { CONNECTED: 4 },
  useCastStateHook: () => castState,
  useRemoteMediaClientHook: () => null,
  useMediaStatusHook: () => null,
}));

const setAudioBoost = vi.fn();
vi.mock("../lib/audioBoost", () => ({ setAudioBoost: (v: number) => setAudioBoost(v) }));

import { useCastSession } from "./useCastSession";

function fakePlayer() {
  return { pause: vi.fn(), play: vi.fn(), volume: 1 };
}

function mount(player: ReturnType<typeof fakePlayer>) {
  return renderHook(() =>
    useCastSession({
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      player: player as any,
      volume: 1,
      streamUrl: "https://example.invalid/s.m3u8",
      mediaId: "m1",
      title: "A Film",
      currentTime: 0,
      fallbackDurationSec: 120,
    }),
  );
}

beforeEach(() => {
  castState = 0;
  showCastDialog.mockReset();
  setAudioBoost.mockReset();
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
});
