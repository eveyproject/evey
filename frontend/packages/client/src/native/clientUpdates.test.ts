import { describe, expect, it, vi } from "vitest";
import {
  CLIENT_UPDATE_MAX_EARLY_DEATHS,
  CLIENT_UPDATE_ROLLBACK_MARKER,
  clientUpdateBootGuard,
  clientUpdateManifestUrl,
  clientUpdateMarkSettled,
  followClientUpdateSource,
  stopFollowingClientUpdateSource,
  watchClientUpdateSettle,
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
    releasedAt: 2_000,
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

  it("puts the override back on every launch without downloading again", async () => {
    const updates = fakeUpdates();
    const store = memoryStore();
    await followClientUpdateSource({ updates, store, serverUrl: "https://a.example", app: "tv" });
    vi.mocked(updates.setUpdateURLAndRequestHeadersOverride).mockClear();
    vi.mocked(updates.checkForUpdateAsync).mockClear();

    const result = await followClientUpdateSource({ updates, store, serverUrl: "https://a.example/", app: "tv" });

    expect(result).toBe("unchanged");
    // expo-updates keeps the override in its own storage, which can drift
    // from ours: it is restated, the download is not redone.
    expect(updates.setUpdateURLAndRequestHeadersOverride).toHaveBeenCalledWith({
      updateUrl: "https://a.example/api/v1/client-updates/tv/manifest",
      requestHeaders: {},
    });
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

  it("reports a failure when the override itself is refused", async () => {
    const onError = vi.fn();
    const updates = fakeUpdates({
      setUpdateURLAndRequestHeadersOverride: vi.fn(() => {
        throw new Error("override refused");
      }),
    });
    const store = memoryStore();

    expect(await followClientUpdateSource({ updates, store, serverUrl: "https://a.example", app: "tv", onError })).toBe("failed");
    expect(onError).toHaveBeenCalledTimes(1);
    // Not recorded: the next launch tries the switch again.
    expect(store.getItem("evey.clientUpdate.source")).toBeNull();
    expect(updates.checkForUpdateAsync).not.toHaveBeenCalled();
  });

  it("stays out of the way when updates are disabled (development builds)", async () => {
    const updates = fakeUpdates({ enabled: false });
    const result = await followClientUpdateSource({ updates, store: memoryStore(), serverUrl: "https://a.example", app: "tv" });

    expect(result).toBe("disabled");
    expect(updates.setUpdateURLAndRequestHeadersOverride).not.toHaveBeenCalled();
  });
});

describe("stopFollowingClientUpdateSource", () => {
  it("clears the override so a removed server is no longer contacted", async () => {
    const updates = fakeUpdates();
    const store = memoryStore();
    await followClientUpdateSource({ updates, store, serverUrl: "https://a.example", app: "mobile" });

    stopFollowingClientUpdateSource({ updates, store });

    expect(updates.setUpdateURLAndRequestHeadersOverride).toHaveBeenLastCalledWith(null);
    // The same server chosen again is a new source: its bundle is fetched.
    vi.mocked(updates.checkForUpdateAsync).mockClear();
    expect(await followClientUpdateSource({ updates, store, serverUrl: "https://a.example", app: "mobile" })).toBe("ready");
    expect(updates.checkForUpdateAsync).toHaveBeenCalledTimes(1);
  });

  it("does nothing in a development build, and survives a refusal", () => {
    const disabled = fakeUpdates({ enabled: false });
    stopFollowingClientUpdateSource({ updates: disabled, store: memoryStore() });
    expect(disabled.setUpdateURLAndRequestHeadersOverride).not.toHaveBeenCalled();

    const onError = vi.fn();
    const refusing = fakeUpdates({
      setUpdateURLAndRequestHeadersOverride: vi.fn(() => {
        throw new Error("override refused");
      }),
    });
    expect(() => stopFollowingClientUpdateSource({ updates: refusing, store: memoryStore(), onError })).not.toThrow();
    expect(onError).toHaveBeenCalledTimes(1);
  });
});

describe("watchClientUpdateSettle", () => {
  it("settles the launch when the user leaves, before the timer", () => {
    const deps = { updates: fakeUpdates(), store: memoryStore() };
    for (let launch = 0; launch < CLIENT_UPDATE_MAX_EARLY_DEATHS; launch++) clientUpdateBootGuard(deps);
    let leave = () => {};
    watchClientUpdateSettle({ ...deps, onLeave: (listener) => (leave = listener), schedule: () => {} });

    leave();

    expect(() => clientUpdateBootGuard(deps)).not.toThrow();
  });

  it("settles the launch once it stayed up long enough", () => {
    const deps = { updates: fakeUpdates(), store: memoryStore() };
    for (let launch = 0; launch < CLIENT_UPDATE_MAX_EARLY_DEATHS; launch++) clientUpdateBootGuard(deps);
    let elapse = () => {};
    watchClientUpdateSettle({ ...deps, onLeave: () => {}, schedule: (callback) => (elapse = callback) });

    elapse();

    expect(() => clientUpdateBootGuard(deps)).not.toThrow();
  });

  it("arms nothing for the bundle built into the app", () => {
    const onLeave = vi.fn();
    const schedule = vi.fn();
    watchClientUpdateSettle({ updates: fakeUpdates({ isEmbeddedLaunch: true }), store: memoryStore(), onLeave, schedule });

    expect(onLeave).not.toHaveBeenCalled();
    expect(schedule).not.toHaveBeenCalled();
  });

  it("does not write when there is nothing to clear", () => {
    const store = memoryStore();
    const deps = { updates: fakeUpdates(), store };
    clientUpdateMarkSettled(deps);
    const setItem = vi.spyOn(store, "setItem");

    clientUpdateMarkSettled(deps);

    expect(setItem).not.toHaveBeenCalled();
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

  it("refuses to run a release older than one that settled here", () => {
    const store = memoryStore();
    clientUpdateMarkSettled({ updates: fakeUpdates({ updateId: "newer", releasedAt: 5_000 }), store });

    expect(() => clientUpdateBootGuard({ updates: fakeUpdates({ updateId: "older", releasedAt: 4_999 }), store })).toThrow(
      CLIENT_UPDATE_ROLLBACK_MARKER,
    );
    expect(() => clientUpdateBootGuard({ updates: fakeUpdates({ updateId: "same", releasedAt: 5_000 }), store })).not.toThrow();
    expect(() => clientUpdateBootGuard({ updates: fakeUpdates({ updateId: "later", releasedAt: 5_001 }), store })).not.toThrow();
  });

  it("only raises that floor when a launch settles, and never lowers it", () => {
    const store = memoryStore();
    clientUpdateBootGuard({ updates: fakeUpdates({ updateId: "newer", releasedAt: 5_000 }), store });
    // It started but did not settle: an older release may still run.
    expect(() => clientUpdateBootGuard({ updates: fakeUpdates({ updateId: "older", releasedAt: 4_000 }), store })).not.toThrow();

    clientUpdateMarkSettled({ updates: fakeUpdates({ updateId: "newer", releasedAt: 5_000 }), store });
    clientUpdateMarkSettled({ updates: fakeUpdates({ updateId: "older", releasedAt: 4_000 }), store });
    expect(store.getItem("evey.clientUpdate.floor")).toBe("5000");
  });

  it("keeps the built-in bundle out of the floor", () => {
    const store = memoryStore();
    clientUpdateMarkSettled({ updates: fakeUpdates({ updateId: "newer", releasedAt: 5_000 }), store });

    expect(() => clientUpdateBootGuard({ updates: fakeUpdates({ isEmbeddedLaunch: true, releasedAt: 1 }), store })).not.toThrow();
    clientUpdateMarkSettled({ updates: fakeUpdates({ isEmbeddedLaunch: true, releasedAt: 9_000 }), store });
    expect(store.getItem("evey.clientUpdate.floor")).toBe("5000");
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
