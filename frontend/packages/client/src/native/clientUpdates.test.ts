import { describe, expect, it, vi } from "vitest";
import {
  CLIENT_UPDATE_MAX_EARLY_DEATHS,
  CLIENT_UPDATE_ROLLBACK_MARKER,
  clientUpdateBootGuard,
  clientUpdateManifestUrl,
  clientUpdateMarkSettled,
  followClientUpdateSource,
  type ClientUpdatesApi,
  type ClientUpdateStore,
} from "./clientUpdates";

function memoryStore(): ClientUpdateStore & { data: Map<string, string> } {
  const data = new Map<string, string>();
  return {
    data,
    getItem: (key) => data.get(key) ?? null,
    setItem: (key, value) => void data.set(key, value),
  };
}

function fakeUpdates(over: Partial<ClientUpdatesApi> = {}): ClientUpdatesApi {
  return {
    enabled: true,
    updateId: "update-1",
    isEmbeddedLaunch: false,
    setUpdateURLAndRequestHeadersOverride: vi.fn(),
    checkForUpdateAsync: vi.fn(async () => ({ isAvailable: true })),
    fetchUpdateAsync: vi.fn(async () => ({ isNew: true })),
    ...over,
  };
}

describe("clientUpdateManifestUrl", () => {
  it("builds the instance's manifest address whatever the trailing slashes", () => {
    expect(clientUpdateManifestUrl("https://evey.example/", "tv")).toBe(
      "https://evey.example/api/v1/client-updates/tv/manifest",
    );
    expect(clientUpdateManifestUrl("http://10.0.0.2:9091", "mobile")).toBe(
      "http://10.0.0.2:9091/api/v1/client-updates/mobile/manifest",
    );
  });
});

describe("followClientUpdateSource", () => {
  it("switches to a new instance and downloads what it offers", async () => {
    const updates = fakeUpdates();
    const store = memoryStore();
    const result = await followClientUpdateSource({ updates, store, serverUrl: "https://a.example", app: "tv" });

    expect(result).toBe("ready");
    expect(updates.setUpdateURLAndRequestHeadersOverride).toHaveBeenCalledWith({
      updateUrl: "https://a.example/api/v1/client-updates/tv/manifest",
      requestHeaders: {},
    });
    expect(updates.fetchUpdateAsync).toHaveBeenCalledTimes(1);
  });

  it("does nothing while the instance is the one already followed", async () => {
    const updates = fakeUpdates();
    const store = memoryStore();
    await followClientUpdateSource({ updates, store, serverUrl: "https://a.example", app: "tv" });
    vi.mocked(updates.setUpdateURLAndRequestHeadersOverride).mockClear();
    vi.mocked(updates.checkForUpdateAsync).mockClear();

    const result = await followClientUpdateSource({ updates, store, serverUrl: "https://a.example/", app: "tv" });

    expect(result).toBe("unchanged");
    expect(updates.setUpdateURLAndRequestHeadersOverride).not.toHaveBeenCalled();
    expect(updates.checkForUpdateAsync).not.toHaveBeenCalled();
  });

  it("switches again when the user moves to another instance", async () => {
    const updates = fakeUpdates();
    const store = memoryStore();
    await followClientUpdateSource({ updates, store, serverUrl: "https://a.example", app: "mobile" });
    const result = await followClientUpdateSource({ updates, store, serverUrl: "https://b.example", app: "mobile" });

    expect(result).toBe("ready");
    expect(updates.setUpdateURLAndRequestHeadersOverride).toHaveBeenLastCalledWith({
      updateUrl: "https://b.example/api/v1/client-updates/mobile/manifest",
      requestHeaders: {},
    });
  });

  it("reports up-to-date when the instance offers nothing new", async () => {
    const updates = fakeUpdates({ checkForUpdateAsync: vi.fn(async () => ({ isAvailable: false })) });
    const result = await followClientUpdateSource({ updates, store: memoryStore(), serverUrl: "https://a.example", app: "tv" });

    expect(result).toBe("up-to-date");
    expect(updates.fetchUpdateAsync).not.toHaveBeenCalled();
  });

  it("keeps the new source when the download fails, and does not redo the switch", async () => {
    const onError = vi.fn();
    const updates = fakeUpdates({
      checkForUpdateAsync: vi.fn(async () => {
        throw new Error("network down");
      }),
    });
    const store = memoryStore();

    expect(await followClientUpdateSource({ updates, store, serverUrl: "https://a.example", app: "tv", onError })).toBe("failed");
    expect(onError).toHaveBeenCalledTimes(1);
    // The override is in place: the launch-time check takes over from here.
    expect(await followClientUpdateSource({ updates, store, serverUrl: "https://a.example", app: "tv", onError })).toBe("unchanged");
  });

  it("stays out of the way when updates are disabled (development builds)", async () => {
    const updates = fakeUpdates({ enabled: false });
    const result = await followClientUpdateSource({ updates, store: memoryStore(), serverUrl: "https://a.example", app: "tv" });

    expect(result).toBe("disabled");
    expect(updates.setUpdateURLAndRequestHeadersOverride).not.toHaveBeenCalled();
  });
});

