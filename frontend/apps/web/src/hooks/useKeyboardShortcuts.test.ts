import { renderHook, act } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { fireEvent } from "@testing-library/react";

const navigateFn = vi.fn();

vi.mock("@tanstack/react-router", () => ({
  useNavigate: () => navigateFn,
}));

describe("useKeyboardShortcuts", () => {
  beforeEach(() => {
    navigateFn.mockClear();
    vi.resetModules();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("navigates to /search on / key when no input focused", async () => {
    const { useKeyboardShortcuts } = await import("./useKeyboardShortcuts");
    renderHook(() => useKeyboardShortcuts());

    act(() => {
      fireEvent.keyDown(window, { key: "/" });
    });

    expect(navigateFn).toHaveBeenCalledWith({ to: "/search" });
  });

  it("navigates to /home on h key", async () => {
    const { useKeyboardShortcuts } = await import("./useKeyboardShortcuts");
    renderHook(() => useKeyboardShortcuts());

    act(() => {
      fireEvent.keyDown(window, { key: "h" });
    });

    expect(navigateFn).toHaveBeenCalledWith({ to: "/home" });
  });

  it("toggles showHelp on ? key", async () => {
    const { useKeyboardShortcuts } = await import("./useKeyboardShortcuts");
    const { result } = renderHook(() => useKeyboardShortcuts());

    expect(result.current.showHelp).toBe(false);

    act(() => {
      fireEvent.keyDown(window, { key: "?" });
    });

    expect(result.current.showHelp).toBe(true);

    act(() => {
      fireEvent.keyDown(window, { key: "?" });
    });

    expect(result.current.showHelp).toBe(false);
  });

  it("does not navigate when an input is focused", async () => {
    const { useKeyboardShortcuts } = await import("./useKeyboardShortcuts");
    renderHook(() => useKeyboardShortcuts());

    const input = document.createElement("input");
    document.body.appendChild(input);
    input.focus();

    act(() => {
      fireEvent.keyDown(window, { key: "h" });
    });

    expect(navigateFn).not.toHaveBeenCalled();

    input.blur();
    document.body.removeChild(input);
  });

  it("blurs focused input on Escape", async () => {
    const { useKeyboardShortcuts } = await import("./useKeyboardShortcuts");
    renderHook(() => useKeyboardShortcuts());

    const input = document.createElement("input");
    document.body.appendChild(input);
    input.focus();

    const blurSpy = vi.spyOn(input, "blur");

    act(() => {
      fireEvent.keyDown(window, { key: "Escape" });
    });

    expect(blurSpy).toHaveBeenCalled();

    document.body.removeChild(input);
  });

  it("removes event listener on unmount", async () => {
    const { useKeyboardShortcuts } = await import("./useKeyboardShortcuts");
    const removeSpy = vi.spyOn(window, "removeEventListener");

    const { unmount } = renderHook(() => useKeyboardShortcuts());
    unmount();

    expect(removeSpy).toHaveBeenCalledWith("keydown", expect.any(Function));
  });
});
