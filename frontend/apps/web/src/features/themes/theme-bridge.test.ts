import { describe, it, expect, vi, beforeEach } from "vitest";
import { THEME_BRIDGE_SCRIPT } from "./theme-bridge";

// Evaluate the bridge script in a fake window context to test its behaviour
// without a real iframe. We create a minimal fake window object that captures
// postMessage calls and exposes the injected evey.theme API.

function runBridge(fakeWindow: Record<string, unknown>) {
  // The bridge references window.parent.postMessage and window.evey.
  // We inject it via Function constructor so it runs in the provided scope.
  // eslint-disable-next-line no-new-func
  const fn = new Function("window", THEME_BRIDGE_SCRIPT);
  fn(fakeWindow);
}

function makeFakeWindow() {
  const messages: unknown[] = [];
  const fakeWindow: Record<string, unknown> = {};
  fakeWindow["parent"] = {
    postMessage: (msg: unknown) => messages.push(msg),
  };
  fakeWindow["evey"] = undefined;
  return { fakeWindow, messages };
}

describe("THEME_BRIDGE_SCRIPT", () => {
  it("defines window.evey.theme", () => {
    const { fakeWindow } = makeFakeWindow();
    runBridge(fakeWindow);
    expect(fakeWindow["evey"]).toBeDefined();
    const evey = fakeWindow["evey"] as { theme: Record<string, unknown> };
    expect(typeof evey.theme.css).toBe("function");
    expect(typeof evey.theme.log).toBe("function");
    expect(typeof evey.theme.registerComponent).toBe("function");
    expect(typeof evey.theme.observe).toBe("function");
  });

  it("evey.theme.css sends theme.css postMessage with rules", () => {
    const { fakeWindow, messages } = makeFakeWindow();
    runBridge(fakeWindow);
    const evey = fakeWindow["evey"] as { theme: { css: (r: string) => void } };
    evey.theme.css("body { background: red; }");
    expect(messages).toHaveLength(1);
    expect(messages[0]).toEqual({ type: "theme.css", rules: "body { background: red; }" });
  });

  it("evey.theme.css ignores non-string input", () => {
    const { fakeWindow, messages } = makeFakeWindow();
    runBridge(fakeWindow);
    const evey = fakeWindow["evey"] as { theme: { css: (r: unknown) => void } };
    evey.theme.css(null as unknown as string);
    evey.theme.css(42 as unknown as string);
    expect(messages).toHaveLength(0);
  });

  it("evey.theme.log sends theme.log postMessage with correct level and msg", () => {
    const { fakeWindow, messages } = makeFakeWindow();
    runBridge(fakeWindow);
    const evey = fakeWindow["evey"] as { theme: { log: (l: string, m: string) => void } };
    evey.theme.log("info", "hello world");
    expect(messages).toHaveLength(1);
    expect(messages[0]).toEqual({ type: "theme.log", level: "info", msg: "hello world" });
  });

  it("evey.theme.log normalises unknown log levels to info", () => {
    const { fakeWindow, messages } = makeFakeWindow();
    runBridge(fakeWindow);
    const evey = fakeWindow["evey"] as { theme: { log: (l: string, m: string) => void } };
    evey.theme.log("verbose", "msg");
    expect((messages[0] as { level: string }).level).toBe("info");
  });

  it("evey.theme.log truncates messages longer than 500 chars", () => {
    const { fakeWindow, messages } = makeFakeWindow();
    runBridge(fakeWindow);
    const evey = fakeWindow["evey"] as { theme: { log: (l: string, m: string) => void } };
    const longMsg = "x".repeat(1000);
    evey.theme.log("warn", longMsg);
    expect((messages[0] as { msg: string }).msg.length).toBe(500);
  });

  it("evey.theme.registerComponent sends not-implemented ack", () => {
    const { fakeWindow, messages } = makeFakeWindow();
    runBridge(fakeWindow);
    const evey = fakeWindow["evey"] as { theme: { registerComponent: (s: string) => void } };
    evey.theme.registerComponent("header");
    expect(messages).toHaveLength(1);
    const msg = messages[0] as { type: string; error: string };
    expect(msg.type).toBe("theme.registerComponent");
    expect(msg.error).toBe("not implemented");
  });

  it("evey.theme.observe sends not-implemented ack", () => {
    const { fakeWindow, messages } = makeFakeWindow();
    runBridge(fakeWindow);
    const evey = fakeWindow["evey"] as { theme: { observe: (k: string) => void } };
    evey.theme.observe("player.playing");
    expect(messages).toHaveLength(1);
    const msg = messages[0] as { type: string; error: string };
    expect(msg.type).toBe("theme.observe");
    expect(msg.error).toBe("not implemented");
  });

  it("bridge does not expose fetch or localStorage on fakeWindow", () => {
    const { fakeWindow } = makeFakeWindow();
    // The bridge script does not add fetch or localStorage to the window.
    runBridge(fakeWindow);
    expect(fakeWindow["fetch"]).toBeUndefined();
    expect(fakeWindow["localStorage"]).toBeUndefined();
  });
});

describe("THEME_BRIDGE_SCRIPT security", () => {
  it("bridge does not set document on fakeWindow", () => {
    const { fakeWindow } = makeFakeWindow();
    runBridge(fakeWindow);
    expect(fakeWindow["document"]).toBeUndefined();
  });

  it("bridge does not set XMLHttpRequest on fakeWindow", () => {
    const { fakeWindow } = makeFakeWindow();
    runBridge(fakeWindow);
    expect(fakeWindow["XMLHttpRequest"]).toBeUndefined();
  });
});

// Verify the THEME_BRIDGE_SCRIPT export itself
describe("THEME_BRIDGE_SCRIPT export", () => {
  it("is a non-empty string", () => {
    expect(typeof THEME_BRIDGE_SCRIPT).toBe("string");
    expect(THEME_BRIDGE_SCRIPT.length).toBeGreaterThan(10);
  });

  it("contains evey.theme definitions", () => {
    expect(THEME_BRIDGE_SCRIPT).toContain("evey.theme");
    expect(THEME_BRIDGE_SCRIPT).toContain("theme.css");
    expect(THEME_BRIDGE_SCRIPT).toContain("theme.log");
  });

  it("does not reference fetch or XMLHttpRequest", () => {
    expect(THEME_BRIDGE_SCRIPT).not.toContain("fetch(");
    expect(THEME_BRIDGE_SCRIPT).not.toContain("XMLHttpRequest");
    expect(THEME_BRIDGE_SCRIPT).not.toContain("localStorage");
  });
});

describe("vi.fn mocking sanity check", () => {
  it("vi.fn works as expected", () => {
    const mock = vi.fn();
    mock("test");
    expect(mock).toHaveBeenCalledWith("test");
  });

  beforeEach(() => {
    vi.clearAllMocks();
  });
});
