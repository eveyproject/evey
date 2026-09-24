import { renderHook, act, cleanup } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

const eveyFetchFn = vi.fn();

vi.mock("@evey/api", () => ({
  eveyFetch: eveyFetchFn,
  syncPlayServiceCreateRoom: () => eveyFetchFn(),
  syncPlayServiceJoinRoom: () => eveyFetchFn(),
  syncPlayServiceGetRoom: () => eveyFetchFn(),
  syncPlayServiceLeaveRoom: () => eveyFetchFn(),
  syncPlayServiceJoinByCode: () => eveyFetchFn(),
}));
const storeState = { accessToken: "header.eyJzdWIiOiJ1c2VyLTEifQ.sig" };
vi.mock("@evey/store", () => ({
  useAuthStore: Object.assign(
    (fn: (s: { accessToken: string | null }) => unknown) => fn(storeState),
    { getState: () => storeState },
  ),
}));

// Fake WebSocket
class MockWebSocket {
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSING = 2;
  static CLOSED = 3;
  static instances: MockWebSocket[] = [];

  readyState = MockWebSocket.CONNECTING;
  url: string;
  protocols: string | string[];
  onopen: ((ev: Event) => void) | null = null;
  onclose: ((ev: CloseEvent) => void) | null = null;
  onmessage: ((ev: MessageEvent) => void) | null = null;
  onerror: ((ev: Event) => void) | null = null;
  send = vi.fn();
  close = vi.fn(() => {
    this.readyState = MockWebSocket.CLOSED;
    this.onclose?.(new CloseEvent("close"));
  });

  constructor(url: string, protocols?: string | string[]) {
    this.url = url;
    this.protocols = protocols ?? "";
    MockWebSocket.instances.push(this);
  }

  simulateOpen() {
    this.readyState = MockWebSocket.OPEN;
    this.onopen?.(new Event("open"));
  }

  simulateMessage(data: Record<string, unknown>) {
    this.onmessage?.(new MessageEvent("message", { data: JSON.stringify(data) }));
  }

  simulateClose() {
    this.readyState = MockWebSocket.CLOSED;
    this.onclose?.(new CloseEvent("close"));
  }

  simulateError() {
    this.onerror?.(new Event("error"));
  }
}

// Assign constants needed by the hook (WebSocket.OPEN check)
Object.assign(MockWebSocket, { CONNECTING: 0, OPEN: 1, CLOSING: 2, CLOSED: 3 });

vi.stubGlobal("WebSocket", MockWebSocket);

function latestWs(): MockWebSocket {
  return MockWebSocket.instances[MockWebSocket.instances.length - 1];
}

