import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { act, renderHook } from "@testing-library/react";

const eveyFetchMock = vi.fn();
vi.mock("@evey/api", () => ({ eveyFetch: (...args: unknown[]) => eveyFetchMock(...args) }));

const native = vi.hoisted(() => ({ appState: null as ((s: string) => void) | null }));
vi.mock("react-native", () => ({
  AppState: {
    addEventListener: (_e: string, cb: (s: string) => void) => {
      native.appState = cb;
      return { remove: () => (native.appState = null) };
    },
  },
}));

import { formatQoESample, useQoEBeacon, variantIndexOf, type QoEBeaconOptions } from "./useQoEBeacon";

// A player whose events the test fires by hand.
function fakePlayer() {
  const listeners = new Map<string, (ev: unknown) => void>();
  return {
    listeners,
    addListener: (event: string, cb: (ev: unknown) => void) => {
      listeners.set(event, cb);
      return { remove: () => listeners.delete(event) };
    },
    emit: (event: string, payload: unknown) => listeners.get(event)?.(payload),
  };
}

const ladder = [1_500_000, 4_500_000, 8_000_000];
const baseOptions: QoEBeaconOptions = {
  mediaId: "m 1",
  sessionId: "sess-1",
  format: "h",
  playing: { width: 1920, height: 1080, bitrate: 4_500_000 },
  ladderBitrates: ladder,
};

function sent(): string[] {
  return eveyFetchMock.mock.calls.map((c) => (c[0] as { data: { details: string } }).data.details);
}

// Brings a fake player to "playing, 12 s buffered ahead at 60 s".
function startPlaying(player: ReturnType<typeof fakePlayer>) {
  act(() => {
    player.emit("statusChange", { status: "readyToPlay", oldStatus: "loading" });
    player.emit("playingChange", { isPlaying: true, oldIsPlaying: false });
    player.emit("timeUpdate", { currentTime: 60, bufferedPosition: 72 });
  });
}

beforeEach(() => {
  vi.useFakeTimers();
  eveyFetchMock.mockReset().mockResolvedValue(undefined);
});
afterEach(() => {
  vi.useRealTimers();
});

describe("formatQoESample", () => {
  it("writes the CMCD keys the server's parser reads, with the flags last", () => {
    expect(formatQoESample({ bufferMs: 12_400.6, starved: true, startup: true, format: "h", sessionId: 'se"ss', playingBps: 4_500_000, topBps: 8_000_000 }))
      .toBe('bl=12401,sf=h,st=v,ot=v,v=1,br=4500,tb=8000,sid="sess",bs,su');
  });
  it("omits what it does not know", () => {
    expect(formatQoESample({ bufferMs: -3, starved: false, startup: false, format: "d", sessionId: null, playingBps: 0, topBps: 0 })).toBe("bl=0,sf=d,st=v,ot=v,v=1");
  });
});

describe("variantIndexOf", () => {
  it("is the rung's index on the ladder, 0 when the engine lists none", () => {
    expect(variantIndexOf({ bitrate: 8_000_000 }, ladder)).toBe(2);
    expect(variantIndexOf({ bitrate: 777 }, ladder)).toBe(0);
    expect(variantIndexOf(null, [])).toBe(0);
  });
});

describe("useQoEBeacon", () => {
  it("posts a qoe_sample every ten seconds while playing, the first one flagged startup", () => {
    const player = fakePlayer();
    renderHook(() => useQoEBeacon(player, baseOptions));
    startPlaying(player);
    act(() => vi.advanceTimersByTime(10_000));
    expect(eveyFetchMock).toHaveBeenCalledTimes(1);
    expect(eveyFetchMock.mock.calls[0][0]).toEqual({
      url: "/api/v1/media/m%201/player-event",
      method: "POST",
      data: { type: "qoe_sample", details: 'bl=12000,sf=h,st=v,ot=v,v=1,br=4500,tb=8000,sid="sess-1",su', fatal: false, code: 0, variant: 1, segment: 0, posSec: 60 },
    });
    act(() => {
      player.emit("timeUpdate", { currentTime: 70, bufferedPosition: 75.5 });
      vi.advanceTimersByTime(10_000);
    });
    expect(sent()[1]).toBe('bl=5500,sf=h,st=v,ot=v,v=1,br=4500,tb=8000,sid="sess-1"');
  });

  it("samples a rebuffer the moment it is seen, flagged starved, and only once", () => {
    const player = fakePlayer();
    renderHook(() => useQoEBeacon(player, baseOptions));
    startPlaying(player);
    act(() => {
      vi.advanceTimersByTime(10_000);
      // Past the minimum gap after the periodic sample, before the next one.
      vi.advanceTimersByTime(6_000);
      player.emit("statusChange", { status: "loading", oldStatus: "readyToPlay" });
    });
    expect(sent()).toHaveLength(2);
    expect(sent()[1]).toContain(",bs");
    expect(sent()[1]).toMatch(/^bl=0,/);
    // Recovered: the next periodic sample carries no flag.
    act(() => {
      player.emit("statusChange", { status: "readyToPlay", oldStatus: "loading" });
      player.emit("timeUpdate", { currentTime: 75, bufferedPosition: 90 });
      vi.advanceTimersByTime(10_000);
    });
    expect(sent()[2]).not.toContain("bs");
  });

  it("does not turn a flapping player into a beacon storm", () => {
    const player = fakePlayer();
    renderHook(() => useQoEBeacon(player, baseOptions));
    startPlaying(player);
    act(() => {
      for (let i = 0; i < 5; i++) {
        player.emit("statusChange", { status: "loading", oldStatus: "readyToPlay" });
        player.emit("statusChange", { status: "readyToPlay", oldStatus: "loading" });
        vi.advanceTimersByTime(500);
      }
    });
    expect(eveyFetchMock).toHaveBeenCalledTimes(1);
  });

  it("sends nothing before the player is ready, while paused, or in the background", () => {
    const player = fakePlayer();
    renderHook(() => useQoEBeacon(player, baseOptions));
    act(() => vi.advanceTimersByTime(30_000));
    expect(eveyFetchMock).not.toHaveBeenCalled();
    startPlaying(player);
    act(() => {
      player.emit("playingChange", { isPlaying: false, oldIsPlaying: true });
      vi.advanceTimersByTime(30_000);
    });
    expect(eveyFetchMock).not.toHaveBeenCalled();
    act(() => {
      player.emit("playingChange", { isPlaying: true, oldIsPlaying: false });
      native.appState?.("background");
      vi.advanceTimersByTime(30_000);
    });
    expect(eveyFetchMock).not.toHaveBeenCalled();
    act(() => {
      native.appState?.("active");
      vi.advanceTimersByTime(10_000);
    });
    expect(eveyFetchMock).toHaveBeenCalledTimes(1);
  });

  it("stays silent on direct play and local files (no format), and without a media", () => {
    const player = fakePlayer();
    renderHook(() => useQoEBeacon(player, { ...baseOptions, format: null }));
    expect(player.listeners.size).toBe(0);
    renderHook(() => useQoEBeacon(player, { ...baseOptions, mediaId: "" }));
    expect(player.listeners.size).toBe(0);
  });

  it("unsubscribes from the player on unmount", () => {
    const player = fakePlayer();
    const { unmount } = renderHook(() => useQoEBeacon(player, baseOptions));
    expect(player.listeners.size).toBe(3);
    unmount();
    expect(player.listeners.size).toBe(0);
    expect(native.appState).toBeNull();
  });
});
