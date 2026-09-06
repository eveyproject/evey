import { beforeEach, describe, expect, it, vi } from "vitest";

// fetcher-setup is pure wiring: the useful assertion is not "the setter was
// called" but "the callback it registered produces the right effect" (auth
// store mutated, navigation fired, stored token cleared). Spy on @evey/api's
// setters so we can capture and invoke those callbacks directly, matching
// the real exported signatures in packages/api/src/fetcher.ts.
// vi.hoisted: vi.mock is lifted above every const, so a plain module-level
// object is still in its temporal dead zone when the factory runs.
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
vi.mock("@evey/api", () => api);

const routerReplace = vi.hoisted(() => vi.fn());
vi.mock("expo-router", () => ({ router: { replace: routerReplace } }));

// clientIdentity reads the app manifest through expo-constants, whose real
// module reaches for a native EventEmitter at import time. The User-Agent it
// builds is asserted below, so the shape has to be the one it actually reads.
vi.mock("expo-constants", () => ({
  default: { expoConfig: { name: "Evey", version: "0.0.1-test" } },
}));

// vi.hoisted: vi.mock is lifted above every const, so a plain module-level
// spy is still in its temporal dead zone when the factory runs.
const clearRefreshToken = vi.hoisted(() => vi.fn());
vi.mock("./refresh-token", () => ({ clearRefreshToken }));

// Overrides the global inert stub (src/test/__mocks__/expo-secure-store.ts)
// so the legacy-fallback test can control what server-config.getServerUrl
// reads, matching expo-secure-store's real async signature.
const store = vi.hoisted(() => ({ values: {} as Record<string, string | null> }));
vi.mock("expo-secure-store", () => ({
  getItemAsync: (key: string) => Promise.resolve(store.values[key] ?? null),
  setItemAsync: () => Promise.resolve(),
  deleteItemAsync: () => Promise.resolve(),
}));

import { useAuthStore, useLocaleStore, useServerListStore } from "@evey/store";
// Real module (not mocked): identity matters for item 5 below, and its own
// refresh mechanics are already covered by auth-refresh.test.ts.
import { forceRefresh } from "./auth-refresh";
import { initFetcher } from "./fetcher-setup";

beforeEach(() => {
  vi.clearAllMocks();
  store.values = {};
  useAuthStore.setState({
    accessToken: null,
    tokensByServer: {},
    profileId: null,
    isAdmin: false,
    impersonatedBy: null,
    impersonatedUsername: null,
    impersonationOrigin: null,
  });
  useServerListStore.setState({ servers: {}, activeServerId: null });
  useLocaleStore.setState({ locale: "en-US", source: null });
});

describe("initFetcher - unauthorized handler", () => {
  it("logs out, clears the stored refresh token, and routes to /login", async () => {
    await initFetcher();
    useAuthStore.setState({ accessToken: "token-A" });

    const handler = api.setUnauthorizedHandler.mock.calls[0]?.[0] as () => void;
    handler();

    expect(useAuthStore.getState().accessToken).toBeNull();
    expect(clearRefreshToken).toHaveBeenCalledTimes(1);
    expect(routerReplace).toHaveBeenCalledWith("/login");
  });
});

describe("initFetcher - refresh-failure latch", () => {
  // Repeated initFetcher() calls across this file each add another subscriber
  // to the (singleton) auth store, but every one evaluates the same equality
  // check on the same state transition, so toHaveBeenCalled()/not are stable
  // regardless of how many prior tests already subscribed.
  it("lifts the latch when a NEW access token lands outside the fetcher's own refresh", async () => {
    await initFetcher();
    api.resetRefreshFailure.mockClear();

    useAuthStore.getState().setTokens("token-A");

    expect(api.resetRefreshFailure).toHaveBeenCalled();
  });

  it("does not lift the latch when the reasserted access token is identical", async () => {
    await initFetcher();
    useAuthStore.getState().setTokens("token-A");
    api.resetRefreshFailure.mockClear();

    useAuthStore.getState().setTokens("token-A");

    expect(api.resetRefreshFailure).not.toHaveBeenCalled();
  });
});

describe("initFetcher - base URL resolution", () => {
  it("uses the active server's URL", async () => {
    useServerListStore.setState({
      servers: { s1: { id: "s1", url: "http://active.test", lastUsedAt: Date.now() } },
      activeServerId: "s1",
    });

    await initFetcher();

    expect(api.setBaseURL).toHaveBeenCalledWith("http://active.test");
  });

  it("falls back to the legacy stored server URL when there is no active server", async () => {
    store.values["evey-server-url"] = "http://legacy.test";

    await initFetcher();

    expect(api.setBaseURL).toHaveBeenCalledWith("http://legacy.test");
  });
});

describe("initFetcher - extra headers", () => {
  it("sets the persistent-session flag and a non-empty User-Agent", async () => {
    await initFetcher();

    const headers = api.setExtraHeaders.mock.calls[0]?.[0] as Record<string, string>;
    expect(headers["X-Persistent-Session"]).toBe("true");
    expect(typeof headers["User-Agent"]).toBe("string");
    expect(headers["User-Agent"].length).toBeGreaterThan(0);
  });
});

describe("initFetcher - refresh handler", () => {
  it("wires forceRefresh directly, so concurrent refreshes coalesce on one round-trip", async () => {
    await initFetcher();

    expect(api.setRefreshHandler).toHaveBeenCalledWith(forceRefresh);
  });
});

describe("initFetcher - token and locale plumbing", () => {
  it("reads the live access token and locale from the stores it wires", async () => {
    await initFetcher();

    useAuthStore.setState({ accessToken: "token-live" });
    const tokenProvider = api.setTokenProvider.mock.calls[0]?.[0] as () => string | null;
    expect(tokenProvider()).toBe("token-live");

    useLocaleStore.setState({ locale: "fr-FR" });
    const localeProvider = api.setLocaleProvider.mock.calls[0]?.[0] as () => string;
    expect(localeProvider()).toBe("fr-FR");
  });

  it("routes a fetcher-issued token through the real auth store setter", async () => {
    await initFetcher();

    const tokensUpdater = api.setTokensUpdater.mock.calls[0]?.[0] as (access: string) => void;
    tokensUpdater("token-from-refresh");

    expect(useAuthStore.getState().accessToken).toBe("token-from-refresh");
  });
});
