import { describe, it, expect, vi, beforeEach } from "vitest";
import { usePlayerStore } from "./player";

describe("usePlayerStore", () => {
  beforeEach(() => {
    usePlayerStore.getState().stop();
  });

  it("starts inactive", () => {
    const state = usePlayerStore.getState();
    expect(state.isActive).toBe(false);
    expect(state.mediaId).toBeNull();
    expect(state.mode).toBe("hidden");
  });

  it("play sets mediaId and mode=full", () => {
    usePlayerStore.getState().play("mi-1", "Test Movie");
    const state = usePlayerStore.getState();
    expect(state.mediaId).toBe("mi-1");
    expect(state.mediaTitle).toBe("Test Movie");
    expect(state.mode).toBe("full");
    expect(state.isActive).toBe(true);
  });

  it("play same mediaId just switches to full", () => {
    usePlayerStore.getState().play("mi-1");
    usePlayerStore.getState().setMode("mini");
    usePlayerStore.getState().play("mi-1");
    const state = usePlayerStore.getState();
    expect(state.mode).toBe("full");
    expect(state.sessionId).toBeNull();
  });

  it("play different mediaId resets session", () => {
    usePlayerStore.getState().play("mi-1");
    usePlayerStore.getState().setSessionId("s-1");
    usePlayerStore.getState().play("mi-2");
    const state = usePlayerStore.getState();
    expect(state.mediaId).toBe("mi-2");
    expect(state.sessionId).toBeNull();
  });

  it("stop resets everything", () => {
    usePlayerStore.getState().play("mi-1");
    usePlayerStore.getState().setSessionId("s-1");
    usePlayerStore.getState().stop();
    const state = usePlayerStore.getState();
    expect(state.isActive).toBe(false);
    expect(state.mediaId).toBeNull();
    expect(state.sessionId).toBeNull();
    expect(state.mode).toBe("hidden");
  });

  it("stop calls _stopSessionFn", () => {
    const fn = vi.fn();
    usePlayerStore.getState().play("mi-1");
    usePlayerStore.getState().setStopSessionFn(fn);
    usePlayerStore.getState().stop();
    expect(fn).toHaveBeenCalledOnce();
  });

  it("setMode updates mode", () => {
    usePlayerStore.getState().play("mi-1");
    usePlayerStore.getState().setMode("mini");
    expect(usePlayerStore.getState().mode).toBe("mini");
  });
});
