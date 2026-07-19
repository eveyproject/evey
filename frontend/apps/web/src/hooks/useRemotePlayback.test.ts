import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act, waitFor } from "@testing-library/react";

import { useRemotePlayback } from "./useRemotePlayback";

// A minimal stand-in for HTMLMediaElement.remote (the Remote Playback API).
// watchAvailability resolves to a callback id and invokes the callback with
// the current availability; the test drives it to flip availability later.
// Connection events are delivered through addEventListener like the real API.
function fakeVideoWithRemote(state = "disconnected") {
  let availabilityCb: ((available: boolean) => void) | null = null;
  const listeners = new Map<string, Set<() => void>>();
  const remote = {
    state,
    addEventListener: (name: string, cb: () => void) => {
      if (!listeners.has(name)) listeners.set(name, new Set());
      listeners.get(name)!.add(cb);
    },
    removeEventListener: (name: string, cb: () => void) => {
      listeners.get(name)?.delete(cb);
    },
    watchAvailability: vi.fn(async (cb: (a: boolean) => void) => {
      availabilityCb = cb;
      cb(false); // no device yet
      return 7; // callback id
    }),
    cancelWatchAvailability: vi.fn(async () => {}),
    prompt: vi.fn(async () => {}),
  };
  const video = { remote } as unknown as HTMLVideoElement;
  const fire = (name: string) => listeners.get(name)?.forEach((cb) => cb());
  return { video, remote, flip: (a: boolean) => availabilityCb?.(a), fire };
}

describe("useRemotePlayback", () => {
  beforeEach(() => vi.clearAllMocks());

  it("stays unavailable when the element has no Remote Playback API (desktop)", () => {
    const video = {} as HTMLVideoElement; // no `remote`
    const { result } = renderHook(() => useRemotePlayback(video));
    expect(result.current.available).toBe(false);
    expect(result.current.supported).toBe(false);
  });

  it("is unavailable until watchAvailability reports a device, then flips", async () => {
    const { video, flip } = fakeVideoWithRemote();
    const { result } = renderHook(() => useRemotePlayback(video));

    expect(result.current.available).toBe(false);
    act(() => flip(true));
    await waitFor(() => expect(result.current.available).toBe(true));
    act(() => flip(false));
    await waitFor(() => expect(result.current.available).toBe(false));
  });

  it("cancels the availability watch on unmount", async () => {
    const { video, remote } = fakeVideoWithRemote();
    const { unmount } = renderHook(() => useRemotePlayback(video));
    await waitFor(() => expect(remote.watchAvailability).toHaveBeenCalled());
    unmount();
    await waitFor(() => expect(remote.cancelWatchAvailability).toHaveBeenCalledWith(7));
  });

  it("swallows a watchAvailability rejection (disableRemotePlayback set)", async () => {
    const remote = {
      state: "disconnected",
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      watchAvailability: vi.fn(async () => { throw new Error("InvalidStateError"); }),
      cancelWatchAvailability: vi.fn(),
      prompt: vi.fn(),
    };
    const video = { remote } as unknown as HTMLVideoElement;
    const { result } = renderHook(() => useRemotePlayback(video));
    // No throw, stays unavailable.
    await waitFor(() => expect(remote.watchAvailability).toHaveBeenCalled());
    expect(result.current.available).toBe(false);
  });

  it("promptForUrl sets the element src to the castable URL, THEN prompts", async () => {
    // This is how HLS casts on Android: the receiver flings the URL we put on
    // the element. The order matters - src must be set before prompt() so the
    // picker offers to load THAT media.
    const { video, remote } = fakeVideoWithRemote();
    const order: string[] = [];
    Object.defineProperty(video, "src", {
      configurable: true,
      set: () => order.push("set-src"),
      get: () => "",
    });
    remote.prompt.mockImplementation(async () => { order.push("prompt"); });

    const { result } = renderHook(() => useRemotePlayback(video));
    await act(async () => { await result.current.promptForUrl("https://x/master.m3u8?token=t"); });

    expect(order).toEqual(["set-src", "prompt"]);
  });

  it("promptForUrl distinguishes handoff, dismissal, and real failure", async () => {
    // The caller pauses the local player only on a REAL handoff, stays silent
    // on a dismissal, and surfaces an error toast on anything else - a picker
    // cancel must not pause the film, and a genuine failure must not be mute.
    const { video, remote } = fakeVideoWithRemote();
    const { result } = renderHook(() => useRemotePlayback(video));

    await act(async () => {
      await expect(result.current.promptForUrl("https://x/m.m3u8")).resolves.toBe("connected");
    });

    const dismissed = new DOMException("dismissed", "NotAllowedError");
    remote.prompt.mockRejectedValueOnce(dismissed);
    await act(async () => {
      await expect(result.current.promptForUrl("https://x/m.m3u8")).resolves.toBe("dismissed");
    });

    remote.prompt.mockRejectedValueOnce(new DOMException("boom", "InvalidStateError"));
    await act(async () => {
      await expect(result.current.promptForUrl("https://x/m.m3u8")).resolves.toBe("error");
    });
  });

  it("tracks the connection through connect/disconnect events", async () => {
    const { video, remote, fire } = fakeVideoWithRemote();
    const { result } = renderHook(() => useRemotePlayback(video));
    expect(result.current.connected).toBe(false);

    remote.state = "connected";
    act(() => fire("connect"));
    await waitFor(() => expect(result.current.connected).toBe(true));

    remote.state = "disconnected";
    act(() => fire("disconnect"));
    await waitFor(() => expect(result.current.connected).toBe(false));
  });

  it("stop() disconnects by toggling disableRemotePlayback", async () => {
    // The spec exposes no disconnect() - flipping disableRemotePlayback runs
    // the "disable remote playback steps", which tears the connection down.
    const { video } = fakeVideoWithRemote("connected");
    const flags: boolean[] = [];
    Object.defineProperty(video, "disableRemotePlayback", {
      configurable: true,
      set: (v: boolean) => flags.push(v),
      get: () => flags[flags.length - 1] ?? false,
    });
    const { result } = renderHook(() => useRemotePlayback(video));
    await act(async () => { await result.current.stop(); });
    expect(flags).toEqual([true, false]);
  });
});
