import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";

// ---------------------------------------------------------------------------
// Mock WebSocket with a controllable fake - seam is globalThis.WebSocket
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

  /** Helper: simulate server opening the connection */
  simulateOpen() {
    this.readyState = MockWebSocket.OPEN;
    this.onopen?.({} as Event);
  }

  /** Helper: simulate receiving a JSON message from the server */
  simulateMessage(data: object) {
    this.onmessage?.({ data: JSON.stringify(data) } as MessageEvent);
  }

  /** Helper: simulate server-side close (triggers reconnect) */
  simulateClose() {
    this.readyState = MockWebSocket.CLOSED;
    this.onclose?.({} as CloseEvent);
  }
}

// ---------------------------------------------------------------------------
// Module mocks
// ---------------------------------------------------------------------------

const auth = vi.hoisted(() => ({ token: "test-token" }));
vi.mock("@evey/store", () => ({
  useAuthStore: {
    getState: () => ({ accessToken: auth.token }),
  },
}));

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
}));

vi.mock("./useServerUrl", () => ({
  useServerUrl: () => "http://localhost:8080",
}));

import { eveyFetch } from "@evey/api";
import { useSyncPlay } from "./useSyncPlay";

const mockFetch = vi.mocked(eveyFetch);

beforeEach(() => {
  vi.clearAllMocks();
  lastWS = null;
  // Replace global WebSocket with our fake
  (globalThis as unknown as Record<string, unknown>).WebSocket = MockWebSocket;
});

