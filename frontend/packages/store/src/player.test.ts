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

  it("stop clears _stopSessionFn; it must be re-armed before the next stop", () => {
    // Contract the player route relies on: stop() nulls _stopSessionFn after
    // firing, so an in-place episode change (no remount) MUST re-arm it (the
    // route re-runs setStopSessionFn on sessionId change) or the next stop()
    // can't tear down the new session, leaking it on the server.
    const fnA = vi.fn();
    const fnB = vi.fn();

    usePlayerStore.getState().play("mi-A");
    usePlayerStore.getState().setStopSessionFn(fnA);
    usePlayerStore.getState().stop(); // ends A
    expect(fnA).toHaveBeenCalledOnce();

    // Without a re-arm, a second stop fires nothing (fn was cleared).
    usePlayerStore.getState().play("mi-B");
    usePlayerStore.getState().stop();
    expect(fnB).not.toHaveBeenCalled();

    // Re-armed (the fix): the new session's stop fn now fires.
    usePlayerStore.getState().play("mi-B2");
    usePlayerStore.getState().setStopSessionFn(fnB);
    usePlayerStore.getState().stop();
    expect(fnB).toHaveBeenCalledOnce();
  });

  it("setMode updates mode", () => {
    usePlayerStore.getState().play("mi-1");
    usePlayerStore.getState().setMode("mini");
    expect(usePlayerStore.getState().mode).toBe("mini");
  });
});
