import { describe, it, expect, vi, beforeEach } from "vitest";

const store: Record<string, string> = {};
vi.mock("expo-secure-store", () => ({
  getItemAsync: vi.fn((key: string) => Promise.resolve(store[key] ?? null)),
  setItemAsync: vi.fn((key: string, value: string) => {
    store[key] = value;
    return Promise.resolve();
  }),
  deleteItemAsync: vi.fn((key: string) => {
    delete store[key];
    return Promise.resolve();
  }),
}));

import { getServerUrl, setServerUrl, clearServerUrl, pingServer, probeServerHealth } from "./server-config";

beforeEach(() => {
  Object.keys(store).forEach((k) => delete store[k]);
});

describe("server-config", () => {
  it("returns null when no URL stored", async () => {
    expect(await getServerUrl()).toBeNull();
  });

  it("stores and retrieves server URL", async () => {
    await setServerUrl("http://myserver:8080");
    expect(await getServerUrl()).toBe("http://myserver:8080");
  });

  it("strips trailing slashes", async () => {
    await setServerUrl("http://myserver:8080///");
    expect(await getServerUrl()).toBe("http://myserver:8080");
  });

  it("clears server URL", async () => {
    await setServerUrl("http://myserver:8080");
    await clearServerUrl();
    expect(await getServerUrl()).toBeNull();
  });

  describe("pingServer", () => {
    it("returns true when an Evey server answers", async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ status: "ok", version: "1" }) });
      expect(await pingServer("http://srv")).toBe(true);
    });

    // A bare 200 is not a server: a captive portal, an ISP's DNS-hijack page
    // or a router's admin page answers 200 to everything, and "connecting" to
    // one cost a beta tester an afternoon (2026-07-29, phone). The television
    // now applies the same rule through the shared probe.
    it("returns false on a 200 that is not Evey", async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => { throw new Error("html"); } });
      expect(await pingServer("http://srv")).toBe(false);
    });

    it("returns false on non-ok response", async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({ ok: false });
      expect(await pingServer("http://srv")).toBe(false);
    });

    it("returns false on network error", async () => {
      globalThis.fetch = vi.fn().mockRejectedValue(new Error("timeout"));
      expect(await pingServer("http://srv")).toBe(false);
    });
  });

  describe("probeServerHealth", () => {
    it("reads the advertised server name, trimmed", async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ status: "ok", version: "1", name: " Salon " }) });
      expect(await probeServerHealth("http://srv")).toEqual({ ok: true, name: "Salon" });
    });

    it("an Evey server without a name (older build) is ok, a non-Evey 200 is not", async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => { throw new Error("html"); } });
      expect(await probeServerHealth("http://srv")).toEqual({ ok: false });
      globalThis.fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ status: "ok", version: "1", name: "  " }) });
      expect(await probeServerHealth("http://srv")).toEqual({ ok: true, name: undefined });
    });
  });
});
