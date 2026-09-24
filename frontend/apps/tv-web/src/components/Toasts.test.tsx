import { describe, expect, it, vi, afterEach } from "vitest";
import { act, render, screen } from "@testing-library/react";
import { useToastStore } from "@evey/store";
import { Toasts } from "./Toasts";

afterEach(() => { vi.useRealTimers(); useToastStore.setState({ toasts: [] }); });

// The store keeps a toast until a display clears it: the /tv never did, and
// every toast stayed on screen for good.
describe("toasts", () => {
  it("clears each toast after its duration, 3 s by default", () => {
    vi.useFakeTimers();
    render(<Toasts />);
    act(() => {
      useToastStore.getState().addToast({ type: "success", message: "short" });
      useToastStore.getState().addToast({ type: "error", message: "long", duration: 6000 });
    });
    expect(screen.getByText("short")).toBeTruthy();
    act(() => { vi.advanceTimersByTime(3100); });
    expect(screen.queryByText("short")).toBeNull();
    expect(screen.getByText("long")).toBeTruthy();
    act(() => { vi.advanceTimersByTime(3000); });
    expect(screen.queryByText("long")).toBeNull();
  });
});
