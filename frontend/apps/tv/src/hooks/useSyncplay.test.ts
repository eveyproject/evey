import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";

// ---------------------------------------------------------------------------
// Mock WebSocket with a controllable fake - mirrors the mobile hook's test
// (frontend/apps/mobile/src/hooks/useSyncPlay.test.ts), same seam
// (globalThis.WebSocket), since this hook follows the exact same protocol.
// ---------------------------------------------------------------------------

interface FakeWS {
  url: string;
  protocols: string[];
  onopen: ((e: Event) => void) | null;
  onclose: ((e: CloseEvent) => void) | null;
  onmessage: ((e: MessageEvent) => void) | null;
  readyState: number;
  send: ReturnType<typeof vi.fn>;
  close: ReturnType<typeof vi.fn>;
}

let lastWS: FakeWS | null = null;

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
  readyState = MockWebSocket.CONNECTING;
  send = vi.fn();
  close = vi.fn().mockImplementation(() => {
    this.readyState = MockWebSocket.CLOSED;
    this.onclose?.({} as CloseEvent);
  });

  constructor(url: string, protocols?: string | string[]) {
    this.url = url;
    this.protocols = Array.isArray(protocols) ? protocols : protocols ? [protocols] : [];
    lastWS = this as unknown as FakeWS;
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

vi.mock("@evey/store", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/store")>()),
  useAuthStore: { getState: () => ({ accessToken: "test-token" }) },
}));

const eveyFetchMock = vi.fn();
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  eveyFetch: (...args: unknown[]) => eveyFetchMock(...args),
}));

vi.mock("./useServerUrl", () => ({ useServerUrl: () => "http://localhost:8080" }));

import { useSyncplay, joinSyncplayRoomByCode } from "./useSyncplay";

beforeEach(() => {
  vi.clearAllMocks();
  lastWS = null;
  (globalThis as unknown as Record<string, unknown>).WebSocket = MockWebSocket;
});

afterEach(() => {
  vi.useRealTimers();
});

function ws(): MockWebSocket {
  if (!lastWS) throw new Error("No WebSocket instantiated yet");
  return lastWS as unknown as MockWebSocket;
}

describe("joinSyncplayRoomByCode", () => {
  it("posts the code and returns the room/media ids", async () => {
    eveyFetchMock.mockResolvedValueOnce({ roomId: "room-1", mediaId: "mi-1" });
    const result = await joinSyncplayRoomByCode("12345");
    expect(result).toEqual({ roomId: "room-1", mediaId: "mi-1" });
    expect(eveyFetchMock).toHaveBeenCalledWith(
      expect.objectContaining({ url: "/api/v1/syncplay/join", method: "POST", data: { code: "12345" } }),
    );
  });

  it("propagates a rejection for an unknown/expired code", async () => {
    eveyFetchMock.mockRejectedValueOnce(new Error("room not found"));
    await expect(joinSyncplayRoomByCode("00000")).rejects.toThrow("room not found");
  });
});

describe("useSyncplay - drift-aware dispatch", () => {
  it("applies the play transition without seeking when the drift is under the threshold", () => {
    const onPlay = vi.fn();
    const getLocalPositionMs = vi.fn(() => 10_300); // 300ms ahead of the incoming target
    const { result } = renderHook(() => useSyncplay({ getLocalPositionMs, onPlay }));

    act(() => { result.current.connect("room-1"); });
    act(() => { ws().simulateOpen(); });
    act(() => { ws().simulateMessage({ type: "play", positionMs: 10_000 }); });

    expect(onPlay).toHaveBeenCalledWith(10_300, false); // stays put, only the transition applies
  });

  it("seeks when the drift is above the threshold", () => {
    const onPlay = vi.fn();
    const getLocalPositionMs = vi.fn(() => 10_000);
    const { result } = renderHook(() => useSyncplay({ getLocalPositionMs, onPlay }));

    act(() => { result.current.connect("room-2"); });
    act(() => { ws().simulateOpen(); });
    act(() => { ws().simulateMessage({ type: "play", positionMs: 20_000 }); });

    expect(onPlay).toHaveBeenCalledWith(20_000, true);
  });

  it("applies pause the same drift-aware way", () => {
    const onPause = vi.fn();
    const getLocalPositionMs = vi.fn(() => 5_000);
    const { result } = renderHook(() => useSyncplay({ getLocalPositionMs, onPause }));

    act(() => { result.current.connect("room-3"); });
    act(() => { ws().simulateOpen(); });
    act(() => { ws().simulateMessage({ type: "pause", positionMs: 5_050 }); });

    expect(onPause).toHaveBeenCalledWith(5_000, false);
  });

  it("always applies an explicit seek regardless of drift", () => {
    const onSeek = vi.fn();
    const getLocalPositionMs = vi.fn(() => 1_000);
    const { result } = renderHook(() => useSyncplay({ getLocalPositionMs, onSeek }));

    act(() => { result.current.connect("room-4"); });
    act(() => { ws().simulateOpen(); });
    act(() => { ws().simulateMessage({ type: "seek", positionMs: 1_050 }); });

    expect(onSeek).toHaveBeenCalledWith(1_050);
  });

  it("resyncs the initial 'state' snapshot on join when the room is far ahead", () => {
    const onPlay = vi.fn();
    const getLocalPositionMs = vi.fn(() => 0);
    const { result } = renderHook(() => useSyncplay({ getLocalPositionMs, onPlay }));

    act(() => { result.current.connect("room-5"); });
    act(() => { ws().simulateOpen(); });
    act(() => { ws().simulateMessage({ type: "state", playing: true, positionMs: 600_000 }); });

    expect(onPlay).toHaveBeenCalledWith(600_000, true);
  });
});

