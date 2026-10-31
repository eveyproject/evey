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
    useAuthStore.setState({ accessToken: "tok" } as never);
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
});
