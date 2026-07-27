import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { onlineManager } from "@tanstack/react-query";
import { readNavigatorOnline, setupOnlineManager } from "./online-manager";

/** Overrides navigator.onLine, which jsdom otherwise defines read-only-ish via a getter. */
function setNavigatorOnline(value: boolean | undefined): void {
  Object.defineProperty(navigator, "onLine", { value, configurable: true });
}

let unsubscribe: (() => void) | undefined;

beforeEach(() => {
  onlineManager.setOnline(true);
});

afterEach(() => {
  unsubscribe?.();
  unsubscribe = undefined;
  setNavigatorOnline(true);
});

describe("readNavigatorOnline", () => {
  it("reflects navigator.onLine when present", () => {
    setNavigatorOnline(true);
    expect(readNavigatorOnline()).toBe(true);
    setNavigatorOnline(false);
    expect(readNavigatorOnline()).toBe(false);
  });

  it("defaults to true when navigator.onLine is absent", () => {
    setNavigatorOnline(undefined);
    expect(readNavigatorOnline()).toBe(true);
  });
});

describe("setupOnlineManager", () => {
  it("seeds the initial state as offline when the browser already is", () => {
    setNavigatorOnline(false);
    unsubscribe = onlineManager.subscribe(() => {});
    setupOnlineManager();
    expect(onlineManager.isOnline()).toBe(false);
  });

  it("transitions online -> offline -> online on window events", () => {
    setNavigatorOnline(true);
    unsubscribe = onlineManager.subscribe(() => {});
    setupOnlineManager();
    expect(onlineManager.isOnline()).toBe(true);

    window.dispatchEvent(new Event("offline"));
    expect(onlineManager.isOnline()).toBe(false);

    window.dispatchEvent(new Event("online"));
    expect(onlineManager.isOnline()).toBe(true);
  });

  it("tears down its listeners when the last subscriber leaves", () => {
    const removeSpy = vi.spyOn(window, "removeEventListener");
    unsubscribe = onlineManager.subscribe(() => {});
    setupOnlineManager();
    unsubscribe();
    unsubscribe = undefined;
    expect(removeSpy).toHaveBeenCalledWith("online", expect.any(Function));
    expect(removeSpy).toHaveBeenCalledWith("offline", expect.any(Function));
    removeSpy.mockRestore();
  });
});
