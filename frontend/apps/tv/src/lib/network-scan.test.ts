import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { scanLocalNetwork } from "./network-scan";

describe("scanLocalNetwork", () => {
  let inFlight = 0;
  let maxInFlight = 0;

  beforeEach(() => {
    inFlight = 0;
    maxInFlight = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: string) => {
        inFlight++;
        maxInFlight = Math.max(maxInFlight, inFlight);
        // Yield once so overlapping probes are observable as in-flight.
        await Promise.resolve();
        inFlight--;
        if (input.startsWith("http://192.168.1.42:9091/")) {
          return {
            ok: true,
            json: async () => ({ status: "ok", version: "0.0.1-beta.33" }),
          } as Response;
        }
        return { ok: false, json: async () => ({}) } as Response;
      }),
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("reports servers whose /healthz answers ok, with their version", async () => {
    const found: { url: string; version?: string }[] = [];
    await scanLocalNetwork((s) => found.push(s));
    expect(found).toEqual([{ url: "http://192.168.1.42:9091", version: "0.0.1-beta.33" }]);
  });

  it("never has more than one batch of probes in flight", async () => {
    await scanLocalNetwork(() => {});
    expect(maxInFlight).toBeGreaterThan(0);
    expect(maxInFlight).toBeLessThanOrEqual(50);
  });

  // The sweep is ~3000 probes over tens of seconds; leaving the screen must
  // stop it. Before the fix the opts were ignored and the whole subnet sweep
  // kept running (and reporting) on an unmounted screen.
  it("stops probing and reporting once the signal aborts", async () => {
    let calls = 0;
    globalThis.fetch = vi.fn().mockImplementation(() => {
      calls++;
      return Promise.resolve({ ok: false });
    });

    const controller = new AbortController();
    const found: Array<{ url: string }> = [];
    const done = scanLocalNetwork((s) => found.push(s), { signal: controller.signal });
    controller.abort();
    await done;

    // A batch (50) may already be in flight when the abort lands; anything
    // beyond ~2 batches means the sweep kept going.
    expect(calls).toBeLessThan(150);
    expect(found).toHaveLength(0);
  }, 30000);
});
