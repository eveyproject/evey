import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, act } from "@testing-library/react";
import { PresenceHeartbeat } from "./PresenceHeartbeat";

const beat = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", () => ({ presenceServiceHeartbeat: (...a: unknown[]) => beat(...a) }));

function setVisibility(state: "visible" | "hidden") {
  Object.defineProperty(document, "visibilityState", { value: state, configurable: true });
}

beforeEach(() => {
  vi.useFakeTimers();
  beat.mockReset().mockResolvedValue({});
  setVisibility("visible");
});
afterEach(() => vi.useRealTimers());

// One beat on mount, then every 30 s (inside the server's 90 s window), none
// while the tab is hidden, one straight away when it becomes visible again;
// a failed beat is swallowed; unmount stops the clock.
describe("PresenceHeartbeat", () => {
  it("beats on mount and every 30 s while visible", () => {
    const { unmount } = render(<PresenceHeartbeat />);
    expect(beat).toHaveBeenCalledTimes(1);
    act(() => vi.advanceTimersByTime(30_000));
    expect(beat).toHaveBeenCalledTimes(2);
    act(() => vi.advanceTimersByTime(29_999));
    expect(beat).toHaveBeenCalledTimes(2);
    unmount();
    act(() => vi.advanceTimersByTime(120_000));
    expect(beat).toHaveBeenCalledTimes(2);
  });

  it("stays silent while hidden and beats at once on becoming visible", () => {
    render(<PresenceHeartbeat />);
    setVisibility("hidden");
    act(() => vi.advanceTimersByTime(60_000));
    expect(beat).toHaveBeenCalledTimes(1);
    setVisibility("visible");
    act(() => {
      document.dispatchEvent(new Event("visibilitychange"));
    });
    expect(beat).toHaveBeenCalledTimes(2);
  });

  it("swallows a failed beat and keeps going", async () => {
    beat.mockRejectedValueOnce(new Error("offline"));
    render(<PresenceHeartbeat />);
    await act(async () => {});
    act(() => vi.advanceTimersByTime(30_000));
    expect(beat).toHaveBeenCalledTimes(2);
  });
});
