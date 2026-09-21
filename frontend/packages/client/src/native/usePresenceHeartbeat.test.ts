import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";

const heartbeat = vi.fn().mockResolvedValue({ onlineCount: 1 });
vi.mock("@evey/api", () => ({
  presenceServiceHeartbeat: (...args: unknown[]) => heartbeat(...args),
}));

type AppStateListener = (state: string) => void;
let appStateListener: AppStateListener | null = null;
const removeSub = vi.fn();
vi.mock("react-native", () => ({
  AppState: {
    addEventListener: (_event: string, cb: AppStateListener) => {
      appStateListener = cb;
      return { remove: removeSub };
    },
  },
}));

import { usePresenceHeartbeat } from "./usePresenceHeartbeat";

beforeEach(() => {
  vi.useFakeTimers();
  heartbeat.mockClear();
  removeSub.mockClear();
  appStateListener = null;
});

afterEach(() => {
  vi.useRealTimers();
});

describe("usePresenceHeartbeat", () => {
  it("beats immediately and then every 30s while the app is active", async () => {
    renderHook(() => usePresenceHeartbeat(true));

    expect(heartbeat).toHaveBeenCalledTimes(1);
    await act(async () => {
      vi.advanceTimersByTime(30_000);
    });
    expect(heartbeat).toHaveBeenCalledTimes(2);
  });

  // A pocketed phone is NOT online: stopping the beats lets the server's 90s
  // window drop the row, which is the honest state. (The web tab does the
  // same via document.visibilityState.)
  it("stops beating when the app goes to background, resumes when active", async () => {
    renderHook(() => usePresenceHeartbeat(true));
    expect(heartbeat).toHaveBeenCalledTimes(1);

    act(() => {
      appStateListener?.("background");
    });
    await act(async () => {
      vi.advanceTimersByTime(120_000);
    });
    expect(heartbeat).toHaveBeenCalledTimes(1);

    act(() => {
      appStateListener?.("active");
    });
    expect(heartbeat).toHaveBeenCalledTimes(2);
  });

  // The layout mounts this hook before its auth early-returns (hooks can't
  // sit after them), so the hook itself must gate on enabled: beating without
  // a token would 401 into the unauthorized flow.
  it("does not beat when disabled, starts once enabled", async () => {
    const { rerender } = renderHook(({ on }) => usePresenceHeartbeat(on), {
      initialProps: { on: false },
    });
    expect(heartbeat).not.toHaveBeenCalled();

    rerender({ on: true });
    expect(heartbeat).toHaveBeenCalledTimes(1);
  });

  it("cleans up its interval and subscription on unmount", async () => {
    const { unmount } = renderHook(() => usePresenceHeartbeat(true));
    unmount();
    await act(async () => {
      vi.advanceTimersByTime(120_000);
    });
    expect(heartbeat).toHaveBeenCalledTimes(1);
    expect(removeSub).toHaveBeenCalled();
  });
});
