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
  send() {}
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
