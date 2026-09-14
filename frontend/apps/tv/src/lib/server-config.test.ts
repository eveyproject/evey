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
    it("returns true on ok response", async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({ ok: true });
      expect(await pingServer("http://srv")).toBe(true);
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

    it("a reachable server without a name (older build, non-JSON body) is still ok", async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => { throw new Error("html"); } });
      expect(await probeServerHealth("http://srv")).toEqual({ ok: true, name: undefined });
      globalThis.fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ status: "ok", version: "1", name: "  " }) });
      expect(await probeServerHealth("http://srv")).toEqual({ ok: true, name: undefined });
    });
  });
});
