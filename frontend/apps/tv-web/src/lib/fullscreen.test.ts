import { describe, expect, it, vi } from "vitest";
import { keepFullscreen, wantsFullscreen } from "./fullscreen";

function fakeDoc(over: { enabled?: boolean; current?: Element | null; request?: () => Promise<void> } = {}) {
  return {
    fullscreenEnabled: over.enabled ?? true,
    fullscreenElement: over.current ?? null,
    documentElement: { requestFullscreen: over.request ?? (() => Promise.resolve()) },
  };
}

describe("wantsFullscreen", () => {
  it("asks on the page served under /tv", () => {
    expect(wantsFullscreen("/tv/", fakeDoc())).toBe(true);
    expect(wantsFullscreen("/tv", fakeDoc())).toBe(true);
    expect(wantsFullscreen("/tv/index.html", fakeDoc())).toBe(true);
  });

  it("leaves anything else alone: the packaged application, the dev server, a lookalike path", () => {
    expect(wantsFullscreen("/media/apps/evey/index.html", fakeDoc())).toBe(false);
    expect(wantsFullscreen("/", fakeDoc())).toBe(false);
    expect(wantsFullscreen("/tvshows", fakeDoc())).toBe(false);
  });

  it("does not ask twice, nor where the browser cannot", () => {
    expect(wantsFullscreen("/tv/", fakeDoc({ current: {} as Element }))).toBe(false);
    expect(wantsFullscreen("/tv/", fakeDoc({ enabled: false }))).toBe(false);
  });
});

describe("keepFullscreen", () => {
  function fakeWin(pathname: string, doc: ReturnType<typeof fakeDoc>) {
    const target = new EventTarget();
    return Object.assign(target, { document: doc, location: { pathname } }) as unknown as Window;
  }

  it("goes full screen on the first key and again after the viewer left it", async () => {
    const request = vi.fn(() => Promise.resolve());
    const doc = fakeDoc({ request });
    const win = fakeWin("/tv/", doc);
    keepFullscreen(win);

    win.dispatchEvent(new Event("keydown"));
    expect(request).toHaveBeenCalledTimes(1);

    doc.fullscreenElement = {} as Element;
    win.dispatchEvent(new Event("click"));
    expect(request).toHaveBeenCalledTimes(1);

    doc.fullscreenElement = null;
    win.dispatchEvent(new Event("keydown"));
    expect(request).toHaveBeenCalledTimes(2);
  });

  it("swallows a refusal", async () => {
    const request = vi.fn(() => Promise.reject(new TypeError("not allowed")));
    const win = fakeWin("/tv/", fakeDoc({ request }));
    keepFullscreen(win);
    win.dispatchEvent(new Event("keydown"));
    await Promise.resolve();
    expect(request).toHaveBeenCalledTimes(1);
  });

  it("never asks outside /tv", () => {
    const request = vi.fn(() => Promise.resolve());
    const win = fakeWin("/", fakeDoc({ request }));
    keepFullscreen(win);
    win.dispatchEvent(new Event("keydown"));
    expect(request).not.toHaveBeenCalled();
  });
});
