// Regression test for the zero-per-frame-allocation review fix: drives many
// simulated animation frames past the entrance animation's settle point and
// asserts createRadialGradient() (and therefore the per-node color template
// literals that used to feed it) is called only a handful of times - once
// per node while it eases in, never once per node per frame forever.

import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";

class FakeStyle {
  cssText = "";
  [key: string]: unknown;
}

class FakeElement {
  tagName: string;
  style = new FakeStyle();
  children: FakeElement[] = [];
  parentNode: FakeElement | null = null;
  clientWidth = 800;
  clientHeight = 480;
  width = 0;
  height = 0;
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
    if (kind === "2d") return mockCtx;
    return null;
  }
}

let gradientCallCount = 0;
const mockCtx: Record<string, unknown> = {
  globalAlpha: 1,
  shadowBlur: 0,
  shadowColor: "",
  fillStyle: "",
  clearRect: () => {},
  beginPath: () => {},
  arc: () => {},
  fill: () => {},
  createRadialGradient: () => {
    gradientCallCount++;
    return { addColorStop: () => {} };
  },
};

function installDomStubs() {
  const documentElement = new FakeElement("html");
  const head = new FakeElement("head");
  const body = new FakeElement("body");

  const doc = {
    documentElement,
    head,
    body,
    hidden: false,
    createElement: (tag: string) => new FakeElement(tag),
    addEventListener: () => {},
    removeEventListener: () => {},
  };

  let pendingFrame: ((ts: number) => void) | null = null;
  const win: Record<string, unknown> = {
    document: doc,
    innerWidth: 800,
    innerHeight: 600,
    devicePixelRatio: 1,
    CSS: { supports: () => false },
    matchMedia: () => ({ matches: false }),
    addEventListener: () => {},
    removeEventListener: () => {},
    requestAnimationFrame: (cb: (ts: number) => void) => {
      pendingFrame = cb;
      return 1;
    },
    cancelAnimationFrame: () => {
      pendingFrame = null;
    },
    setInterval: () => 0,
    clearInterval: () => {},
    setTimeout: (fn: () => void, ms: number) => setTimeout(fn, ms) as unknown as number,
    evey: undefined,
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

  return {
    doc,
    win,
    // Drives whatever frame the renderer currently has scheduled, advancing
    // the clock by ~1 frame at 60fps (matches requestAnimationFrame's real
    // timestamp contract: monotonically increasing milliseconds).
    driveFrame: (ts: number) => {
      const cb = pendingFrame;
      pendingFrame = null;
      cb?.(ts);
    },
  };
}

describe("CanvasRenderer gradient caching (zero per-frame allocation once settled)", () => {
  beforeEach(() => {
    gradientCallCount = 0;
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("stops calling createRadialGradient once the entrance animation has settled", async () => {
    vi.resetModules();
    const { driveFrame } = installDomStubs();
    const { CanvasRenderer } = await import("../src/adapters/render/CanvasRenderer");

    const renderer = new CanvasRenderer();
    const host = document.createElement("div") as unknown as FakeElement;
    renderer.mount(host as unknown as HTMLElement);

    const nodes = [
      { id: "decade-1990", decade: 1990, count: 5, share: 0.5, x: -0.4, y: 0.1, radius: 0.15, representative: [] },
      { id: "decade-2000", decade: 2000, count: 5, share: 0.5, x: 0.4, y: -0.2, radius: 0.18, representative: [] },
    ];
    renderer.setScene({ nodes });

    // Drive well past the settle point (opacity 0->1 with
    // ENTER_EASE_PER_60FPS=0.1 needs ~73 frames at 60fps to fall under the
    // settle epsilon; give it a generous margin) at a steady 16.67ms cadence.
    // Every one of these frames is a legitimate "real change" (the entrance
    // animation is actually moving/fading the node in), so the gradient is
    // expected to be rebuilt repeatedly here - that's not the regression.
    let ts = 0;
    for (let i = 0; i < 150; i++) {
      ts += 16.67;
      driveFrame(ts);
    }
    const gradientCallsAfterSettling = gradientCallCount;
    expect(gradientCallsAfterSettling).toBeGreaterThan(0); // built while easing in

    // The actual regression proof: once settled, the glow keeps "breathing"
    // (shadowBlur/globalAlpha change every frame) but position/radius no
    // longer do, so 150 more idle frames must add ZERO further
    // createRadialGradient calls - the cached gradient is reused untouched.
    for (let i = 0; i < 150; i++) {
      ts += 16.67;
      driveFrame(ts);
    }
    expect(gradientCallCount).toBe(gradientCallsAfterSettling);

    renderer.dispose();
  });
});
