import { renderHook, act } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { useDocumentPiP } from "./useDocumentPiP";

// Minimal fake PiP window: a real Document/body pair (jsdom can create these)
// plus a fake `close()` that synchronously fires `pagehide`, mirroring real
// browser behavior closely enough for our purposes while letting the test
// control the timing explicitly instead of racing a real async event.
function installFakeDocumentPiP() {
  const pipDocument = document.implementation.createHTMLDocument("pip");
  const listeners: Record<string, Array<() => void>> = {};
  const closeSpy = vi.fn(() => {
    for (const fn of listeners.pagehide ?? []) fn();
  });

  const pipWindow = {
    document: pipDocument,
    close: closeSpy,
    addEventListener: (type: string, fn: () => void) => {
      (listeners[type] ??= []).push(fn);
    },
  } as unknown as Window;

  window.documentPictureInPicture = {
    requestWindow: vi.fn().mockResolvedValue(pipWindow),
    window: pipWindow,
  };

  return { pipWindow, closeSpy };
}

describe("useDocumentPiP", () => {
  let container: HTMLElement;
  let parent: HTMLElement;

  beforeEach(() => {
    parent = document.createElement("div");
    container = document.createElement("div");
    container.setAttribute("data-player-container", "");
    parent.appendChild(container);
    document.body.appendChild(parent);
  });

  afterEach(() => {
    delete (window as unknown as { documentPictureInPicture?: unknown }).documentPictureInPicture;
    parent.remove();
    vi.restoreAllMocks();
  });

  it("restores the container to its original parent and closes the PiP window when the hook unmounts while open", async () => {
    const { closeSpy } = installFakeDocumentPiP();
    const { result, unmount } = renderHook(() => useDocumentPiP());

    await act(async () => {
      await result.current.toggle();
    });

    expect(result.current.isOpen).toBe(true);
    // Container really moved out of its original parent.
    expect(container.parentNode).not.toBe(parent);

    // Unmounting must not throw (this is the removeChild crash we're
    // guarding against) and must restore the node synchronously, before
    // React's own removeChild runs.
    expect(() => unmount()).not.toThrow();

    expect(container.parentNode).toBe(parent);
    expect(closeSpy).toHaveBeenCalledTimes(1);
  });

  it("restore is idempotent: pagehide firing after the unmount cleanup already restored is a no-op", async () => {
    const { pipWindow } = installFakeDocumentPiP();
    const { result, unmount } = renderHook(() => useDocumentPiP());

    await act(async () => {
      await result.current.toggle();
    });

    unmount();
    expect(container.parentNode).toBe(parent);

    // Simulate the async pagehide event arriving late (after unmount already
    // restored + closed). Must not move the node again or throw.
    expect(() => pipWindow.close()).not.toThrow();
    expect(container.parentNode).toBe(parent);
  });

  it("close() via toggle() restores the node during a normal close (not just unmount)", async () => {
    installFakeDocumentPiP();
    const { result } = renderHook(() => useDocumentPiP());

    await act(async () => {
      await result.current.toggle();
    });
    expect(result.current.isOpen).toBe(true);

    await act(async () => {
      await result.current.toggle();
    });

    expect(result.current.isOpen).toBe(false);
    expect(container.parentNode).toBe(parent);
  });
});
