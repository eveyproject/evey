import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";

// Controllable WebSocket fake - the seam is globalThis.WebSocket, as in the
// mobile twin of this test.
interface FakeWS {
  url: string;
  protocols: string[];
  onopen: ((e: Event) => void) | null;
  onclose: ((e: CloseEvent) => void) | null;
  onmessage: ((e: MessageEvent) => void) | null;
  onerror: ((e: Event) => void) | null;
  send: ReturnType<typeof vi.fn>;
  close: ReturnType<typeof vi.fn>;
}

let lastWS: FakeWS | null = null;
let wsInstances: FakeWS[] = [];

class MockWebSocket {
  url: string;
  protocols: string[];
  onopen: ((e: Event) => void) | null = null;
  onclose: ((e: CloseEvent) => void) | null = null;
  onmessage: ((e: MessageEvent) => void) | null = null;
  onerror: ((e: Event) => void) | null = null;
  send = vi.fn();
  close = vi.fn();

  constructor(url: string, protocols?: string | string[]) {
    this.url = url;
    this.protocols = Array.isArray(protocols) ? protocols : protocols ? [protocols] : [];
    lastWS = this as unknown as FakeWS;
    wsInstances.push(lastWS);
  }

  simulateOpen() {
    this.onopen?.({} as Event);
  }

  simulateMessage(data: object) {
    this.onmessage?.({ data: JSON.stringify(data) } as MessageEvent);
  }

  simulateClose() {
    this.onclose?.({} as CloseEvent);
  }
}

function ws(): MockWebSocket {
  if (!lastWS) throw new Error("No WebSocket instantiated yet");
  return lastWS as unknown as MockWebSocket;
}

const pushMock = vi.fn();
vi.mock("expo-router", () => ({
  useRouter: () => ({ push: pushMock }),
}));

let mockAccessToken: string | null = "test-token";
let mockServerUrl: string | undefined = "https://tv.example";
vi.mock("@evey/store", () => ({
  useAuthStore: (selector: (s: { accessToken: string | null }) => unknown) =>
    selector({ accessToken: mockAccessToken }),
  useServerListStore: (
    selector: (s: { getActiveServer: () => { url: string } | undefined }) => unknown,
  ) => selector({ getActiveServer: () => (mockServerUrl ? { url: mockServerUrl } : undefined) }),
}));

const getItemAsyncMock = vi.fn();
const setItemAsyncMock = vi.fn();
vi.mock("expo-secure-store", () => ({
  getItemAsync: (...args: unknown[]) => getItemAsyncMock(...args),
  setItemAsync: (...args: unknown[]) => setItemAsyncMock(...args),
}));

vi.mock("react-native", () => ({
  Platform: { OS: "android", isTV: true },
}));

import { useDeviceRegistration } from "./useDeviceRegistration";

beforeEach(() => {
  vi.clearAllMocks();
  vi.useRealTimers();
  lastWS = null;
  wsInstances = [];
  mockAccessToken = "test-token";
  mockServerUrl = "https://tv.example";
  getItemAsyncMock.mockResolvedValue("device-tv-1");
  (globalThis as unknown as Record<string, unknown>).WebSocket = MockWebSocket;
});

// The TV registers itself as a "Play On" target: same socket and auth as
// the phone, but it announces deviceType=tv so the picker labels it right,
// and a loadAndPlay command lands on the player route.
describe("useDeviceRegistration (tv)", () => {
  it("does not connect without a token or without an active server", async () => {
    mockAccessToken = null;
    renderHook(() => useDeviceRegistration());
    await act(async () => {});
    expect(lastWS).toBeNull();

    mockAccessToken = "test-token";
    mockServerUrl = undefined;
    renderHook(() => useDeviceRegistration());
    await act(async () => {});
    expect(lastWS).toBeNull();
  });

  it("derives wss from an https server and authenticates via the Bearer subprotocol", async () => {
    renderHook(() => useDeviceRegistration());
    await act(async () => {});
    expect(ws().url).toBe("wss://tv.example/api/v1/devices/ws");
    expect(ws().protocols).toEqual(["Bearer.test-token"]);
  });

  it("registers as an Android TV with the persisted device id on open", async () => {
    renderHook(() => useDeviceRegistration());
    await act(async () => {});
    act(() => { ws().simulateOpen(); });

    expect(ws().send).toHaveBeenCalledTimes(1);
    expect(JSON.parse(ws().send.mock.calls[0][0] as string)).toEqual({
      type: "register",
      deviceId: "device-tv-1",
      name: "Android TV",
      deviceType: "tv",
    });
  });

  it("mints and persists a device id once when none is stored", async () => {
    getItemAsyncMock.mockResolvedValue(null);
    renderHook(() => useDeviceRegistration());
    await act(async () => {});
    act(() => { ws().simulateOpen(); });

    expect(setItemAsyncMock).toHaveBeenCalledWith("evey-device-id", expect.any(String));
    const frame = JSON.parse(ws().send.mock.calls[0][0] as string);
    expect(frame.deviceId.length).toBeGreaterThan(0);
  });

  it("routes loadAndPlay to the player, forwarding a positive position only", async () => {
    renderHook(() => useDeviceRegistration());
    await act(async () => {});
    act(() => { ws().simulateOpen(); });

    act(() => { ws().simulateMessage({ type: "loadAndPlay", mediaId: "media-42", positionMs: 12000 }); });
    expect(pushMock).toHaveBeenLastCalledWith({
      pathname: "/(auth)/player/[id]",
      params: { id: "media-42", positionMs: "12000" },
    });

    act(() => { ws().simulateMessage({ type: "loadAndPlay", mediaId: "media-7", positionMs: 0 }); });
    expect(pushMock).toHaveBeenLastCalledWith({
      pathname: "/(auth)/player/[id]",
      params: { id: "media-7" },
    });
  });

  it("ignores play/pause/seek, a loadAndPlay without media id, and malformed frames", async () => {
    renderHook(() => useDeviceRegistration());
    await act(async () => {});
    act(() => { ws().simulateOpen(); });

    expect(() => {
      act(() => { ws().simulateMessage({ type: "pause" }); });
      act(() => { ws().simulateMessage({ type: "loadAndPlay" }); });
      act(() => { ws().onmessage?.({ data: "not-json{{" } as MessageEvent); });
    }).not.toThrow();
    expect(pushMock).not.toHaveBeenCalled();
  });

  it("reconnects with backoff after a close, and not after unmount", async () => {
    vi.useFakeTimers();
    const { unmount } = renderHook(() => useDeviceRegistration());
    await act(async () => { await vi.runOnlyPendingTimersAsync(); });
    act(() => { ws().simulateOpen(); });

    act(() => { ws().simulateClose(); });
    await act(async () => { await vi.advanceTimersByTimeAsync(1500); });
    expect(wsInstances.length).toBe(2);

    const opened = wsInstances.length;
    unmount();
    expect(ws().close).toHaveBeenCalled();
    await act(async () => { await vi.advanceTimersByTimeAsync(60_000); });
    expect(wsInstances.length).toBe(opened);
    vi.useRealTimers();
  });
});
