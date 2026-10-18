import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";

// ---------------------------------------------------------------------------
// Mock WebSocket with a controllable fake - seam is globalThis.WebSocket.
// Mirrors useSyncPlay.test.ts's fake.
// ---------------------------------------------------------------------------

interface FakeWS {
  url: string;
  protocols: string[];
  onopen: ((e: Event) => void) | null;
  onclose: ((e: CloseEvent) => void) | null;
  onmessage: ((e: MessageEvent) => void) | null;
  onerror: ((e: Event) => void) | null;
  readyState: number;
  send: ReturnType<typeof vi.fn>;
  close: ReturnType<typeof vi.fn>;
}

let lastWS: FakeWS | null = null;
let wsInstances: FakeWS[] = [];

class MockWebSocket {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSING = 2;
  static readonly CLOSED = 3;

  readonly CONNECTING = 0;
  readonly OPEN = 1;
  readonly CLOSING = 2;
  readonly CLOSED = 3;

  url: string;
  protocols: string[];
  onopen: ((e: Event) => void) | null = null;
  onclose: ((e: CloseEvent) => void) | null = null;
  onmessage: ((e: MessageEvent) => void) | null = null;
  onerror: ((e: Event) => void) | null = null;
  readyState = MockWebSocket.CONNECTING;
  send = vi.fn();
  close = vi.fn().mockImplementation(() => {
    this.readyState = MockWebSocket.CLOSED;
  });

  constructor(url: string, protocols?: string | string[]) {
    this.url = url;
    this.protocols = Array.isArray(protocols) ? protocols : protocols ? [protocols] : [];
    lastWS = this as unknown as FakeWS;
    wsInstances.push(lastWS);
  }

  simulateOpen() {
    this.readyState = MockWebSocket.OPEN;
    this.onopen?.({} as Event);
  }

  simulateMessage(data: object) {
    this.onmessage?.({ data: JSON.stringify(data) } as MessageEvent);
  }

  simulateClose() {
    this.readyState = MockWebSocket.CLOSED;
    this.onclose?.({} as CloseEvent);
  }
}

function ws(): MockWebSocket {
  if (!lastWS) throw new Error("No WebSocket instantiated yet");
  return lastWS as unknown as MockWebSocket;
}

// ---------------------------------------------------------------------------
// Module mocks
// ---------------------------------------------------------------------------

const pushMock = vi.fn();
vi.mock("expo-router", () => ({
  useRouter: () => ({ push: pushMock }),
}));

let mockAccessToken: string | null = "test-token";
vi.mock("@evey/store", () => ({
  useAuthStore: (selector: (s: { accessToken: string | null }) => unknown) =>
    selector({ accessToken: mockAccessToken }),
}));

let mockServerUrl = "http://myserver:8080";
vi.mock("./useServerUrl", () => ({
  useServerUrl: () => mockServerUrl,
}));

const getItemAsyncMock = vi.fn();
const setItemAsyncMock = vi.fn();
vi.mock("expo-secure-store", () => ({
  getItemAsync: (...args: unknown[]) => getItemAsyncMock(...args),
  setItemAsync: (...args: unknown[]) => setItemAsyncMock(...args),
}));

vi.mock("expo-constants", () => ({
  default: { deviceName: null },
}));

let appStateListener: ((state: string) => void) | null = null;
const appStateRemoveMock = vi.fn();
vi.mock("react-native", async () => {
  const actual = await vi.importActual<typeof import("react-native")>("react-native");
  return {
    ...actual,
    AppState: {
      addEventListener: vi.fn((_event: string, listener: (state: string) => void) => {
        appStateListener = listener;
        return { remove: appStateRemoveMock };
      }),
      currentState: "active",
    },
  };
});

import { useDeviceRegistration } from "./useDeviceRegistration";

beforeEach(() => {
  vi.clearAllMocks();
  vi.useRealTimers();
  lastWS = null;
  wsInstances = [];
  appStateListener = null;
  mockAccessToken = "test-token";
  mockServerUrl = "http://myserver:8080";
  getItemAsyncMock.mockResolvedValue("device-abc-123");
  (globalThis as unknown as Record<string, unknown>).WebSocket = MockWebSocket;
});

