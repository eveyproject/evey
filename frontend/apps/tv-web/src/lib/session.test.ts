import { describe, expect, it, vi, beforeEach } from "vitest";

const { reloadCatalogue, activeServer } = vi.hoisted(() => ({ reloadCatalogue: vi.fn(), activeServer: { url: null as string | null } }));

vi.mock("../i18n", () => ({ reloadCatalogue }));
vi.mock("./servedServer", () => ({ servingServer: async () => null }));
vi.mock("@evey/api", () => ({ setBaseURL: vi.fn(), setCredentialsMode: vi.fn() }));
vi.mock("@evey/client", () => ({
  createAuthRefresh: () => ({ forceRefresh: vi.fn(), ensureAccessToken: vi.fn() }),
  initFetcher: async () => {},
  parseEveyHealth: () => null,
}));
vi.mock("@evey/store", () => ({
  useAuthStore: { getState: () => ({ logout: vi.fn() }) },
  useServerListStore: { getState: () => ({ getActiveServer: () => (activeServer.url ? { url: activeServer.url } : null) }) },
  normalizeServerUrl: (u: string) => u,
}));

import { bootSession } from "./session";

beforeEach(() => reloadCatalogue.mockReset());

// Measured on 2026-09-23: a reload with a remembered server never fetched its
// catalogue, and every screen fell back to the embedded English.
describe("booting on a remembered server", () => {
  it("loads that server's catalogue", async () => {
    activeServer.url = "http://server";
    await bootSession(() => {});
    expect(reloadCatalogue).toHaveBeenCalled();
  });

  it("asks for nothing while no server is known", async () => {
    activeServer.url = null;
    await bootSession(() => {});
    expect(reloadCatalogue).not.toHaveBeenCalled();
  });
});