describe("useSyncplay - disconnect does not touch playback", () => {
  it("marks disconnected without throwing and without calling onPlay/onPause", () => {
    const onPlay = vi.fn();
    const onPause = vi.fn();
    const { result } = renderHook(() => useSyncplay({ getLocalPositionMs: () => 0, onPlay, onPause }));

    act(() => { result.current.connect("room-6"); });
    act(() => { ws().simulateOpen(); });
    expect(result.current.state.connected).toBe(true);

    vi.useFakeTimers();
    expect(() => {
      act(() => { ws().simulateClose(); });
    }).not.toThrow();

    expect(result.current.state.connected).toBe(false);
    expect(onPlay).not.toHaveBeenCalled();
    expect(onPause).not.toHaveBeenCalled();
    vi.clearAllTimers();
  });
});

describe("useSyncplay - membership", () => {
  it("tracks member_joined and member_left", () => {
    const { result } = renderHook(() => useSyncplay({ getLocalPositionMs: () => 0 }));
    act(() => { result.current.connect("room-7"); });
    act(() => { ws().simulateOpen(); });

    act(() => { ws().simulateMessage({ type: "member_joined", userId: "u1", username: "Alice" }); });
    expect(result.current.state.members).toEqual([{ userId: "u1", username: "Alice" }]);

    act(() => { ws().simulateMessage({ type: "member_left", userId: "u1" }); });
    expect(result.current.state.members).toEqual([]);
  });
});

describe("useSyncplay - leaveRoom", () => {
  it("resets state and calls the leave endpoint", async () => {
    const { result } = renderHook(() => useSyncplay({ getLocalPositionMs: () => 0 }));
    act(() => { result.current.connect("room-8"); });
    act(() => { ws().simulateOpen(); });

    eveyFetchMock.mockResolvedValueOnce(undefined);
    await act(async () => { await result.current.leaveRoom(); });

    expect(result.current.state).toEqual({ roomId: null, roomCode: null, connected: false, isHost: false, members: [], queue: [], trackIndex: 0 });
    expect(eveyFetchMock).toHaveBeenCalledWith(
      expect.objectContaining({ url: "/api/v1/syncplay/rooms/room-8/leave", method: "DELETE" }),
    );
  });
});

// What the television's copy lacked until it shared the web's socket
// (@evey/client useSyncPlaySocket, 2026-09-24).
describe("useSyncplay - what the web already handled", () => {
  it("keeps a play pressed while the socket opens and sends it once open", () => {
    const { result } = renderHook(() => useSyncplay({ getLocalPositionMs: () => 0 }));
    act(() => { result.current.connect("room-9"); });
    act(() => { result.current.sendPlay(1234); });
    expect(ws().send).not.toHaveBeenCalled();
    act(() => { ws().simulateOpen(); });
    expect(ws().send).toHaveBeenCalledWith(JSON.stringify({ type: "play", positionMs: 1234 }));
  });

  it("backs off and gives up after five reconnects instead of retrying every second forever", () => {
    vi.useFakeTimers();
    const sockets = new Set<unknown>();
    const { result } = renderHook(() => useSyncplay({ getLocalPositionMs: () => 0 }));
    act(() => { result.current.connect("room-9"); });
    for (let i = 0; i < 10; i++) {
      sockets.add(ws());
      act(() => { ws().simulateClose(); });
      act(() => { vi.advanceTimersByTime(60_000); });
    }
    sockets.add(ws());
    vi.useRealTimers();
    expect(sockets.size).toBe(1 + 5);
  });
});
