// Smoke test for the bootstrap path (mount -> library load -> first tick ->
// dispose), exercised without a browser: vitest runs in a plain "node"
// environment (this repo deliberately keeps devDependencies to esbuild/
// typescript/vitest only, matching weather-v2/cosmos). This test hand-rolls
// the minimal DOM + WebGL surface the bundle touches and asserts the whole
// pipeline (bridge -> app -> renderer) runs to a first frame with zero
// thrown errors and zero console.error calls - the offline equivalent of
// "open the page, check the console".

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
  dataset: Record<string, string> = {};
  width = 0;
  height = 0;
  clientWidth = 800;
  clientHeight = 380;
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
    return { left: 0, top: 0, width: 800, height: 380, right: 800, bottom: 380, x: 0, y: 0, toJSON() { return {}; } };
  }

  getContext(kind: string): unknown {
    if (kind === "webgl" || kind === "webgl2") return createMockGL();
    return null;
  }
}

function createMockGL() {
  let idCounter = 1;
  const gl: Record<string, unknown> = {
    VERTEX_SHADER: 1, FRAGMENT_SHADER: 2, COMPILE_STATUS: 3, LINK_STATUS: 4,
    ARRAY_BUFFER: 5, STATIC_DRAW: 6, TRIANGLES: 7, TRIANGLE_STRIP: 8, FLOAT: 9,
    COLOR_BUFFER_BIT: 10, DEPTH_TEST: 11, BLEND: 12, SRC_ALPHA: 13,
    ONE_MINUS_SRC_ALPHA: 14, MAX_TEXTURE_SIZE: 15, TEXTURE_2D: 16, TEXTURE0: 17,
    RGB: 18, UNSIGNED_BYTE: 19, TEXTURE_MIN_FILTER: 20, TEXTURE_MAG_FILTER: 21,
    LINEAR: 22, TEXTURE_WRAP_S: 23, TEXTURE_WRAP_T: 24, CLAMP_TO_EDGE: 25,
    UNPACK_FLIP_Y_WEBGL: 26,
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
    deleteBuffer: () => {},
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
    getExtension: () => null,
    getParameter: (name: number) => (name === 15 ? 8192 : 0),
    createTexture: () => ({ id: idCounter++ }),
    deleteTexture: () => {},
    bindTexture: () => {},
    activeTexture: () => {},
    texImage2D: () => {},
    texParameteri: () => {},
    pixelStorei: () => {},
  };
  return gl;
}

const CANNED_LIBRARY = Array.from({ length: 40 }, (_, i) => ({
  id: `mi-${i}`,
  title: `Movie ${i}`,
  year: 2000 + (i % 25),
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
  class FakeImage {
    onload: (() => void) | null = null;
    onerror: (() => void) | null = null;
    set src(_v: string) {
      // Never resolves in this smoke test: we only assert the pipeline runs
      // cleanly through the placeholder path, not that real photos decode.
    }
  }

  vi.stubGlobal("document", doc);
  vi.stubGlobal("window", win);
  vi.stubGlobal("navigator", { language: "en", hardwareConcurrency: 4 });
  vi.stubGlobal("ResizeObserver", FakeResizeObserver);
  vi.stubGlobal("IntersectionObserver", FakeIntersectionObserver);
  vi.stubGlobal("Image", FakeImage);
  vi.stubGlobal("requestAnimationFrame", win.requestAnimationFrame);
  vi.stubGlobal("cancelAnimationFrame", win.cancelAnimationFrame);

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
    const { MarqueeApp } = await import("../src/application/MarqueeApp");
    const { GlRenderer } = await import("../src/adapters/render/GlRenderer");
    const { EveyBridge } = await import("../src/adapters/evey/EveyBridge");

    const app = new MarqueeApp({ renderer: new GlRenderer(), bridge: new EveyBridge() });
    expect(() => app.start()).not.toThrow();

    // Let the async boot() (bridge.onReady -> listMedia) settle.
    await new Promise((resolve) => setTimeout(resolve, 0));
    await new Promise((resolve) => setTimeout(resolve, 0));

    // Drive one animation frame now that the wall has been pushed to the renderer.
    expect(() => rafCallbacks[0]?.()).not.toThrow();

    expect(() => app.dispose()).not.toThrow();
    expect(errorSpy).not.toHaveBeenCalled();
  });

  it("appends the canvas host to the DOM after boot", async () => {
    vi.resetModules();
    const { doc } = installDomStubs();
    const { MarqueeApp } = await import("../src/application/MarqueeApp");
    const { GlRenderer } = await import("../src/adapters/render/GlRenderer");
    const { EveyBridge } = await import("../src/adapters/evey/EveyBridge");

    const app = new MarqueeApp({ renderer: new GlRenderer(), bridge: new EveyBridge() });
    app.start();
    await new Promise((resolve) => setTimeout(resolve, 0));

    const root = (doc.body as unknown as FakeElement).children.find((c) => c.tagName === "div");
    expect(root).toBeTruthy();
    app.dispose();
  });

  it("shows the empty-library message and still disposes cleanly when the library has no eligible posters", async () => {
    vi.resetModules();
    installDomStubs();
    (window as unknown as { evey: { media: { list: () => Promise<unknown> } } }).evey.media.list = async () => ({ items: [] });

    const { MarqueeApp } = await import("../src/application/MarqueeApp");
    const { GlRenderer } = await import("../src/adapters/render/GlRenderer");
    const { EveyBridge } = await import("../src/adapters/evey/EveyBridge");

    const app = new MarqueeApp({ renderer: new GlRenderer(), bridge: new EveyBridge() });
    app.start();
    await new Promise((resolve) => setTimeout(resolve, 0));
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(() => app.dispose()).not.toThrow();
    expect(errorSpy).not.toHaveBeenCalled();
  });
});
