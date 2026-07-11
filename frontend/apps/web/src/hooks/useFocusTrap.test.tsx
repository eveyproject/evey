import { renderHook, act } from "@testing-library/react";
import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { useFocusTrap } from "./useFocusTrap";

// Helper: build a container with N buttons appended to document.body.
// Returns { container, buttons, cleanup }.
function buildContainer(count: number, containerTabIndex?: number) {
  const container = document.createElement("div");
  if (containerTabIndex !== undefined) {
    container.setAttribute("tabindex", String(containerTabIndex));
  }
  const buttons: HTMLButtonElement[] = [];
  for (let i = 0; i < count; i++) {
    const btn = document.createElement("button");
    btn.textContent = `Button ${i}`;
    container.appendChild(btn);
    buttons.push(btn);
  }
  document.body.appendChild(container);
  return {
    container,
    buttons,
    cleanup: () => {
      if (container.parentNode) container.parentNode.removeChild(container);
    },
  };
}

describe("useFocusTrap", () => {
  // Sentinel button placed outside the trap container - used as the
  // "previously focused" element to verify restore-on-deactivation.
  let sentinel: HTMLButtonElement;

  beforeEach(() => {
    sentinel = document.createElement("button");
    sentinel.textContent = "sentinel";
    document.body.appendChild(sentinel);
  });

  afterEach(() => {
    if (sentinel.parentNode) sentinel.parentNode.removeChild(sentinel);
  });

  it("focuses the first focusable child on activation", () => {
    const { container, buttons, cleanup } = buildContainer(3);

    const ref = { current: container };
    const { unmount } = renderHook(
      ({ active }: { active: boolean }) => useFocusTrap(ref, active),
      { initialProps: { active: true } },
    );

    expect(document.activeElement).toBe(buttons[0]);

    unmount();
    cleanup();
  });

  it("focuses the container itself when it has no focusable children", () => {
    // Container gets tabindex=-1 so it is programmatically focusable.
    const container = document.createElement("div");
    container.setAttribute("tabindex", "-1");
    document.body.appendChild(container);

    const ref = { current: container };
    const { unmount } = renderHook(
      ({ active }: { active: boolean }) => useFocusTrap(ref, active),
      { initialProps: { active: true } },
    );

    expect(document.activeElement).toBe(container);

    unmount();
    document.body.removeChild(container);
  });

  it("wraps Tab from last focusable child to first", () => {
    const { container, buttons, cleanup } = buildContainer(3);

    const ref = { current: container };
    renderHook(
      ({ active }: { active: boolean }) => useFocusTrap(ref, active),
      { initialProps: { active: true } },
    );

    // Manually move focus to the last button (simulating user tabbing there).
    act(() => {
      buttons[2].focus();
    });
    expect(document.activeElement).toBe(buttons[2]);

    // Dispatch Tab on window - the handler should wrap to first.
    act(() => {
      const event = new KeyboardEvent("keydown", {
        key: "Tab",
        bubbles: true,
        cancelable: true,
      });
      window.dispatchEvent(event);
    });

    expect(document.activeElement).toBe(buttons[0]);

    cleanup();
  });

  it("wraps Shift+Tab from first focusable child to last", () => {
    const { container, buttons, cleanup } = buildContainer(3);

    const ref = { current: container };
    renderHook(
      ({ active }: { active: boolean }) => useFocusTrap(ref, active),
      { initialProps: { active: true } },
    );

    // Focus is already on buttons[0] from activation; confirm.
    expect(document.activeElement).toBe(buttons[0]);

    // Dispatch Shift+Tab on window - should wrap to last.
    act(() => {
      const event = new KeyboardEvent("keydown", {
        key: "Tab",
        shiftKey: true,
        bubbles: true,
        cancelable: true,
      });
      window.dispatchEvent(event);
    });

    expect(document.activeElement).toBe(buttons[2]);

    cleanup();
  });

  it("restores focus to the element that was focused before activation", () => {
    const { container, cleanup } = buildContainer(2);

    // Focus the sentinel BEFORE activating the trap.
    sentinel.focus();
    expect(document.activeElement).toBe(sentinel);

    const ref = { current: container };
    const { rerender, unmount } = renderHook(
      ({ active }: { active: boolean }) => useFocusTrap(ref, active),
      { initialProps: { active: true } },
    );

    // Trap is now active - focus should have moved into the container.
    expect(document.activeElement).not.toBe(sentinel);

    // Deactivate - should restore focus to sentinel.
    act(() => {
      rerender({ active: false });
    });

    expect(document.activeElement).toBe(sentinel);

    unmount();
    cleanup();
  });

  it("restores focus on unmount while still active", () => {
    const { container, cleanup } = buildContainer(2);

    sentinel.focus();
    expect(document.activeElement).toBe(sentinel);

    const ref = { current: container };
    const { unmount } = renderHook(
      ({ active }: { active: boolean }) => useFocusTrap(ref, active),
      { initialProps: { active: true } },
    );

    expect(document.activeElement).not.toBe(sentinel);

    // Unmount while still active - cleanup should still restore focus.
    act(() => {
      unmount();
    });

    expect(document.activeElement).toBe(sentinel);

    cleanup();
  });
});
