import { describe, it, expect, vi, beforeEach } from "vitest";

beforeEach(() => {
  vi.clearAllMocks();
});

// probeHost is not exported; test it via scanLocalNetwork behavior
// and test the exported scanLocalNetwork function directly.
// We also test probeHost indirectly via a small network with 1 host.

import { scanLocalNetwork } from "./network-scan";

describe("scanLocalNetwork", () => {
  it("calls onFound for hosts that respond ok", async () => {
    globalThis.fetch = vi.fn().mockImplementation((url: string) => {
      // Only respond ok for a specific host/port
      if (url === "http://192.168.1.1:9091/healthz") {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ version: "1.2.3" }),
        });
      }
      return Promise.resolve({ ok: false });
    });

    const found: Array<{ url: string; version?: string }> = [];
    await scanLocalNetwork((s) => found.push(s));

    expect(found).toHaveLength(1);
    expect(found[0].url).toBe("http://192.168.1.1:9091");
    expect(found[0].version).toBe("1.2.3");
  }, 30000);

  it("calls onFound without version when json parse fails", async () => {
    globalThis.fetch = vi.fn().mockImplementation((url: string) => {
      if (url === "http://192.168.0.5:8096/healthz") {
        return Promise.resolve({
          ok: true,
          json: () => Promise.reject(new Error("invalid json")),
        });
      }
      return Promise.resolve({ ok: false });
    });

    const found: Array<{ url: string; version?: string }> = [];
    await scanLocalNetwork((s) => found.push(s));

    const match = found.find((s) => s.url === "http://192.168.0.5:8096");
    expect(match).toBeDefined();
    expect(match?.version).toBeUndefined();
  }, 30000);

  it("skips hosts that throw network errors", async () => {
    globalThis.fetch = vi.fn().mockRejectedValue(new Error("network error"));

    const found: Array<{ url: string; version?: string }> = [];
    await scanLocalNetwork((s) => found.push(s));

    expect(found).toHaveLength(0);
  }, 30000);

  it("calls onFound multiple times for multiple responding hosts", async () => {
    const respondingUrls = new Set([
      "http://192.168.1.10:9091/healthz",
      "http://10.0.0.1:8096/healthz",
    ]);

    globalThis.fetch = vi.fn().mockImplementation((url: string) => {
      if (respondingUrls.has(url)) {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ version: "2.0" }),
        });
      }
      return Promise.resolve({ ok: false });
    });

    const found: Array<{ url: string; version?: string }> = [];
    await scanLocalNetwork((s) => found.push(s));

    expect(found).toHaveLength(2);
    const urls = found.map((s) => s.url);
    expect(urls).toContain("http://192.168.1.10:9091");
    expect(urls).toContain("http://10.0.0.1:8096");
  }, 30000);

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
