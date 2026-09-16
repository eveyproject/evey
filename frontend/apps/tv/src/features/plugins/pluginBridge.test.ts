import { describe, it, expect, beforeEach, vi } from "vitest";
import { PLUGIN_BRIDGE_SCRIPT } from "./pluginBridge";

// The script runs INSIDE the plugin's WebView. Evaluate it against jsdom
// with a captured ReactNativeWebView.postMessage (guest -> host) and feed it
// host -> guest frames as window "message" events.
type Frame = Record<string, unknown>;

function boot(): { sent: Frame[]; deliver: (data: unknown) => void } {
  const sent: Frame[] = [];
  (window as unknown as { ReactNativeWebView: unknown }).ReactNativeWebView = {
    postMessage: (s: string) => sent.push(JSON.parse(s) as Frame),
  };
  // eslint-disable-next-line no-new-func
  new Function(PLUGIN_BRIDGE_SCRIPT)();
  return {
    sent,
    deliver: (data) => window.dispatchEvent(new MessageEvent("message", { data })),
  };
}

const evey = () => (window as unknown as { evey: any }).evey;

beforeEach(() => {
  vi.useFakeTimers();
});

describe("plugin bridge (WebView guest side)", () => {
  it("announces ready once the document is loaded and exposes the evey surface", async () => {
    const { sent } = boot();
    await vi.runAllTimersAsync();
    expect(sent).toContainEqual({ type: "ready" });
    expect(typeof evey().navigate).toBe("function");
    expect(typeof evey().media.poster).toBe("function");
  });

  it("forwards navigate, toast and resize as plain frames", async () => {
    const { sent } = boot();
    evey().navigate("/media/m1");
    evey().toast("info", "hello");
    evey().resize(320);
    expect(sent).toContainEqual({ type: "navigate", path: "/media/m1" });
    expect(sent).toContainEqual({ type: "toast", level: "info", message: "hello" });
    expect(sent).toContainEqual({ type: "resize", height: 320 });
  });

  it("delivers init to late and early listeners alike, and accepts a serialized frame", async () => {
    const { deliver } = boot();
    const early = vi.fn();
    evey().onInit(early);
    deliver(JSON.stringify({ type: "init", pluginId: "p1", theme: "dark" }));
    expect(early).toHaveBeenCalledWith(expect.objectContaining({ pluginId: "p1" }));
    const late = vi.fn();
    evey().onInit(late);
    expect(late).toHaveBeenCalledWith(expect.objectContaining({ pluginId: "p1" }));
  });

  it("correlates rpc results by id, resolving data and rejecting errors", async () => {
    const { sent, deliver } = boot();
    const get = evey().prefs.get("volume");
    const list = evey().media.list({ limit: 3 });
    const rpcs = sent.filter((f) => f.type === "rpc");
    expect(rpcs).toHaveLength(2);
    expect(rpcs[0]).toMatchObject({ method: "prefs.get", args: { key: "volume" } });
    expect(rpcs[1]).toMatchObject({ method: "media.list", args: { limit: 3 } });
    expect(rpcs[0].id).not.toBe(rpcs[1].id);

    deliver({ type: "rpc-result", id: rpcs[1].id, ok: true, data: [{ id: "m1" }] });
    deliver({ type: "rpc-result", id: rpcs[0].id, ok: false, error: "no such pref" });
    await expect(list).resolves.toEqual([{ id: "m1" }]);
    await expect(get).rejects.toMatchObject({ message: "no such pref" });
    // A stray result for an unknown id is ignored.
    expect(() => deliver({ type: "rpc-result", id: 9999, ok: true })).not.toThrow();
  });

  it("ignores frames it cannot parse", () => {
    const { deliver } = boot();
    expect(() => deliver("not json")).not.toThrow();
    expect(() => deliver(null)).not.toThrow();
  });
});
