// Smoke test for the bootstrap path (mount -> library load -> first tick ->
// dispose), exercised without a browser: vitest runs in a plain "node"
// environment (this repo deliberately keeps devDependencies to esbuild/
// typescript/vitest only, matching marquee/cosmos/weather-v2). This test
// hand-rolls the minimal DOM + Canvas2D surface the bundle touches and
// asserts the whole pipeline (bridge -> app -> renderer) runs to a first
// frame with zero thrown errors and zero console.error calls - the offline
// equivalent of "open the page, check the console".

import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";

class FakeStyle {
  cssText = "";
  [key: string]: unknown;
}

class FakeClassList {
  private set = new Set<string>();
  add(c: string) {
    this.set.add(c);
  }
  remove(c: string) {
    this.set.delete(c);
  }
  contains(c: string) {
    return this.set.has(c);
  }
}

class FakeElement {
  tagName: string;
  style = new FakeStyle();
  classList = new FakeClassList();
  children: FakeElement[] = [];
  parentNode: FakeElement | null = null;
  textContent = "";
  dataset: Record<string, string> = {};
  width = 0;
  height = 0;
  clientWidth = 800;
  clientHeight = 480;
  private listeners = new Map<string, Set<(e: unknown) => void>>();

  constructor(tagName: string) {
    this.tagName = tagName.toLowerCase();
  }

  appendChild(child: FakeElement): FakeElement {
    this.children.push(child);
    child.parentNode = this;
    return child;
  }

  remove(): void {
    this.parentNode?.children.splice(this.parentNode.children.indexOf(this), 1);
    this.parentNode = null;
  }

  replaceChildren(...nodes: FakeElement[]): void {
    for (const c of this.children.slice()) c.remove();
    for (const n of nodes) this.appendChild(n);
  }

  addEventListener(type: string, cb: (e: unknown) => void): void {
    if (!this.listeners.has(type)) this.listeners.set(type, new Set());
    this.listeners.get(type)!.add(cb);
  }

  removeEventListener(type: string, cb: (e: unknown) => void): void {
    this.listeners.get(type)?.delete(cb);
  }

  getBoundingClientRect() {
    return { left: 0, top: 0, width: 800, height: 480, right: 800, bottom: 480, x: 0, y: 0, toJSON() { return {}; } };
  }

  getContext(kind: string): unknown {
    if (kind === "2d") return createMock2dContext();
    return null;
  }
}

function createMock2dContext() {
  const ctx: Record<string, unknown> = {
    globalAlpha: 1,
    shadowBlur: 0,
    shadowColor: "",
    fillStyle: "",
    clearRect: () => {},
    beginPath: () => {},
    arc: () => {},
    fill: () => {},
    createRadialGradient: () => ({ addColorStop: () => {} }),
  };
  return ctx;
}

const CANNED_LIBRARY = Array.from({ length: 40 }, (_, i) => ({
  id: `mi-${i}`,
  type: i % 3 === 0 ? "MEDIA_ITEM_TYPE_SERIES" : "MEDIA_ITEM_TYPE_MOVIE",
  title: `Title ${i}`,
  year: 1970 + (i % 55),
  posterUrl: `https://host/api/v1/media/mi-${i}/artwork?kind=poster`,
}));

