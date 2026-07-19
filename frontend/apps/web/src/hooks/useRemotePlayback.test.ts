import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act, waitFor } from "@testing-library/react";

import { useRemotePlayback } from "./useRemotePlayback";

// A minimal stand-in for HTMLMediaElement.remote (the Remote Playback API).
// watchAvailability resolves to a callback id and invokes the callback with
// the current availability; the test drives it to flip availability later.
function fakeVideoWithRemote() {
  let availabilityCb: ((available: boolean) => void) | null = null;
  const remote = {
    watchAvailability: vi.fn(async (cb: (a: boolean) => void) => {
      availabilityCb = cb;
      cb(false); // no device yet
      return 7; // callback id
    }),
    cancelWatchAvailability: vi.fn(async () => {}),
    prompt: vi.fn(async () => {}),
  };
  const video = { remote } as unknown as HTMLVideoElement;
  return { video, remote, flip: (a: boolean) => availabilityCb?.(a) };
}

describe("useRemotePlayback", () => {
  beforeEach(() => vi.clearAllMocks());

  it("stays unavailable when the element has no Remote Playback API (desktop)", () => {
    const video = {} as HTMLVideoElement; // no `remote`
    const { result } = renderHook(() => useRemotePlayback(video));
    expect(result.current.available).toBe(false);
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

  it("prompt() opens the native picker via the element's remote", async () => {
    const { video, remote } = fakeVideoWithRemote();
    const { result } = renderHook(() => useRemotePlayback(video));
    await act(async () => { await result.current.prompt(); });
    expect(remote.prompt).toHaveBeenCalledTimes(1);
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

  it("cancels the availability watch on unmount", async () => {
    const { video, remote } = fakeVideoWithRemote();
    const { unmount } = renderHook(() => useRemotePlayback(video));
    await waitFor(() => expect(remote.watchAvailability).toHaveBeenCalled());
    unmount();
    await waitFor(() => expect(remote.cancelWatchAvailability).toHaveBeenCalledWith(7));
  });

  it("swallows a watchAvailability rejection (disableRemotePlayback set)", async () => {
    const remote = {
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
});
