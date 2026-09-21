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

import { getServerUrl, setServerUrl, clearServerUrl, pingServer, resolveServerUrl } from "./server-config";

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
    const eveyHealth = { status: "ok", db: "ok", version: "beta-0.0.1" };

    it("returns true when an EVEY server answers", async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => eveyHealth });
      expect(await pingServer("http://srv")).toBe(true);
    });

    // A bare 200 used to be enough, so a captive portal / ISP hijack page
    // passed as "connected" and every later API call failed silently.
    it("returns false when something else answers 200", async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ message: "hi" }) });
      expect(await pingServer("http://srv")).toBe(false);
      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => { throw new Error("not json"); },
      });
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
});

// The API layer and the rest of the app must never disagree about WHICH
// server this is. They did (initFetcher read only the legacy key), and a
// divergence pointed every API call at nothing while the UI looked fine.
describe("resolveServerUrl", () => {
  it("prefers the active server of the multi-server store", async () => {
    await setServerUrl("http://legacy:9091");
    expect(await resolveServerUrl("http://active:9091")).toBe("http://active:9091");
  });

  it("falls back to the legacy key when no server is active", async () => {
    await setServerUrl("http://legacy:9091");
    expect(await resolveServerUrl(null)).toBe("http://legacy:9091");
    expect(await resolveServerUrl(undefined)).toBe("http://legacy:9091");
  });

  it("returns null when neither knows a server", async () => {
    await clearServerUrl();
    expect(await resolveServerUrl(null)).toBeNull();
  });
});
