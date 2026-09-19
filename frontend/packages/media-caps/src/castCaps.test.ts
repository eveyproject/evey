import { describe, it, expect, vi, afterEach } from "vitest";
import { askReceiverCaps, CAPS_NAMESPACE, CAPS_REQUEST, parseCapsReport, type CapsTransport } from "./castCaps";

const observed = { videoCodecs: ["h264", "hevc"], maxHeight: 2160, hdr: true, nativeDolby: false, maxAudioChannels: 2, maxH264Level: 51, h264MaxHeight: 1080 };

// A transport the test drives by hand: every send is counted, an answer can
// be scripted per send or pushed unprompted (the READY broadcast).
function fakeTransport(answerFor?: (sendIndex: number) => unknown) {
  let handler: ((raw: unknown) => void) | null = null;
  const t = {
    sends: 0,
    unsubscribed: 0,
    subscribed: 0,
    push(raw: unknown) { handler?.(raw); },
    transport: {
      send: vi.fn(async () => {
        t.sends += 1;
        const a = answerFor?.(t.sends);
        if (a !== undefined) queueMicrotask(() => handler?.(a));
      }),
      subscribe: vi.fn((h: (raw: unknown) => void) => { t.subscribed += 1; handler = h; return () => { t.unsubscribed += 1; handler = null; }; }),
    } as CapsTransport,
  };
  return t;
}

afterEach(() => { vi.useRealTimers(); });

describe("parseCapsReport", () => {
  it("accepts an object or its JSON, keeps the caps verbatim, user agent as diagnostic", () => {
    const report = { type: "caps", caps: observed, userAgent: "CrKey/1.56" };
    expect(parseCapsReport(report)).toEqual({ caps: observed, userAgent: "CrKey/1.56" });
    expect(parseCapsReport(JSON.stringify(report))).toEqual({ caps: observed, userAgent: "CrKey/1.56" });
  });

  it("fills what a receiver may omit with the conservative values", () => {
    expect(parseCapsReport({ type: "caps", caps: { videoCodecs: ["h264", 7], maxHeight: 1080 } })).toEqual({
      caps: { videoCodecs: ["h264"], maxHeight: 1080, hdr: false, nativeDolby: false, maxAudioChannels: 2, maxH264Level: 40, h264MaxHeight: undefined },
      userAgent: null,
    });
  });

  it("rejects anything that is not a caps report", () => {
    expect(parseCapsReport(null)).toBeNull();
    expect(parseCapsReport("not json")).toBeNull();
    expect(parseCapsReport({ type: "load" })).toBeNull();
    expect(parseCapsReport({ type: "caps" })).toBeNull();
    expect(parseCapsReport({ type: "caps", caps: { maxHeight: "big", videoCodecs: ["h264"] } })).toBeNull();
    expect(parseCapsReport({ type: "caps", caps: { maxHeight: 1080 } })).toBeNull();
  });
});

describe("askReceiverCaps", () => {
  it("listens before it asks, sends the request on the namespace shape, and takes an immediate answer", async () => {
    const t = fakeTransport(() => ({ type: "caps", caps: observed, userAgent: "ua" }));
    const got = await askReceiverCaps(t.transport, { timeoutMs: 1000, retryMs: 100 });
    expect(got).toEqual({ caps: observed, userAgent: "ua" });
    expect(t.subscribed).toBe(1);
    expect(t.transport.send).toHaveBeenCalledWith(CAPS_REQUEST);
    expect(t.unsubscribed, "listener removed once answered").toBe(1);
    expect(CAPS_NAMESPACE).toBe("urn:x-cast:org.eveyproject.caps");
  });

  it("re-asks every retryMs and takes a late answer within the budget", async () => {
    vi.useFakeTimers();
    const t = fakeTransport((n) => (n >= 3 ? { type: "caps", caps: observed } : undefined));
    const p = askReceiverCaps(t.transport, { timeoutMs: 5000, retryMs: 500 });
    await vi.advanceTimersByTimeAsync(1200);
    expect((await p)?.caps).toEqual(observed);
    expect(t.sends).toBe(3);
    await vi.advanceTimersByTimeAsync(5000);
    expect(t.sends, "no send after resolution").toBe(3);
    expect(vi.getTimerCount(), "no timer left behind").toBe(0);
  });

  it("takes the receiver's unprompted READY broadcast", async () => {
    vi.useFakeTimers();
    const t = fakeTransport();
    const p = askReceiverCaps(t.transport, { timeoutMs: 5000, retryMs: 500 });
    await vi.advanceTimersByTimeAsync(2000);
    t.push(JSON.stringify({ type: "caps", caps: observed, userAgent: "CrKey" }));
    expect((await p)?.userAgent).toBe("CrKey");
    expect(t.unsubscribed).toBe(1);
    expect(vi.getTimerCount()).toBe(0);
  });

  it("resolves null at the end of the budget when nothing valid ever comes, listener and timers gone", async () => {
    vi.useFakeTimers();
    const t = fakeTransport(() => ({ type: "caps", caps: { maxHeight: "big" } })); // malformed every time
    const p = askReceiverCaps(t.transport, { timeoutMs: 5000, retryMs: 500 });
    await vi.advanceTimersByTimeAsync(5000);
    expect(await p).toBeNull();
    expect(t.sends).toBeGreaterThanOrEqual(10);
    expect(t.unsubscribed).toBe(1);
    expect(vi.getTimerCount()).toBe(0);
  });

  it("a send that keeps failing is just silence until the budget; a subscribe that throws is null at once", async () => {
    vi.useFakeTimers();
    const failing = fakeTransport();
    failing.transport.send = vi.fn(async () => { throw new Error("channel closed"); });
    const p = askReceiverCaps(failing.transport, { timeoutMs: 600, retryMs: 200 });
    await vi.advanceTimersByTimeAsync(600);
    expect(await p).toBeNull();
    expect(failing.unsubscribed).toBe(1);

    const broken: CapsTransport = { send: async () => {}, subscribe: () => { throw new Error("no session"); } };
    expect(await askReceiverCaps(broken, { timeoutMs: 600, retryMs: 200 })).toBeNull();
    expect(vi.getTimerCount()).toBe(0);
  });

  it("the first valid answer wins; a second one is ignored", async () => {
    vi.useFakeTimers();
    const t = fakeTransport();
    const p = askReceiverCaps(t.transport, { timeoutMs: 5000, retryMs: 500 });
    t.push({ type: "caps", caps: observed });
    t.push({ type: "caps", caps: { ...observed, maxHeight: 720 } });
    expect((await p)?.caps.maxHeight).toBe(2160);
  });
});