describe("useSyncPlay", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    MockWebSocket.instances = [];
    eveyFetchFn.mockReset();
  });

  afterEach(() => {
    cleanup();
  });

  async function loadHook(opts = {}) {
    const mod = await import("./useSyncPlay");
    return renderHook(() => mod.useSyncPlay(opts));
  }

  it("returns disconnected initial state", async () => {
    const { result } = await loadHook();
    const [state] = result.current;
    expect(state).toEqual({
      roomId: null,
      roomCode: null,
      members: [],
      connected: false,
      isHost: false,
      queue: [],
      trackIndex: 0,
    });
  });

  describe("createRoom", () => {
    it("creates room, sets state as host, and opens WebSocket", async () => {
      eveyFetchFn.mockResolvedValueOnce({ id: "room-1", code: "ABC123" });
      const { result } = await loadHook();

      let roomId: string;
      await act(async () => {
        roomId = await result.current[1].createRoom("media-1");
      });

      expect(roomId!).toBe("room-1");
      expect(eveyFetchFn).toHaveBeenCalled();

      const ws = latestWs();
      expect(ws.url).toContain("/api/v1/syncplay/rooms/room-1/ws");
      expect(ws.protocols).toEqual([expect.stringContaining("Bearer.")]);

      act(() => ws.simulateOpen());

      const [state] = result.current;
      expect(state.roomId).toBe("room-1");
      expect(state.roomCode).toBe("ABC123");
      expect(state.isHost).toBe(true);
      expect(state.connected).toBe(true);
    });
  });

  describe("joinRoom", () => {
    it("joins room, fetches info, sets state as non-host", async () => {
      eveyFetchFn
        .mockResolvedValueOnce(undefined) // POST join
        .mockResolvedValueOnce({ members: [{ userId: "u2", username: "Bob" }], code: "XYZ" }); // GET info
      const { result } = await loadHook();

      await act(async () => {
        await result.current[1].joinRoom("room-2");
      });

      expect(eveyFetchFn).toHaveBeenCalled();

      const ws = latestWs();
      act(() => ws.simulateOpen());

      const [state] = result.current;
      expect(state.roomId).toBe("room-2");
      expect(state.roomCode).toBe("XYZ");
      expect(state.isHost).toBe(false);
      expect(state.members).toEqual([{ userId: "u2", username: "Bob" }]);
      expect(state.connected).toBe(true);
    });
  });

  describe("joinByCode", () => {
    it("joins by code and returns roomId + mediaId", async () => {
      eveyFetchFn.mockResolvedValueOnce({ roomId: "room-3", mediaId: "media-5", wsUrl: "ws://x" });
      const { result } = await loadHook();

      let res: { roomId: string; mediaId: string; mediaType?: string };
      await act(async () => {
        res = await result.current[1].joinByCode("MYCODE");
      });

      expect(res!).toMatchObject({ roomId: "room-3", mediaId: "media-5" });
      expect(eveyFetchFn).toHaveBeenCalled();
      expect(latestWs().url).toContain("/api/v1/syncplay/rooms/room-3/ws");
    });
  });

  describe("leaveRoom", () => {
    it("closes WebSocket, resets state, and calls leave API", async () => {
      eveyFetchFn
        .mockResolvedValueOnce({ id: "room-1", code: "C" }) // createRoom
        .mockResolvedValueOnce(undefined); // leaveRoom DELETE
      const { result } = await loadHook();

      await act(async () => {
        await result.current[1].createRoom("media-1");
      });
      const ws = latestWs();
      act(() => ws.simulateOpen());
      expect(result.current[0].connected).toBe(true);

      await act(async () => {
        await result.current[1].leaveRoom();
      });

      expect(ws.close).toHaveBeenCalled();
      expect(eveyFetchFn).toHaveBeenCalled();
      const [state] = result.current;
      expect(state.roomId).toBeNull();
      expect(state.connected).toBe(false);
      expect(state.members).toEqual([]);
    });

    it("swallows leave API errors gracefully", async () => {
      eveyFetchFn
        .mockResolvedValueOnce({ id: "room-1", code: "C" })
        .mockRejectedValueOnce(new Error("network"));
      const { result } = await loadHook();

      await act(async () => {
        await result.current[1].createRoom("media-1");
      });
      // Should not throw
      await act(async () => {
        await result.current[1].leaveRoom();
      });
      expect(result.current[0].roomId).toBeNull();
    });
  });

  describe("send play/pause/seek", () => {
    async function setupConnected() {
      eveyFetchFn.mockResolvedValueOnce({ id: "room-1", code: "C" });
      const hook = await loadHook();
      await act(async () => {
        await hook.result.current[1].createRoom("media-1");
      });
      const ws = latestWs();
      act(() => ws.simulateOpen());
      return { hook, ws };
    }

    it("sendPlay sends play message via WebSocket", async () => {
      const { hook, ws } = await setupConnected();
      act(() => hook.result.current[1].sendPlay(5000));
      expect(ws.send).toHaveBeenCalledWith(JSON.stringify({ type: "play", positionMs: 5000 }));
    });

    it("sendPause sends pause message via WebSocket", async () => {
      const { hook, ws } = await setupConnected();
      act(() => hook.result.current[1].sendPause(12000));
      expect(ws.send).toHaveBeenCalledWith(JSON.stringify({ type: "pause", positionMs: 12000 }));
    });

    it("sendSeek sends seek message via WebSocket", async () => {
      const { hook, ws } = await setupConnected();
      act(() => hook.result.current[1].sendSeek(30000));
      expect(ws.send).toHaveBeenCalledWith(JSON.stringify({ type: "seek", positionMs: 30000 }));
    });

    it("does not send when WebSocket is not open", async () => {
      eveyFetchFn.mockResolvedValueOnce({ id: "room-1", code: "C" });
      const { result } = await loadHook();
      await act(async () => {
        await result.current[1].createRoom("media-1");
      });
      // WS still CONNECTING (not open)
      act(() => result.current[1].sendPlay(1000));
      expect(latestWs().send).not.toHaveBeenCalled();
    });
  });

  describe("receiving messages", () => {
    it("calls onPlay callback on play message", async () => {
      const onPlay = vi.fn();
      eveyFetchFn.mockResolvedValueOnce({ id: "room-1", code: "C" });
      const { result } = await loadHook({ onPlay });

      await act(async () => {
        await result.current[1].createRoom("media-1");
      });
      const ws = latestWs();
      act(() => ws.simulateOpen());
      act(() => ws.simulateMessage({ type: "play", positionMs: 7000 }));

      expect(onPlay).toHaveBeenCalledWith(7000, true);
    });

    it("calls onPause callback on pause message", async () => {
      const onPause = vi.fn();
      eveyFetchFn.mockResolvedValueOnce({ id: "room-1", code: "C" });
      const { result } = await loadHook({ onPause });

      await act(async () => {
        await result.current[1].createRoom("media-1");
      });
      const ws = latestWs();
      act(() => ws.simulateOpen());
      act(() => ws.simulateMessage({ type: "pause", positionMs: 9000 }));

      expect(onPause).toHaveBeenCalledWith(9000, true);
    });

    it("calls onSeek callback on seek message", async () => {
      const onSeek = vi.fn();
      eveyFetchFn.mockResolvedValueOnce({ id: "room-1", code: "C" });
      const { result } = await loadHook({ onSeek });

      await act(async () => {
        await result.current[1].createRoom("media-1");
      });
      const ws = latestWs();
      act(() => ws.simulateOpen());
      act(() => ws.simulateMessage({ type: "seek", positionMs: 60000 }));

      expect(onSeek).toHaveBeenCalledWith(60000);
    });

    it("defaults positionMs to 0 when missing", async () => {
      const onPlay = vi.fn();
      eveyFetchFn.mockResolvedValueOnce({ id: "room-1", code: "C" });
      const { result } = await loadHook({ onPlay });

      await act(async () => {
        await result.current[1].createRoom("media-1");
      });
      const ws = latestWs();
      act(() => ws.simulateOpen());
      act(() => ws.simulateMessage({ type: "play" }));

      expect(onPlay).toHaveBeenCalledWith(0, true);
    });

    it("handles member_joined - adds member and calls callback", async () => {
      const onMemberJoined = vi.fn();
      eveyFetchFn.mockResolvedValueOnce({ id: "room-1", code: "C" });
      const { result } = await loadHook({ onMemberJoined });

      await act(async () => {
        await result.current[1].createRoom("media-1");
      });
      const ws = latestWs();
      act(() => ws.simulateOpen());
      act(() => ws.simulateMessage({ type: "member_joined", userId: "u3", username: "Charlie" }));

      expect(result.current[0].members).toEqual([{ userId: "u3", username: "Charlie" }]);
      expect(onMemberJoined).toHaveBeenCalledWith("u3", "Charlie");
    });

    it("handles member_left - removes member and calls callback", async () => {
      const onMemberLeft = vi.fn();
      eveyFetchFn.mockResolvedValueOnce({ id: "room-1", code: "C" });
      const { result } = await loadHook({ onMemberLeft });

      await act(async () => {
        await result.current[1].createRoom("media-1");
      });
      const ws = latestWs();
      act(() => ws.simulateOpen());
      act(() => ws.simulateMessage({ type: "member_joined", userId: "u3", username: "Charlie" }));
      expect(result.current[0].members).toHaveLength(1);

      act(() => ws.simulateMessage({ type: "member_left", userId: "u3" }));
      expect(result.current[0].members).toEqual([]);
      expect(onMemberLeft).toHaveBeenCalledWith("u3");
    });

    it("deduplicates members on repeated member_joined", async () => {
      eveyFetchFn.mockResolvedValueOnce({ id: "room-1", code: "C" });
      const { result } = await loadHook();

      await act(async () => {
        await result.current[1].createRoom("media-1");
      });
      const ws = latestWs();
      act(() => ws.simulateOpen());
      act(() => ws.simulateMessage({ type: "member_joined", userId: "u3", username: "Charlie" }));
      act(() => ws.simulateMessage({ type: "member_joined", userId: "u3", username: "Charlie" }));

      expect(result.current[0].members).toEqual([{ userId: "u3", username: "Charlie" }]);
    });

    // A late joiner lands on the room's position and takes its play state: it
    // used to be seeked only, so a paused room's newcomer kept playing alone.
    it("lands a late joiner on the room's position, paused or playing like the room", async () => {
      const onPlay = vi.fn();
      const onPause = vi.fn();
      eveyFetchFn.mockResolvedValueOnce({ id: "room-1", code: "C" });
      const { result } = await loadHook({ onPlay, onPause });

      await act(async () => {
        await result.current[1].createRoom("media-1");
      });
      const ws = latestWs();
      act(() => ws.simulateOpen());
      act(() => ws.simulateMessage({ type: "state", positionMs: 45000 }));
      expect(onPause).toHaveBeenCalledWith(45000, true);
      act(() => ws.simulateMessage({ type: "state", positionMs: 46000, playing: true }));
      expect(onPlay).toHaveBeenCalledWith(46000, true);
    });

    it("ignores malformed JSON messages", async () => {
      const onPlay = vi.fn();
      eveyFetchFn.mockResolvedValueOnce({ id: "room-1", code: "C" });
      const { result } = await loadHook({ onPlay });

      await act(async () => {
        await result.current[1].createRoom("media-1");
      });
      const ws = latestWs();
      act(() => ws.simulateOpen());

      // Send raw bad JSON
      act(() => {
        ws.onmessage?.(new MessageEvent("message", { data: "not json{{{" }));
      });

      expect(onPlay).not.toHaveBeenCalled();
    });
  });

  describe("disconnect/reconnect", () => {
    it("sets connected=false on WebSocket close", async () => {
      eveyFetchFn.mockResolvedValueOnce({ id: "room-1", code: "C" });
      const { result } = await loadHook();

      await act(async () => {
        await result.current[1].createRoom("media-1");
      });
      const ws = latestWs();
      act(() => ws.simulateOpen());
      expect(result.current[0].connected).toBe(true);

      act(() => ws.simulateClose());
      expect(result.current[0].connected).toBe(false);
    });

    it("closes previous WebSocket when connecting to a new room", async () => {
      eveyFetchFn
        .mockResolvedValueOnce({ id: "room-1", code: "C1" })
        .mockResolvedValueOnce({ id: "room-2", code: "C2" });
      const { result } = await loadHook();

      await act(async () => {
        await result.current[1].createRoom("media-1");
      });
      const ws1 = latestWs();
      act(() => ws1.simulateOpen());

      await act(async () => {
        await result.current[1].createRoom("media-2");
      });

      expect(ws1.close).toHaveBeenCalled();
      expect(MockWebSocket.instances).toHaveLength(2);
    });

    it("cleans up WebSocket on unmount", async () => {
      eveyFetchFn.mockResolvedValueOnce({ id: "room-1", code: "C" });
      const { result, unmount } = await loadHook();

      await act(async () => {
        await result.current[1].createRoom("media-1");
      });
      const ws = latestWs();
      act(() => ws.simulateOpen());

      act(() => unmount());
      expect(ws.close).toHaveBeenCalled();
    });

    it("does not reconnect to the old room after switching rooms", async () => {
      vi.useFakeTimers();
      try {
        eveyFetchFn
          .mockResolvedValueOnce({ id: "room-1", code: "C1" })
          .mockResolvedValueOnce({ id: "room-2", code: "C2" });
        const { result } = await loadHook();

        await act(async () => {
          await result.current[1].createRoom("media-1");
        });
        act(() => latestWs().simulateOpen());
        // room-1 drops involuntarily -> schedules a reconnect timer to room-1
        act(() => latestWs().simulateClose());

        // switch to room-2 before that reconnect fires
        await act(async () => {
          await result.current[1].createRoom("media-2");
        });
        act(() => latestWs().simulateOpen());

        const countAfterSwitch = MockWebSocket.instances.length; // room-1 + room-2
        // fire any pending timers: a stale room-1 reconnect would open a 3rd ws
        act(() => {
          vi.advanceTimersByTime(60_000);
        });

        expect(MockWebSocket.instances).toHaveLength(countAfterSwitch);
        expect(latestWs().url).toContain("room-2");
      } finally {
        vi.useRealTimers();
      }
    });
  });
});
