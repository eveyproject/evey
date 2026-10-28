import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

const mockEveyFetch = vi.fn(() => Promise.resolve(undefined));
vi.mock("./fetcher", () => ({ eveyFetch: (...args: unknown[]) => mockEveyFetch(...args) }));

import { createPlayerEventReporter } from "./playerEventReporter";

// The beacon must be loud enough to diagnose an incident and quiet enough to
// never become one: a broken stream fires the same decode error dozens of
// times a second, and each accepted event is a server log line.
describe("playerEventReporter", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    mockEveyFetch.mockClear();
  });
  afterEach(() => vi.useRealTimers());

  it("posts the event with its full context", () => {
    const report = createPlayerEventReporter("media-1");
    report({ type: "hls_media_error", details: "bufferAppendError", fatal: true, code: 3, variant: 2, segment: 1023, posSec: 3066.5, state: "rs=4 buf=320.0-347.2" });

    expect(mockEveyFetch).toHaveBeenCalledWith({
      url: "/api/v1/media/media-1/player-event",
      method: "POST",
      data: {
        type: "hls_media_error",
        details: "bufferAppendError",
        fatal: true,
        code: 3,
        variant: 2,
        segment: 1023,
        posSec: 3066.5,
        state: "rs=4 buf=320.0-347.2",
      },
    });
  });

  it("sends a native player's diagnosis alongside, and nothing when there is none", () => {
    const report = createPlayerEventReporter("media-1");
    const diag = {
      renderer: "video", formatSupported: "NO_EXCEEDS_CAPABILITIES", decoder: "c2.vendor.avc.decoder",
      mime: "video/avc", codecs: "avc1.640033", width: 3840, height: 2160, fps: 60, bitrateKbps: 8000,
      playMethod: "direct_play", errorCode: "", causes: ["Decoder init failed"],
    };
    report({ type: "native_player_error", details: "x", fatal: true, diag });
    expect(mockEveyFetch.mock.calls[0][0]).toMatchObject({ data: { type: "native_player_error", diag } });

    report({ type: "hls_fatal", details: "y" });
    expect(mockEveyFetch.mock.calls[1][0]).not.toHaveProperty("data.diag");
  });

  it("keeps the snapshot out of the dedupe key", () => {
    // The buffer picture changes on every stall; the stall itself is the same
    // event, and reporting each picture would drain the session budget.
    const report = createPlayerEventReporter("media-1");
    report({ type: "hls_media_error", details: "bufferNudgeOnStall", state: "buf=1-2" });
    report({ type: "hls_media_error", details: "bufferNudgeOnStall", state: "buf=3-4" });
    expect(mockEveyFetch).toHaveBeenCalledTimes(1);
  });

  it("dedupes identical consecutive events inside the window, and lets them through after it", () => {
    const report = createPlayerEventReporter("media-1");
    report({ type: "hls_media_error", details: "bufferAppendError" });
    report({ type: "hls_media_error", details: "bufferAppendError" });
    report({ type: "hls_media_error", details: "bufferAppendError" });
    expect(mockEveyFetch).toHaveBeenCalledTimes(1);

    // A DIFFERENT event goes through immediately - it is new information.
    report({ type: "hls_fatal", details: "fragParsingError" });
    expect(mockEveyFetch).toHaveBeenCalledTimes(2);

    // The same event again after the window: the error is persisting, which
    // is itself worth one more line.
    vi.advanceTimersByTime(6_000);
    report({ type: "hls_fatal", details: "fragParsingError" });
    expect(mockEveyFetch).toHaveBeenCalledTimes(3);
  });

  it("stops at the per-session budget: the first events carry all the value", () => {
    const report = createPlayerEventReporter("media-1");
    for (let i = 0; i < 100; i++) {
      report({ type: "hls_media_error", details: `err-${i}` });
    }
    expect(mockEveyFetch.mock.calls.length).toBeLessThanOrEqual(20);
  });

  it("never throws when the POST fails: telemetry must not break playback", async () => {
    mockEveyFetch.mockReturnValueOnce(Promise.reject(new Error("429")));
    const report = createPlayerEventReporter("media-1");
    expect(() => report({ type: "hls_fatal" })).not.toThrow();
    await vi.runAllTimersAsync();
  });
});
