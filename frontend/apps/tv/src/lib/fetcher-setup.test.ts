import { beforeEach, describe, expect, it, vi } from "vitest";

// fetcher-setup is pure wiring: assert what the registered callbacks DO, not
// that a setter was called. Spread the real @evey/api so the file under test
// keeps every other export it imports (mockShapeGuard).
const api = vi.hoisted(() => ({
  setBaseURL: vi.fn(),
  setExtraHeaders: vi.fn(),
  setTokenProvider: vi.fn(),
  setTokensUpdater: vi.fn(),
  setLocaleProvider: vi.fn(),
  setUnauthorizedHandler: vi.fn(),
  setRefreshHandler: vi.fn(),
  resetRefreshFailure: vi.fn(),
}));
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  ...api,
}));

const routerReplace = vi.hoisted(() => vi.fn());
vi.mock("expo-router", () => ({ router: { replace: routerReplace } }));

const serverUrl = vi.hoisted(() => ({ value: null as string | null }));
vi.mock("./server-config", () => ({ getServerUrl: () => Promise.resolve(serverUrl.value) }));

import { useAuthStore, useLocaleStore } from "@evey/store";
import { forceRefresh } from "./auth-refresh";
import { initFetcher } from "./fetcher-setup";

beforeEach(() => {
  vi.clearAllMocks();
  serverUrl.value = null;
  useAuthStore.setState({ accessToken: null });
  useLocaleStore.setState({ locale: "fr-FR" });
});

describe("initFetcher (tv) - unauthorized handler", () => {
  // A TV has no keyboard: a 401 scoped to one call while a token is still
  // held must not eject the viewer. Only a gone session routes to login.
  it("routes to /login only once the session is actually gone", async () => {
    await initFetcher();
    const handler = api.setUnauthorizedHandler.mock.calls[0]?.[0] as () => void;

    useAuthStore.setState({ accessToken: "still-held" });
    handler();
    expect(routerReplace).not.toHaveBeenCalled();

    useAuthStore.setState({ accessToken: null });
    handler();
    expect(routerReplace).toHaveBeenCalledWith("/login");
  });
});

describe("initFetcher (tv) - providers", () => {
  it("reads the live token and locale from the stores", async () => {
    await initFetcher();
    const tokenProvider = api.setTokenProvider.mock.calls[0]?.[0] as () => string | null;
    const localeProvider = api.setLocaleProvider.mock.calls[0]?.[0] as () => string;

    expect(tokenProvider()).toBeNull();
    useAuthStore.setState({ accessToken: "tok-1" });
    expect(tokenProvider()).toBe("tok-1");
    expect(localeProvider()).toBe("fr-FR");
  });

  it("lifts the refresh-failure latch on a NEW token only", async () => {
    await initFetcher();
    api.resetRefreshFailure.mockClear();

    useAuthStore.setState({ accessToken: "tok-A" });
    expect(api.resetRefreshFailure).toHaveBeenCalled();

    api.resetRefreshFailure.mockClear();
    useAuthStore.setState({ accessToken: "tok-A" });
    expect(api.resetRefreshFailure).not.toHaveBeenCalled();
    useAuthStore.setState({ accessToken: null });
    expect(api.resetRefreshFailure).not.toHaveBeenCalled();
  });
});

describe("initFetcher (tv) - base URL, headers, refresh", () => {
  it("sets the stored server as base URL, skipping it when none is stored", async () => {
    await initFetcher();
    expect(api.setBaseURL).not.toHaveBeenCalled();

    serverUrl.value = "http://tv.test";
    await initFetcher();
    expect(api.setBaseURL).toHaveBeenCalledWith("http://tv.test");
  });

  it("identifies the TV app in the User-Agent and keeps the session persistent", async () => {
    await initFetcher();
    const headers = api.setExtraHeaders.mock.calls[0]?.[0] as Record<string, string>;
    expect(headers["X-Persistent-Session"]).toBe("true");
    // The build rides in the product token, like the phone's.
    expect(headers["User-Agent"]).toMatch(/^EveyTV\/\d+\.\d+\.\d+ \(Android TV/);
  });

  it("wires forceRefresh itself so concurrent refreshes coalesce", async () => {
    await initFetcher();
    expect(api.setRefreshHandler).toHaveBeenCalledWith(forceRefresh);
  });
});
