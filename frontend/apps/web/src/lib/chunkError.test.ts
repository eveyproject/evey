import { describe, it, expect, beforeEach, vi, afterEach } from "vitest";
import {
  errorMessage,
  isChunkLoadError,
  reloadOnceForChunkError,
  resetPreloadErrorForTests,
} from "./chunkError";

describe("chunkError", () => {
  beforeEach(() => {
    resetPreloadErrorForTests();
    sessionStorage.clear();
  });

  describe("isChunkLoadError", () => {
    it("recognises the ChunkLoadError name", () => {
      expect(isChunkLoadError(Object.assign(new Error("boom"), { name: "ChunkLoadError" }))).toBe(true);
    });

    it("recognises the per-engine wordings of a failed module import", () => {
      // Firefox / Chromium / WebKit / webpack-era bundles, in that order.
      expect(isChunkLoadError(new Error("error loading dynamically imported module: /assets/x.js"))).toBe(true);
      expect(isChunkLoadError(new Error("Failed to fetch dynamically imported module"))).toBe(true);
      expect(isChunkLoadError(new Error("Importing a module script failed."))).toBe(true);
      expect(isChunkLoadError(new Error("Loading chunk 42 failed"))).toBe(true);
    });

    it("says no to an ordinary error", () => {
      expect(isChunkLoadError(new Error("cannot read profile"))).toBe(false);
    });

    it("does not throw on the shapeless values React actually hands over", () => {
      // The whole bug: `undefined` reached componentDidCatch and `.name` was
      // read off it. Every one of these must answer, not crash.
      expect(isChunkLoadError(undefined)).toBe(false);
      expect(isChunkLoadError(null)).toBe(false);
      expect(isChunkLoadError("boom")).toBe(false);
      expect(isChunkLoadError(42)).toBe(false);
    });

    it("recognises an undefined error when Vite just reported a preload failure", () => {
      window.dispatchEvent(new Event("vite:preloadError"));
      expect(isChunkLoadError(undefined)).toBe(true);
    });

    it("still judges a shaped error on its own evidence after a preload failure", () => {
      // A crash carrying its own message is not a chunk failure just because one
      // happened moments earlier - otherwise every later bug would be reloaded away.
      window.dispatchEvent(new Event("vite:preloadError"));
      expect(isChunkLoadError(new Error("cannot read profile"))).toBe(false);
    });

    it("forgets a preload failure once it is stale", () => {
      vi.useFakeTimers();
      try {
        window.dispatchEvent(new Event("vite:preloadError"));
        vi.advanceTimersByTime(11_000);
        expect(isChunkLoadError(undefined)).toBe(false);
      } finally {
        vi.useRealTimers();
      }
    });
  });

  describe("errorMessage", () => {
    it("reads a message when there is one and stays quiet otherwise", () => {
      expect(errorMessage(new Error("nope"))).toBe("nope");
      expect(errorMessage(undefined)).toBe("");
      expect(errorMessage(null)).toBe("");
      expect(errorMessage("nope")).toBe("");
    });
  });

  describe("reloadOnceForChunkError", () => {
    const reload = vi.fn();

    beforeEach(() => {
      reload.mockClear();
      Object.defineProperty(window, "location", { value: { reload, href: "/" }, writable: true });
    });

    it("reloads the first time and refuses to loop on the second", () => {
      expect(reloadOnceForChunkError()).toBe(true);
      expect(reload).toHaveBeenCalledOnce();

      expect(reloadOnceForChunkError()).toBe(false);
      expect(reload).toHaveBeenCalledOnce();
    });

    it("does not reload when sessionStorage is unusable", () => {
      // Firefox with cookies blocked: reading storage throws, so the loop guard
      // cannot be trusted and a reload loop would be worse than one error screen.
      const original = Object.getOwnPropertyDescriptor(globalThis, "sessionStorage");
      Object.defineProperty(globalThis, "sessionStorage", {
        get() {
          throw new Error("storage denied");
        },
        configurable: true,
      });
      try {
        expect(reloadOnceForChunkError()).toBe(false);
        expect(reload).not.toHaveBeenCalled();
      } finally {
        if (original) Object.defineProperty(globalThis, "sessionStorage", original);
      }
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });
});

// The navigation-time hole: a route chunk that fails while the router loads it
// never reaches an error boundary - the transition aborts and the page stays,
// so the click reads as dead. The vite:preloadError listener itself must
// reload (once), not merely record evidence for a boundary that will never run.
describe("navigation-time chunk failure", () => {
  beforeEach(() => {
    // Its own clean session: the once-per-session key must not be inherited
    // from whatever the tests above left behind.
    resetPreloadErrorForTests();
    sessionStorage.clear();
  });

  it("reloads once straight from the vite:preloadError event", () => {
    const reloadMock = vi.fn();
    Object.defineProperty(window, "location", {
      value: { reload: reloadMock, href: "/" },
      writable: true,
    });
    window.dispatchEvent(new Event("vite:preloadError"));
    expect(reloadMock).toHaveBeenCalledOnce();
    // A second failure in the same session must NOT loop.
    window.dispatchEvent(new Event("vite:preloadError"));
    expect(reloadMock).toHaveBeenCalledOnce();
  });
});