describe("clientUpdateBootGuard", () => {
  it("lets an update start as long as it has not died too many times in a row", () => {
    const deps = { updates: fakeUpdates(), store: memoryStore() };
    for (let launch = 0; launch < CLIENT_UPDATE_MAX_EARLY_DEATHS; launch++) {
      expect(() => clientUpdateBootGuard(deps)).not.toThrow();
    }
  });

  it("asks for a rollback on the launch after the last allowed death", () => {
    const deps = { updates: fakeUpdates(), store: memoryStore() };
    for (let launch = 0; launch < CLIENT_UPDATE_MAX_EARLY_DEATHS; launch++) clientUpdateBootGuard(deps);

    expect(() => clientUpdateBootGuard(deps)).toThrow(CLIENT_UPDATE_ROLLBACK_MARKER);
  });

  it("forgets the deaths once a launch settled", () => {
    const deps = { updates: fakeUpdates(), store: memoryStore() };
    for (let launch = 0; launch < CLIENT_UPDATE_MAX_EARLY_DEATHS; launch++) clientUpdateBootGuard(deps);
    clientUpdateMarkSettled(deps);

    expect(() => clientUpdateBootGuard(deps)).not.toThrow();
  });

  it("counts each update on its own", () => {
    const store = memoryStore();
    const first = { updates: fakeUpdates({ updateId: "update-1" }), store };
    for (let launch = 0; launch < CLIENT_UPDATE_MAX_EARLY_DEATHS; launch++) clientUpdateBootGuard(first);

    expect(() => clientUpdateBootGuard({ updates: fakeUpdates({ updateId: "update-2" }), store })).not.toThrow();
  });

  it("restarts the count after asking for a rollback", () => {
    const deps = { updates: fakeUpdates(), store: memoryStore() };
    for (let launch = 0; launch < CLIENT_UPDATE_MAX_EARLY_DEATHS; launch++) clientUpdateBootGuard(deps);
    expect(() => clientUpdateBootGuard(deps)).toThrow();

    // Should the same update start again, it gets a full new allowance
    // instead of throwing at every launch.
    expect(() => clientUpdateBootGuard(deps)).not.toThrow();
  });

  it("never throws for the bundle built into the app", () => {
    const store = memoryStore();
    store.setItem("evey.clientUpdate.guard", JSON.stringify({ id: "update-1", deaths: 99 }));

    expect(() => clientUpdateBootGuard({ updates: fakeUpdates({ isEmbeddedLaunch: true }), store })).not.toThrow();
    expect(() => clientUpdateBootGuard({ updates: fakeUpdates({ updateId: null }), store })).not.toThrow();
    expect(() => clientUpdateBootGuard({ updates: fakeUpdates({ enabled: false }), store })).not.toThrow();
  });

  it("survives a store that fails or holds garbage", () => {
    const broken: ClientUpdateStore = {
      getItem: () => {
        throw new Error("keystore locked");
      },
      setItem: () => {
        throw new Error("keystore locked");
      },
    };
    expect(() => clientUpdateBootGuard({ updates: fakeUpdates(), store: broken })).not.toThrow();
    expect(() => clientUpdateMarkSettled({ updates: fakeUpdates(), store: broken })).not.toThrow();

    const garbage = memoryStore();
    garbage.setItem("evey.clientUpdate.guard", "{not json");
    expect(() => clientUpdateBootGuard({ updates: fakeUpdates(), store: garbage })).not.toThrow();
  });
});
