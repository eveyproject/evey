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

  // Regression, measured in production: the tap handler awaited a token fetch
  // before prompting, which burned the tap's transient user activation. Chrome
  // rejected every attempt with NotAllowedError - 13 taps, 13 instant
  // rejections, no picker ever shown. prompt() therefore touches NOTHING: the
  // element already carries the castable URL, so the caller can reach it with
  // no await in between.
  it("prompt() goes straight to the picker without touching the element src", async () => {
    const { video, remote } = fakeVideoWithRemote();
    const order: string[] = [];
    Object.defineProperty(video, "src", {
      configurable: true,
      set: () => order.push("set-src"),
      get: () => "https://x/master.m3u8?token=cast",
    });
    remote.prompt.mockImplementation(async () => { order.push("prompt"); });

    const { result } = renderHook(() => useRemotePlayback(video));
    await act(async () => { await result.current.prompt(); });

    expect(order).toEqual(["prompt"]);
  });

  it("prompt() hands the outcome back so the caller can react honestly", async () => {
    const { video, remote } = fakeVideoWithRemote();
    const { result } = renderHook(() => useRemotePlayback(video));

    remote.prompt.mockResolvedValueOnce(undefined);
    await act(async () => { expect(await result.current.prompt()).toBe("connected"); });

    remote.prompt.mockRejectedValueOnce(Object.assign(new Error("x"), { name: "NotAllowedError" }));
    await act(async () => { expect(await result.current.prompt()).toBe("dismissed"); });

    remote.prompt.mockRejectedValueOnce(Object.assign(new Error("x"), { name: "NotFoundError" }));
    await act(async () => { expect(await result.current.prompt()).toBe("error"); });
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

  // Telemetry. A cast that fails on the phone leaves ZERO server-side trace:
  // the fling is phone-to-TV, and if the picker never yields a device the
  // server sees nothing at all. Diagnosing the 2026-07-20 reports meant
  // reading tea leaves. These tests pin the beacon that makes an attempt and
  // its outcome show up in the admin logs.
  describe("cast telemetry", () => {
    it("reports the attempt with the availability the phone actually saw", async () => {
      const { video, flip } = fakeVideoWithRemote();
      const report = vi.fn();
      const { result } = renderHook(() => useRemotePlayback(video, report));

      act(() => flip(true));
      await waitFor(() => expect(result.current.available).toBe(true));
      await act(async () => { await result.current.promptForUrl("https://x/m.m3u8?token=secret"); });

      const attempt = report.mock.calls.map((c) => c[0]).find((e) => e.type === "cast_prompt");
      expect(attempt).toBeDefined();
      // "did his TV show up?" - the single fact no log could answer before.
      expect(attempt.details).toContain("available=true");
      expect(attempt.details).toContain("supported=true");
    });

    it("reports available=false when no device was ever seen", async () => {
      const { video } = fakeVideoWithRemote(); // watchAvailability reports false
      const report = vi.fn();
      const { result } = renderHook(() => useRemotePlayback(video, report));
      await act(async () => { await result.current.promptForUrl("https://x/m.m3u8"); });

      const attempt = report.mock.calls.map((c) => c[0]).find((e) => e.type === "cast_prompt");
      expect(attempt.details).toContain("available=false");
    });

    it("reports the outcome: handoff, dismissal, and the real error name", async () => {
      const { video, remote } = fakeVideoWithRemote();
      const report = vi.fn();
      const { result } = renderHook(() => useRemotePlayback(video, report));
      const outcomes = () =>
        report.mock.calls.map((c) => c[0]).filter((e) => e.type === "cast_prompt_result");

      await act(async () => { await result.current.promptForUrl("https://x/m.m3u8"); });
      expect(outcomes().at(-1).details).toBe("connected");

      remote.prompt.mockRejectedValueOnce(new DOMException("dismissed", "NotAllowedError"));
      await act(async () => { await result.current.promptForUrl("https://x/m.m3u8"); });
      expect(outcomes().at(-1).details).toBe("dismissed");

      // The DOMException NAME is the diagnostic payload: NotSupportedError
      // (source not flingable) and NotFoundError (no device took it) are
      // completely different bugs, and the old empty catch erased both.
      remote.prompt.mockRejectedValueOnce(new DOMException("boom", "NotSupportedError"));
      await act(async () => { await result.current.promptForUrl("https://x/m.m3u8"); });
      expect(outcomes().at(-1).details).toBe("error:NotSupportedError");
      expect(outcomes().at(-1).fatal).toBe(true);
    });

    it("NEVER puts the cast URL or its token in the telemetry", async () => {
      // The cast token is a 12-hour bearer credential for the media. Beacons
      // land in the admin logs, which are read and pasted around - a token in
      // there is a leak, not a diagnostic.
      const { video, remote } = fakeVideoWithRemote();
      const report = vi.fn();
      const { result } = renderHook(() => useRemotePlayback(video, report));
      remote.prompt.mockRejectedValueOnce(new DOMException("boom", "InvalidStateError"));
      await act(async () => {
        await result.current.promptForUrl("https://evey.example.net/hls/master.m3u8?token=SEKRIT-9f2a");
      });

      // Guard the guard: without this the assertions below pass vacuously
      // when nothing is reported at all, and the test proves nothing.
      expect(report).toHaveBeenCalled();
      const blob = JSON.stringify(report.mock.calls);
      expect(blob).not.toContain("SEKRIT-9f2a");
      expect(blob).not.toContain("token=");
      expect(blob).not.toContain("master.m3u8");
    });

    it("works without a reporter (telemetry is optional, never required)", async () => {
      const { video } = fakeVideoWithRemote();
      const { result } = renderHook(() => useRemotePlayback(video));
      await act(async () => {
        await expect(result.current.promptForUrl("https://x/m.m3u8")).resolves.toBe("connected");
      });
    });
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
