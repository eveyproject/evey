// Smoke test for the bootstrap path (mount -> first push -> dispose),
// exercised without a browser: vitest runs in a plain "node" environment
// (this repo deliberately keeps devDependencies to esbuild/typescript/vitest
// only, matching weather-v2/cosmos, so a real jsdom/Playwright environment is
// not available here). This test hand-rolls the minimal DOM + WebGL surface
// the bundle touches and asserts the whole pipeline (bridge -> app ->
// renderer) runs to a first frame with zero thrown errors and zero
// console.error calls - the offline equivalent of "open the page, check the
// console".

import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";

class FakeStyle {
  cssText = "";
  [key: string]: unknown;
}

class FakeClassList {
  private set = new Set<string>();
  add(c: string) { this.set.add(c); }
  remove(c: string) { this.set.delete(c); }
  contains(c: string) { return this.set.has(c); }
}

class FakeElement {
  tagName: string;
  style = new FakeStyle();
  classList = new FakeClassList();
  children: FakeElement[] = [];
  parentNode: FakeElement | null = null;
  textContent = "";
  width = 0;
  height = 0;
  clientWidth = 800;
  clientHeight = 420;
  scrollHeight = 120;
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
    return { left: 0, top: 0, width: 800, height: 420, right: 800, bottom: 420, x: 0, y: 0, toJSON() { return {}; } };
  }

  querySelector<T>(): T | null {
    return null;
  }

  getContext(kind: string): unknown {
    if (kind === "webgl" || kind === "webgl2") return createMockGL();
    return null;
  }
}

function createMockGL() {
  let idCounter = 1;
  const gl: Record<string, unknown> = {
    // constants (arbitrary distinct values, only compared for equality by our own code)
    VERTEX_SHADER: 1, FRAGMENT_SHADER: 2, COMPILE_STATUS: 3, LINK_STATUS: 4,
    ARRAY_BUFFER: 5, STATIC_DRAW: 6, DYNAMIC_DRAW: 7, TRIANGLES: 8, LINES: 9,
    POINTS: 10, FLOAT: 11, COLOR_BUFFER_BIT: 12, DEPTH_TEST: 13, BLEND: 14,
    SRC_ALPHA: 15, ONE: 16, MAX_TEXTURE_SIZE: 17,
    createShader: () => ({ id: idCounter++ }),
    shaderSource: () => {},
    compileShader: () => {},
    getShaderParameter: () => true,
    getShaderInfoLog: () => "",
    deleteShader: () => {},
    createProgram: () => ({ id: idCounter++ }),
    attachShader: () => {},
    linkProgram: () => {},
    deleteProgram: () => {},
    getProgramParameter: () => true,
    getProgramInfoLog: () => "",
    useProgram: () => {},
    createBuffer: () => ({ id: idCounter++ }),
    bindBuffer: () => {},
    bufferData: () => {},
    getAttribLocation: () => idCounter++,
    enableVertexAttribArray: () => {},
    disableVertexAttribArray: () => {},
    vertexAttribPointer: () => {},
    getUniformLocation: () => ({ id: idCounter++ }),
    uniform1f: () => {}, uniform2f: () => {}, uniform3f: () => {}, uniform4f: () => {}, uniform1i: () => {},
    drawArrays: () => {},
    clear: () => {},
    clearColor: () => {},
    viewport: () => {},
    enable: () => {},
    disable: () => {},
    blendFunc: () => {},
    deleteBuffer: () => {},
    getExtension: () => null,
    getParameter: (name: number) => (name === 17 ? 8192 : 0),
  };
  return gl;
}

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
    requestAnimationFrame: (cb: () => void) => { rafCallbacks.push(cb); return rafCallbacks.length; },
    cancelAnimationFrame: () => {},
    setInterval: (fn: () => void, ms: number) => setInterval(fn, ms) as unknown as number,
    clearInterval: (id: number) => clearInterval(id as unknown as ReturnType<typeof setInterval>),
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

  return { doc, win };
}

describe("bootstrap smoke test", () => {
  let errorSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    installDomStubs();
    errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    errorSpy.mockRestore();
  });

  it("mounts, pushes a target, and disposes without throwing or logging console errors", async () => {
    const { LuminaApp } = await import("../src/application/LuminaApp");
    const { GlRenderer } = await import("../src/adapters/render/GlRenderer");
    const { EveyBridge } = await import("../src/adapters/evey/EveyBridge");

    const app = new LuminaApp({ renderer: new GlRenderer(), bridge: new EveyBridge() });
    expect(() => app.start()).not.toThrow();

    // Let the async boot() (loadSettings -> push) settle.
    await new Promise((resolve) => setTimeout(resolve, 0));
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(() => app.dispose()).not.toThrow();
    expect(errorSpy).not.toHaveBeenCalled();
  });

  it("the canvas host is appended to the DOM after boot", async () => {
    vi.resetModules();
    const { doc } = installDomStubs();
    const { LuminaApp } = await import("../src/application/LuminaApp");
    const { GlRenderer } = await import("../src/adapters/render/GlRenderer");
    const { EveyBridge } = await import("../src/adapters/evey/EveyBridge");

    const app = new LuminaApp({ renderer: new GlRenderer(), bridge: new EveyBridge() });
    app.start();
    await new Promise((resolve) => setTimeout(resolve, 0));

    const root = (doc.body as unknown as FakeElement).children.find((c) => c.tagName === "div");
    expect(root).toBeTruthy();
    app.dispose();
  });

  it("pointer events forwarded by the overlay reach the renderer without throwing", async () => {
    vi.resetModules();
    installDomStubs();
    const { LuminaApp } = await import("../src/application/LuminaApp");
    const { GlRenderer } = await import("../src/adapters/render/GlRenderer");
    const { EveyBridge } = await import("../src/adapters/evey/EveyBridge");

    const renderer = new GlRenderer();
    const app = new LuminaApp({ renderer, bridge: new EveyBridge() });
    app.start();
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(() => renderer.setPointer(0.5, -0.3)).not.toThrow();
    app.dispose();
  });
});