describe("useDeviceRegistration", () => {
  it("does not connect when there is no access token", async () => {
    mockAccessToken = null;
    renderHook(() => useDeviceRegistration());
    await act(async () => {});
    expect(lastWS).toBeNull();
  });

  it("does not connect when the server URL hasn't resolved", async () => {
    mockServerUrl = "";
    renderHook(() => useDeviceRegistration());
    await act(async () => {});
    expect(lastWS).toBeNull();
  });

  it("connects to the devices websocket with wss/ws derived from the server URL and Bearer protocol", async () => {
    renderHook(() => useDeviceRegistration());
    await act(async () => {});

    expect(ws().url).toBe("ws://myserver:8080/api/v1/devices/ws");
    expect(ws().protocols).toEqual(["Bearer.test-token"]);
  });

  it("sends a register frame with the persisted device id and deviceType=mobile on open", async () => {
    renderHook(() => useDeviceRegistration());
    await act(async () => {});
    act(() => { ws().simulateOpen(); });

    expect(ws().send).toHaveBeenCalledTimes(1);
    const frame = JSON.parse(ws().send.mock.calls[0][0] as string);
    expect(frame).toEqual({
      type: "register",
      deviceId: "device-abc-123",
      name: "Android Phone",
      deviceType: "mobile",
    });
  });

  it("generates and persists a new device id when none exists yet", async () => {
    getItemAsyncMock.mockResolvedValue(null);
    renderHook(() => useDeviceRegistration());
    await act(async () => {});
    act(() => { ws().simulateOpen(); });

    expect(setItemAsyncMock).toHaveBeenCalledWith("evey-device-id", expect.any(String));
    const frame = JSON.parse(ws().send.mock.calls[0][0] as string);
    expect(typeof frame.deviceId).toBe("string");
    expect(frame.deviceId.length).toBeGreaterThan(0);
  });

  it("navigates to the player route on a loadAndPlay command", async () => {
    renderHook(() => useDeviceRegistration());
    await act(async () => {});
    act(() => { ws().simulateOpen(); });

    act(() => {
      ws().simulateMessage({ type: "loadAndPlay", mediaId: "media-42", positionMs: 12000 });
    });

    expect(pushMock).toHaveBeenCalledWith({
      pathname: "/(auth)/player/[id]",
      params: { id: "media-42", positionMs: "12000" },
    });
  });

  it("navigates without positionMs param when the command omits it", async () => {
    renderHook(() => useDeviceRegistration());
    await act(async () => {});
    act(() => { ws().simulateOpen(); });

    act(() => {
      ws().simulateMessage({ type: "loadAndPlay", mediaId: "media-7" });
    });

    expect(pushMock).toHaveBeenCalledWith({
      pathname: "/(auth)/player/[id]",
      params: { id: "media-7" },
    });
  });

  it("does not navigate and does not throw for play/pause/seek commands", async () => {
    renderHook(() => useDeviceRegistration());
    await act(async () => {});
    act(() => { ws().simulateOpen(); });

    expect(() => {
      act(() => { ws().simulateMessage({ type: "play", positionMs: 1000 }); });
      act(() => { ws().simulateMessage({ type: "pause", positionMs: 1000 }); });
      act(() => { ws().simulateMessage({ type: "seek", positionMs: 1000 }); });
    }).not.toThrow();
    expect(pushMock).not.toHaveBeenCalled();
  });

  it("ignores malformed JSON without throwing", async () => {
    renderHook(() => useDeviceRegistration());
    await act(async () => {});
    act(() => { ws().simulateOpen(); });

    expect(() => {
      act(() => { ws().onmessage?.({ data: "not-json{{" } as MessageEvent); });
    }).not.toThrow();
  });

  it("schedules a reconnect after the socket closes", async () => {
    vi.useFakeTimers();
    renderHook(() => useDeviceRegistration());
    await act(async () => {
      await vi.runOnlyPendingTimersAsync();
    });
    act(() => { ws().simulateOpen(); });

    act(() => { ws().simulateClose(); });
    // Backoff schedules a new connect attempt ~1s later - advance and expect
    // a second WebSocket instance to have been created.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1500);
    });

    expect(wsInstances.length).toBeGreaterThanOrEqual(2);
    vi.useRealTimers();
  });

  it("closes the socket and stops reconnecting when the app backgrounds", async () => {
    vi.useFakeTimers();
    renderHook(() => useDeviceRegistration());
    await act(async () => {
      await vi.runOnlyPendingTimersAsync();
    });
    act(() => { ws().simulateOpen(); });
    const firstSocket = ws();

    expect(appStateListener).not.toBeNull();
    act(() => { appStateListener?.("background"); });

    expect(firstSocket.close).toHaveBeenCalled();

    // No reconnect should be scheduled while backgrounded.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(30_000);
    });
    expect(wsInstances.length).toBe(1);
    vi.useRealTimers();
  });

  it("reconnects when the app returns to active after backgrounding", async () => {
    vi.useFakeTimers();
    renderHook(() => useDeviceRegistration());
    await act(async () => {
      await vi.runOnlyPendingTimersAsync();
    });
    act(() => { ws().simulateOpen(); });

    act(() => { appStateListener?.("background"); });
    expect(wsInstances.length).toBe(1);

    await act(async () => {
      appStateListener?.("active");
      await vi.runOnlyPendingTimersAsync();
    });

    expect(wsInstances.length).toBe(2);
    vi.useRealTimers();
  });

  it("closes the socket and removes the AppState listener on unmount (logout/navigation away)", async () => {
    const { unmount } = renderHook(() => useDeviceRegistration());
    await act(async () => {});
    act(() => { ws().simulateOpen(); });
    const socket = ws();

    unmount();

    expect(socket.close).toHaveBeenCalled();
    expect(appStateRemoveMock).toHaveBeenCalled();
  });

  it("opens no socket when it unmounts while the device id is still being read (StrictMode double mount)", async () => {
    let resolveId: (id: string) => void = () => {};
    getItemAsyncMock.mockReturnValue(new Promise<string>((resolve) => { resolveId = resolve; }));
    const { unmount } = renderHook(() => useDeviceRegistration());

    unmount();
    await act(async () => { resolveId("device-abc-123"); });

    expect(wsInstances.length).toBe(0);
  });
});
