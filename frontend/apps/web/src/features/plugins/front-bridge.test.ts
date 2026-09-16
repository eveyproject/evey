import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { BRIDGE_SCRIPT } from "./front-bridge";

// The bridge runs INSIDE the plugin iframe. What it must guarantee: every call
// becomes one postMessage to the parent, an RPC settles from the matching
// rpc-result only, init is replayed to late subscribers, and a message from
// anything but the parent frame is ignored (a sibling iframe must not be able
// to fake an init or resolve someone else's RPC).

type Sent = { type: string; [k: string]: unknown };
type EveyWindow = Window & { evey?: { navigate(p: string): void; onInit(cb: (d: unknown) => void): void; prefs: { get(k: string): Promise<unknown> } } };

function loadBridge() {
  const sent: Sent[] = [];
  const fakeParent = { postMessage: (data: Sent) => { sent.push(data); } } as unknown as Window;
  Object.defineProperty(window, "parent", { value: fakeParent, configurable: true });
  // eslint-disable-next-line no-new-func
  new Function(BRIDGE_SCRIPT)();
  const post = (data: unknown, source: unknown = fakeParent) =>
    window.dispatchEvent(new MessageEvent("message", { data, source: source as Window }));
  return { sent, post, evey: (window as EveyWindow).evey! };
}

describe("plugin front bridge", () => {
  const originalParent = window.parent;
  beforeEach(() => { delete (window as EveyWindow).evey; });
  afterEach(() => { Object.defineProperty(window, "parent", { value: originalParent, configurable: true }); });

  it("announces itself and forwards navigate as a message to the parent", async () => {
    const { sent, evey } = loadBridge();
    await new Promise((r) => queueMicrotask(() => r(null)));
    expect(sent.some((m) => m.type === "ready")).toBe(true);
    evey.navigate("/media/m1");
    expect(sent.at(-1)).toEqual({ type: "navigate", path: "/media/m1" });
  });

  it("settles an rpc from the parent's matching result, and ignores a foreign source", async () => {
    const { sent, post, evey } = loadBridge();
    const pending = evey.prefs.get("k");
    const call = sent.find((m) => m.type === "rpc")!;
    expect(call).toMatchObject({ method: "prefs.get", args: { key: "k" } });

    // A sibling frame answering with the right id must not settle it.
    post({ type: "rpc-result", id: call.id, ok: true, data: { value: "forged", found: true } }, { not: "parent" });
    let settled = false;
    void pending.then(() => { settled = true; });
    await new Promise((r) => setTimeout(r, 0));
    expect(settled).toBe(false);

    post({ type: "rpc-result", id: call.id, ok: true, data: { value: "real", found: true } });
    await expect(pending).resolves.toEqual({ value: "real", found: true });

    const failing = evey.prefs.get("missing");
    const call2 = sent.filter((m) => m.type === "rpc").at(-1)!;
    post({ type: "rpc-result", id: call2.id, ok: false, error: "denied" });
    await expect(failing).rejects.toMatchObject({ message: "denied" });
  });

  it("replays init to a subscriber that registers after the init arrived", () => {
    const { post, evey } = loadBridge();
    const seen: unknown[] = [];
    evey.onInit((d) => seen.push(d));
    post({ type: "init", context: { a: 1 }, locale: "fr", theme: "dark" });
    expect(seen).toHaveLength(1);
    evey.onInit((d) => seen.push(d));
    expect(seen).toHaveLength(2);
    // Init from a foreign source is not an init.
    post({ type: "init", context: {}, locale: "xx", theme: "x" }, { not: "parent" });
    expect(seen).toHaveLength(2);
  });
});