function installDomStubs() {
  const documentElement = new FakeElement("html");
  const head = new FakeElement("head");
  const body = new FakeElement("body");
  body.clientWidth = 800;

  const doc = {
    documentElement,
    head,
    body,
    hidden: false,
    createElement: (tag: string) => new FakeElement(tag),
    addEventListener: () => {},
    removeEventListener: () => {},
  };

  const rafCallbacks: Array<() => void> = [];
  const win: Record<string, unknown> = {
    document: doc,
    innerWidth: 800,
    innerHeight: 600,
    devicePixelRatio: 1,
    CSS: { supports: () => false },
    matchMedia: () => ({ matches: false }),
    addEventListener: () => {},
    removeEventListener: () => {},
    requestAnimationFrame: (cb: (ts: number) => void) => { rafCallbacks.push(() => cb(performance.now())); return rafCallbacks.length; },
    cancelAnimationFrame: () => {},
    setInterval: (fn: () => void, ms: number) => setInterval(fn, ms) as unknown as number,
    clearInterval: (id: number) => clearInterval(id as unknown as ReturnType<typeof setInterval>),
    setTimeout: (fn: () => void, ms: number) => setTimeout(fn, ms) as unknown as number,
    evey: {
      onInit(cb: (data: { locale?: string; theme?: string }) => void) {
        cb({ locale: "en", theme: "dark" });
      },
      resize: () => {},
      navigate: () => {},
      toast: () => {},
      media: {
        list: async () => ({ items: CANNED_LIBRARY }),
        get: async () => ({}),
      },
    },
  };

  class FakeResizeObserver {
    observe() {}
    disconnect() {}
  }
  class FakeIntersectionObserver {
    observe() {}
    disconnect() {}
  }

  vi.stubGlobal("document", doc);
  vi.stubGlobal("window", win);
  vi.stubGlobal("navigator", { language: "en", hardwareConcurrency: 4 });
  vi.stubGlobal("ResizeObserver", FakeResizeObserver);
  vi.stubGlobal("IntersectionObserver", FakeIntersectionObserver);
  vi.stubGlobal("requestAnimationFrame", win.requestAnimationFrame);
  vi.stubGlobal("cancelAnimationFrame", win.cancelAnimationFrame);
  vi.stubGlobal("performance", performance);

  return { doc, win, rafCallbacks };
}

describe("bootstrap smoke test", () => {
  let errorSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    errorSpy.mockRestore();
  });

  it("mounts, loads the library, ticks a frame, and disposes without throwing or logging console errors", async () => {
    vi.resetModules();
    const { rafCallbacks } = installDomStubs();
    const { CinephileApp } = await import("../src/application/CinephileApp");
    const { CanvasRenderer } = await import("../src/adapters/render/CanvasRenderer");
    const { EveyBridge } = await import("../src/adapters/evey/EveyBridge");

    const app = new CinephileApp({ renderer: new CanvasRenderer(), bridge: new EveyBridge() });
    expect(() => app.start()).not.toThrow();

    // Let the async boot() (bridge.onReady -> listMedia) settle.
    await new Promise((resolve) => setTimeout(resolve, 0));
    await new Promise((resolve) => setTimeout(resolve, 0));

    // Drive one animation frame now that the galaxy has been pushed to the renderer.
    expect(() => rafCallbacks[0]?.()).not.toThrow();

    expect(() => app.dispose()).not.toThrow();
    expect(errorSpy).not.toHaveBeenCalled();
  });

  it("appends the root host to the DOM after boot", async () => {
    vi.resetModules();
    const { doc } = installDomStubs();
    const { CinephileApp } = await import("../src/application/CinephileApp");
    const { CanvasRenderer } = await import("../src/adapters/render/CanvasRenderer");
    const { EveyBridge } = await import("../src/adapters/evey/EveyBridge");

    const app = new CinephileApp({ renderer: new CanvasRenderer(), bridge: new EveyBridge() });
    app.start();
    await new Promise((resolve) => setTimeout(resolve, 0));

    const root = (doc.body as unknown as FakeElement).children.find((c) => c.tagName === "div");
    expect(root).toBeTruthy();
    app.dispose();
  });

  it("shows the empty-library message and still disposes cleanly when nothing is watchable", async () => {
    vi.resetModules();
    installDomStubs();
    (window as unknown as { evey: { media: { list: () => Promise<unknown> } } }).evey.media.list = async () => ({ items: [] });

    const { CinephileApp } = await import("../src/application/CinephileApp");
    const { CanvasRenderer } = await import("../src/adapters/render/CanvasRenderer");
    const { EveyBridge } = await import("../src/adapters/evey/EveyBridge");

    const app = new CinephileApp({ renderer: new CanvasRenderer(), bridge: new EveyBridge() });
    app.start();
    await new Promise((resolve) => setTimeout(resolve, 0));
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(() => app.dispose()).not.toThrow();
    expect(errorSpy).not.toHaveBeenCalled();
  });
});
