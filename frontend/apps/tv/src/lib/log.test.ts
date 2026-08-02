import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { logger, recentLogs } from "./log";

describe("logger", () => {
  let logSpy: ReturnType<typeof vi.spyOn>;
  let warnSpy: ReturnType<typeof vi.spyOn>;
  let errorSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    logSpy = vi.spyOn(console, "log").mockImplementation(() => {});
    warnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});
    errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
  });

  afterEach(() => {
    logSpy.mockRestore();
    warnSpy.mockRestore();
    errorSpy.mockRestore();
  });

  it("prefixes debug/info lines with EVEY|<level>|<tag>| and routes them through console.log", () => {
    const log = logger("test-tag");
    log.debug("hello");
    log.info("world");
    expect(logSpy).toHaveBeenNthCalledWith(1, "EVEY|debug|test-tag| hello");
    expect(logSpy).toHaveBeenNthCalledWith(2, "EVEY|info|test-tag| world");
    expect(warnSpy).not.toHaveBeenCalled();
    expect(errorSpy).not.toHaveBeenCalled();
  });

  it("prefixes warn lines with EVEY|warn|<tag>| and routes them through console.warn", () => {
    const log = logger("test-tag");
    log.warn("something odd");
    expect(warnSpy).toHaveBeenCalledWith("EVEY|warn|test-tag| something odd");
  });

  it("prefixes error lines with EVEY|error|<tag>| and routes them through console.error", () => {
    const log = logger("test-tag");
    log.error("boom");
    expect(errorSpy).toHaveBeenCalledWith("EVEY|error|test-tag| boom");
  });

  it("joins multiple arguments into one line", () => {
    const log = logger("test-tag");
    log.warn("failed:", "details", 42);
    expect(warnSpy).toHaveBeenCalledWith("EVEY|warn|test-tag| failed: details 42");
  });

  it("renders an Error argument as name: message", () => {
    const log = logger("test-tag");
    log.error(new Error("oops"));
    expect(errorSpy).toHaveBeenCalledWith("EVEY|error|test-tag| Error: oops");
  });

  it("keeps every emitted line in recentLogs, oldest first", () => {
    const log = logger("ring-tag");
    log.info("first");
    log.info("second");
    const lines = recentLogs();
    expect(lines.at(-2)).toBe("EVEY|info|ring-tag| first");
    expect(lines.at(-1)).toBe("EVEY|info|ring-tag| second");
  });

  it("bounds the ring buffer even after far more lines than its max", () => {
    const log = logger("flood-tag");
    for (let i = 0; i < 500; i++) log.info(`line-${i}`);
    const lines = recentLogs();
    // RING_MAX is 200: any earlier flood from other tests may already be in
    // the ring, so just assert the bound holds and the tail is what we expect.
    expect(lines.length).toBeLessThanOrEqual(200);
    expect(lines.at(-1)).toBe("EVEY|info|flood-tag| line-499");
  });
});