afterEach(() => {
  vi.useRealTimers();
});

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function ws(): MockWebSocket {
  if (!lastWS) throw new Error("No WebSocket instantiated yet");
  return lastWS as unknown as MockWebSocket;
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("useSyncPlay - message dispatch", () => {
  it("calls onPlay callback for 'play' message", async () => {
    const onPlay = vi.fn();
    const { result } = renderHook(() => useSyncPlay({ onPlay }));

    // The assertion below used to be a bare `result.current.createRoom;` expression
    // statement (no-op, tested nothing despite the "ensure hook is live" comment).
    expect(result.current.createRoom).toBeInstanceOf(Function);
    // Manually call connect via createRoom mock
    mockFetch.mockResolvedValueOnce({ id: "room-1", code: "12345" });

    await act(async () => {
      await result.current.createRoom("mi-abc");
    });

    act(() => { ws().simulateOpen(); });
    act(() => { ws().simulateMessage({ type: "play", positionMs: 5000 }); });

    expect(onPlay).toHaveBeenCalledWith(5000, true);
  });

  it("calls onPause callback for 'pause' message", async () => {
    const onPause = vi.fn();
    const { result } = renderHook(() => useSyncPlay({ onPause }));

    mockFetch.mockResolvedValueOnce({ id: "room-2", code: "22222" });
    await act(async () => { await result.current.createRoom("mi-xyz"); });

    act(() => { ws().simulateOpen(); });
    act(() => { ws().simulateMessage({ type: "pause", positionMs: 3000 }); });

    expect(onPause).toHaveBeenCalledWith(3000, true);
  });

  it("calls onSeek callback for 'seek' message", async () => {
    const onSeek = vi.fn();
    const { result } = renderHook(() => useSyncPlay({ onSeek }));

    mockFetch.mockResolvedValueOnce({ id: "room-3", code: "33333" });
    await act(async () => { await result.current.createRoom("mi-seek"); });

    act(() => { ws().simulateOpen(); });
    act(() => { ws().simulateMessage({ type: "seek", positionMs: 9999 }); });

    expect(onSeek).toHaveBeenCalledWith(9999);
  });

  it("calls onMemberJoined and updates members state on 'member_joined'", async () => {
    const onMemberJoined = vi.fn();
    const { result } = renderHook(() => useSyncPlay({ onMemberJoined }));

    mockFetch.mockResolvedValueOnce({ id: "room-4", code: "44444" });
    await act(async () => { await result.current.createRoom("mi-m"); });

    act(() => { ws().simulateOpen(); });
    act(() => {
      ws().simulateMessage({ type: "member_joined", userId: "usr-alice", username: "Alice" });
    });

    expect(onMemberJoined).toHaveBeenCalledWith("usr-alice", "Alice");
    expect(result.current.state.members).toEqual([{ userId: "usr-alice", username: "Alice" }]);
  });

  it("removes member from state on 'member_left'", async () => {
    const onMemberLeft = vi.fn();
    const { result } = renderHook(() => useSyncPlay({ onMemberLeft }));

    mockFetch.mockResolvedValueOnce({ id: "room-5", code: "55555" });
    await act(async () => { await result.current.createRoom("mi-ml"); });

    act(() => { ws().simulateOpen(); });

    // Add member first
    act(() => {
      ws().simulateMessage({ type: "member_joined", userId: "usr-bob", username: "Bob" });
    });
    expect(result.current.state.members).toHaveLength(1);

    act(() => {
      ws().simulateMessage({ type: "member_left", userId: "usr-bob" });
    });
    expect(result.current.state.members).toHaveLength(0);
    expect(onMemberLeft).toHaveBeenCalledWith("usr-bob");
  });

  it("calls onTrackChange and updates trackIndex on 'setIndex' message", async () => {
    const onTrackChange = vi.fn();
    const { result } = renderHook(() => useSyncPlay({ onTrackChange }));

    mockFetch.mockResolvedValueOnce({ id: "room-6", code: "66666" });
    await act(async () => { await result.current.createRoom("mi-ti"); });

    act(() => { ws().simulateOpen(); });
    act(() => {
      ws().simulateMessage({ type: "setIndex", trackIndex: 2, mediaId: "mi-ep2" });
    });

    expect(onTrackChange).toHaveBeenCalledWith(2, "mi-ep2");
    expect(result.current.state.trackIndex).toBe(2);
  });

  it("ignores malformed JSON without throwing", async () => {
    const { result } = renderHook(() => useSyncPlay({}));

    mockFetch.mockResolvedValueOnce({ id: "room-7", code: "77777" });
    await act(async () => { await result.current.createRoom("mi-bad"); });

    act(() => { ws().simulateOpen(); });
    expect(() => {
      act(() => { ws().onmessage?.({ data: "not-json{{" } as MessageEvent); });
    }).not.toThrow();
  });
});

describe("useSyncPlay - state transitions", () => {
  it("sets connected=true after onopen fires", async () => {
    const { result } = renderHook(() => useSyncPlay({}));

    mockFetch.mockResolvedValueOnce({ id: "room-8", code: "88888" });
    await act(async () => { await result.current.createRoom("mi-conn"); });

    act(() => { ws().simulateOpen(); });
    expect(result.current.state.connected).toBe(true);
  });

  it("sets connected=false after onclose fires", async () => {
    const { result } = renderHook(() => useSyncPlay({}));

    mockFetch.mockResolvedValueOnce({ id: "room-9", code: "99999" });
    await act(async () => { await result.current.createRoom("mi-disc"); });

    act(() => { ws().simulateOpen(); });
    expect(result.current.state.connected).toBe(true);

    // Simulate server close; suppress reconnect timer
    vi.useFakeTimers();
    act(() => { ws().simulateClose(); });
    expect(result.current.state.connected).toBe(false);
    vi.clearAllTimers();
  });

  it("createRoom sets roomId, roomCode, isHost=true", async () => {
    const { result } = renderHook(() => useSyncPlay({}));

    mockFetch.mockResolvedValueOnce({ id: "room-cr", code: "ABCDE" });
    await act(async () => { await result.current.createRoom("mi-movie"); });

    expect(result.current.state.roomId).toBe("room-cr");
    expect(result.current.state.roomCode).toBe("ABCDE");
    expect(result.current.state.isHost).toBe(true);
  });

  // The phone's room builder: a music room from a queue of tracks.
  it("createRoomFromQueue posts the queue as a music room and hosts it", async () => {
    const { result } = renderHook(() => useSyncPlay({}));

    mockFetch
      .mockResolvedValueOnce({ id: "room-q", code: "QUEUE1" })
      .mockResolvedValueOnce({ queue: ["t1", "t2"], trackIndex: 0 });
    await act(async () => { await result.current.createRoomFromQueue(["t1", "t2"]); });

    expect(mockFetch).toHaveBeenNthCalledWith(1, expect.objectContaining({
      url: "/api/v1/syncplay/rooms",
      method: "POST",
      data: expect.objectContaining({ queue: ["t1", "t2"], mediaType: "music" }),
    }));
    expect(result.current.state.isHost).toBe(true);
    expect(result.current.state.roomCode).toBe("QUEUE1");
    expect(result.current.state.queue).toEqual(["t1", "t2"]);
  });

  it("joinByCode sets isHost=false and returns roomId/mediaId/mediaType (video when the server names none)", async () => {
    const { result } = renderHook(() => useSyncPlay({}));

    mockFetch
      .mockResolvedValueOnce({ roomId: "room-j", mediaId: "mi-joined" })
      // second call = GET room info
      .mockResolvedValueOnce({ queue: ["mi-joined"], trackIndex: 0 });

    let joinResult: { roomId: string; mediaId: string; mediaType: string } | undefined;
    await act(async () => {
      joinResult = await result.current.joinByCode("ABCDE");
    });

    expect(joinResult).toEqual({ roomId: "room-j", mediaId: "mi-joined", mediaType: "video" });
    expect(result.current.state.isHost).toBe(false);
    expect(result.current.state.roomId).toBe("room-j");
  });

  it("leaveRoom resets state to null/empty and calls DELETE endpoint", async () => {
    const { result } = renderHook(() => useSyncPlay({}));

    mockFetch.mockResolvedValueOnce({ id: "room-lr", code: "LEAVE" });
    await act(async () => { await result.current.createRoom("mi-lv"); });

    act(() => { ws().simulateOpen(); });
    expect(result.current.state.roomId).toBe("room-lr");

    mockFetch.mockResolvedValueOnce(undefined); // DELETE response
    await act(async () => { await result.current.leaveRoom(); });

    expect(result.current.state.roomId).toBeNull();
    expect(result.current.state.connected).toBe(false);
    expect(mockFetch).toHaveBeenCalledWith(
      expect.objectContaining({ url: "/api/v1/syncplay/rooms/room-lr/leave", method: "DELETE" }),
    );
  });
});

describe("useSyncPlay - send helpers", () => {
  async function setupConnectedRoom() {
    const { result } = renderHook(() => useSyncPlay({}));
    mockFetch.mockResolvedValueOnce({ id: "room-s", code: "SEND0" });
    await act(async () => { await result.current.createRoom("mi-s"); });
    act(() => { ws().simulateOpen(); });
    return result;
  }

  it("sendPlay sends correct JSON payload", async () => {
    const result = await setupConnectedRoom();
    act(() => { result.current.sendPlay(1234); });
    expect(ws().send).toHaveBeenCalledWith(JSON.stringify({ type: "play", positionMs: 1234 }));
  });

  it("sendPause sends correct JSON payload", async () => {
    const result = await setupConnectedRoom();
    act(() => { result.current.sendPause(5678); });
    expect(ws().send).toHaveBeenCalledWith(JSON.stringify({ type: "pause", positionMs: 5678 }));
  });

  it("sendNext sends correct JSON payload", async () => {
    const result = await setupConnectedRoom();
    act(() => { result.current.sendNext(); });
    expect(ws().send).toHaveBeenCalledWith(JSON.stringify({ type: "next" }));
  });

  it("sendSetIndex sends correct JSON payload", async () => {
    const result = await setupConnectedRoom();
    act(() => { result.current.sendSetIndex(3); });
    expect(ws().send).toHaveBeenCalledWith(JSON.stringify({ type: "setIndex", trackIndex: 3 }));
  });
});

describe("useSyncPlay - reconnect", () => {
  it("does not reconnect to the old room after switching rooms", async () => {
    vi.useFakeTimers();
    mockFetch
      .mockResolvedValueOnce({ id: "room-1", code: "C1" })
      .mockResolvedValueOnce({ id: "room-2", code: "C2" });
    const { result } = renderHook(() => useSyncPlay());

    await act(async () => { await result.current.createRoom("mi-1"); });
    act(() => { ws().simulateOpen(); });
    expect(ws().url).toContain("room-1");
    // room-1 drops involuntarily -> schedules a reconnect timer to room-1
    act(() => { ws().simulateClose(); });

    // switch to room-2 before that reconnect fires
    await act(async () => { await result.current.createRoom("mi-2"); });
    act(() => { ws().simulateOpen(); });
    expect(ws().url).toContain("room-2");

    // fire any pending timers: a stale room-1 reconnect would replace lastWS
    act(() => { vi.advanceTimersByTime(60_000); });
    expect(ws().url).toContain("room-2");
  });
});

// What the phone's own socket lacked until it shared the web's
// (@evey/client useSyncPlaySocket, 2026-09-24).
describe("useSyncPlay - what the web already handled", () => {
  it("backs off and gives up after five reconnects instead of retrying every second forever", async () => {
    vi.useFakeTimers();
    let sockets = 0;
    const Counting = class extends MockWebSocket {
      constructor(url: string, protocols?: string | string[]) {
        super(url, protocols);
        sockets += 1;
      }
    };
    (globalThis as unknown as Record<string, unknown>).WebSocket = Counting;
    mockFetch.mockResolvedValueOnce({ id: "room-1", code: "C1" });
    const { result } = renderHook(() => useSyncPlay());
    await act(async () => { await result.current.createRoom("mi-1"); });

    // The server is down: every socket closes without ever opening.
    for (let i = 0; i < 10; i++) {
      act(() => { ws().simulateClose(); });
      act(() => { vi.advanceTimersByTime(60_000); });
    }
    expect(sockets).toBe(1 + 5);
  });

  it("jumps to the sender's position and plays on an 'everyone sync to me' resync", async () => {
    const onSeek = vi.fn();
    const onPlay = vi.fn();
    mockFetch.mockResolvedValueOnce({ id: "room-1", code: "C1" });
    const { result } = renderHook(() => useSyncPlay({ onSeek, onPlay }));
    await act(async () => { await result.current.createRoom("mi-1"); });
    act(() => { ws().simulateOpen(); });
    act(() => { ws().simulateMessage({ type: "resync", positionMs: 42_000 }); });
    expect(onSeek).toHaveBeenCalledWith(42_000);
    expect(onPlay).toHaveBeenCalledWith(42_000, true);
  });

  it("hands the host controls to the member the server promotes", async () => {
    // A JWT whose subject is usr-me.
    auth.token = `h.${btoa(JSON.stringify({ sub: "usr-me" }))}.s`;
    mockFetch.mockResolvedValueOnce({ roomId: "room-1", mediaId: "mi-1" }).mockResolvedValueOnce({});
    const { result } = renderHook(() => useSyncPlay());
    await act(async () => { await result.current.joinByCode("ABC123"); });
    act(() => { ws().simulateOpen(); });
    expect(result.current.state.isHost).toBe(false);
    act(() => { ws().simulateMessage({ type: "host_changed", userId: "usr-me" }); });
    expect(result.current.state.isHost).toBe(true);
    auth.token = "test-token";
  });
});

describe("useSyncPlay - the position it sends", () => {
  // The phone sends player.currentTime * 1000: a float the server's int64
  // could not decode, so the room never followed the host's pause.
  it("sends whole milliseconds", async () => {
    mockFetch.mockResolvedValueOnce({ id: "room-1", code: "C1" });
    const { result } = renderHook(() => useSyncPlay());
    await act(async () => { await result.current.createRoom("mi-1"); });
    act(() => { ws().simulateOpen(); });
    act(() => { result.current.sendPause(67833.333); });
    expect(ws().send).toHaveBeenLastCalledWith(JSON.stringify({ type: "pause", positionMs: 67833 }));
  });
});
