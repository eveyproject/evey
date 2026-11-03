import { renderHook, act } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { useAuthStore } from "@evey/store";
import { useDeviceRegistrationCore } from "./deviceRegistrationCore";

class FakeSocket {
  static instances: FakeSocket[] = [];
  onopen: (() => void) | null = null;
  onmessage: unknown = null;
  onerror: unknown = null;
  onclose: (() => void) | null = null;
  constructor(public url: string) {
    FakeSocket.instances.push(this);
  }
  sent: string[] = [];
  send(frame: string) { this.sent.push(frame); }
  close() {}
}

// A JWT whose exp is offsetSec from now: the socket reads only its exp.
function jwt(offsetSec: number): string {
  const payload = btoa(JSON.stringify({ exp: Math.floor(Date.now() / 1000) + offsetSec }));
  return `h.${payload}.s`;
}

async function flush() {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0);
  });
}

describe("useDeviceRegistrationCore", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    FakeSocket.instances = [];
    vi.stubGlobal("WebSocket", FakeSocket);
    // A day of validity: the backoff test advances the fake clock an hour.
    useAuthStore.setState({ accessToken: jwt(86_400) } as never);
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  function mountTv() {
    return renderHook(() =>
      useDeviceRegistrationCore({
        deviceType: "tv",
        deviceName: () => "d",
        serverUrl: "http://srv",
        pauseInBackground: false,
        onLoadAndPlay: vi.fn(),
        getDeviceId: async () => "id",
      }),
    );
  }

  // The server drops a socket silent for two minutes; without a client
  // keepalive every device reconnected on that cadence, and a server restart
  // had them all come back in step.
  it("pings an open socket on a jittered cadence so the server never times it out", async () => {
    vi.spyOn(Math, "random").mockReturnValue(0.5);
    const { unmount } = mountTv();
    await flush();
    const sock = FakeSocket.instances[0];
    act(() => sock.onopen?.());
    expect(sock.sent).toHaveLength(1);
    await act(async () => { await vi.advanceTimersByTimeAsync(44_000); });
    expect(sock.sent).toHaveLength(1);
    await act(async () => { await vi.advanceTimersByTimeAsync(1_000); });
    expect(sock.sent).toHaveLength(2);
    expect(JSON.parse(sock.sent[1])).toEqual({ type: "ping" });
    await act(async () => { await vi.advanceTimersByTimeAsync(45_000); });
    expect(sock.sent).toHaveLength(3);
    unmount();
    await act(async () => { await vi.advanceTimersByTimeAsync(90_000); });
    expect(sock.sent).toHaveLength(3);
  });

  it("spreads the ping cadence by a fifth either way", async () => {
    vi.spyOn(Math, "random").mockReturnValue(0);
    const { unmount } = mountTv();
    await flush();
    const sock = FakeSocket.instances[0];
    act(() => sock.onopen?.());
    await act(async () => { await vi.advanceTimersByTimeAsync(35_000); });
    expect(sock.sent).toHaveLength(1);
    await act(async () => { await vi.advanceTimersByTimeAsync(1_000); });
    expect(sock.sent).toHaveLength(2);
    unmount();
  });

  it("jitters the reconnect delay so restarted devices do not come back in step", async () => {
    vi.spyOn(Math, "random").mockReturnValue(1);
    const { unmount } = mountTv();
    await flush();
    act(() => FakeSocket.instances[0].onclose?.());
    await act(async () => { await vi.advanceTimersByTimeAsync(1_000); });
    expect(FakeSocket.instances).toHaveLength(1);
    await act(async () => { await vi.advanceTimersByTimeAsync(200); });
    expect(FakeSocket.instances).toHaveLength(2);
    unmount();
  });

  it("reconnects at once on return to foreground even after the socket looked unreachable", async () => {
    let notify: (bg: boolean) => void = () => {};
    const { unmount } = renderHook(() =>
      useDeviceRegistrationCore({
        deviceType: "mobile",
        deviceName: () => "d",
        serverUrl: "http://srv",
        pauseInBackground: true,
        onLoadAndPlay: vi.fn(),
        getDeviceId: async () => "id",
        subscribeBackground: (cb) => {
          notify = cb;
          return () => {};
        },
      }),
    );
    await flush();
    // Six opens that never succeed push the socket into the long retry cadence.
    for (let i = 0; i < 6; i++) {
      const sock = FakeSocket.instances[FakeSocket.instances.length - 1];
      act(() => sock.onclose?.());
      await act(async () => {
        await vi.advanceTimersByTimeAsync(11 * 60_000);
      });
    }
    act(() => notify(true));
    const before = FakeSocket.instances.length;
    act(() => notify(false));
    await flush();
    expect(FakeSocket.instances.length).toBe(before + 1);
    unmount();
  });

  // Back from hours in the background, the held token is long expired: a
  // socket opened with it is a 401 handshake. It waits for the resume
  // refresh, whose new token restarts the effect and connects.
  it("waits for a live token instead of opening a socket with an expired one", async () => {
    useAuthStore.setState({ accessToken: jwt(-3600) } as never);
    const { unmount } = renderHook(() =>
      useDeviceRegistrationCore({
        deviceType: "tv",
        deviceName: () => "d",
        serverUrl: "http://srv",
        pauseInBackground: false,
        onLoadAndPlay: vi.fn(),
        getDeviceId: async () => "id",
      }),
    );
    await flush();
    expect(FakeSocket.instances).toHaveLength(0);

    act(() => useAuthStore.setState({ accessToken: jwt(900) } as never));
    await flush();
    expect(FakeSocket.instances).toHaveLength(1);
    unmount();
  });
});
