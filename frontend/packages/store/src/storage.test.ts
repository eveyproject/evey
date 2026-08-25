import { describe, it, expect, beforeEach } from "vitest";
import { getZustandStorage, setZustandStorage } from "./storage";
import type { StateStorage } from "zustand/middleware";

beforeEach(() => {
  // Reset to default (localStorage) by setting null via module internals
  // Re-import fresh state is not possible; reset via setZustandStorage with a new custom one
  // then clear it by re-calling with a no-op and testing getItem returns null
  localStorage.clear();
  // Reset custom storage by pointing back to localStorage behavior
  setZustandStorage({
    getItem: (name) => localStorage.getItem(name),
    setItem: (name, value) => localStorage.setItem(name, value),
    removeItem: (name) => localStorage.removeItem(name),
  });
});

describe("getZustandStorage", () => {
  it("returns storage that can get/set/remove items", () => {
    const storage = getZustandStorage();
    storage.setItem("test-key", "test-value");
    expect(storage.getItem("test-key")).toBe("test-value");
    storage.removeItem("test-key");
    expect(storage.getItem("test-key")).toBeNull();
  });

  it("returns null for missing keys", () => {
    const storage = getZustandStorage();
    expect(storage.getItem("nonexistent")).toBeNull();
  });
});

describe("setZustandStorage", () => {
  it("overrides the storage backend", () => {
    const store: Record<string, string> = {};
    const custom: StateStorage = {
      getItem: (name) => store[name] ?? null,
      setItem: (name, value) => { store[name] = value; },
      removeItem: (name) => { delete store[name]; },
    };

    setZustandStorage(custom);
    const storage = getZustandStorage();

    storage.setItem("k", "v");
    expect(store["k"]).toBe("v");
    expect(storage.getItem("k")).toBe("v");
    storage.removeItem("k");
    expect(storage.getItem("k")).toBeNull();
  });

  it("custom storage is isolated from localStorage", () => {
    const store: Record<string, string> = {};
    setZustandStorage({
      getItem: (name) => store[name] ?? null,
      setItem: (name, value) => { store[name] = value; },
      removeItem: (name) => { delete store[name]; },
    });

    const storage = getZustandStorage();
    storage.setItem("isolated-key", "value");
    expect(localStorage.getItem("isolated-key")).toBeNull();
  });
});

// The bug the block below pins: React Native has no window.localStorage, and
// ESM evaluates the store modules BEFORE the app wires its storage - so the
// persist middleware captured the no-op fallback forever. Servers, theme,
// locale and the auth mirrors were never written, and every launch started
// from scratch while "Manage servers" claimed there was nothing saved.
describe("late-wired zustand storage", () => {
  function memoryStorage(seed: Record<string, string> = {}) {
    const data = new Map(Object.entries(seed));
    const storage: StateStorage = {
      getItem: (k) => data.get(k) ?? null,
      setItem: (k, v) => { data.set(k, v); },
      removeItem: (k) => { data.delete(k); },
    };
    return { storage, data };
  }

  it("writes reach a storage wired AFTER the store module was evaluated", async () => {
    const mod = await import("./theme");
    const storageMod = await import("./storage");
    const { storage, data } = memoryStorage();
    storageMod.setZustandStorage(storage);

    mod.useThemeStore.getState().setTheme("amoled");
    // persist writes asynchronously via the JSON storage - flush microtasks
    await new Promise((r) => setTimeout(r, 0));

    expect(data.get("evey-theme"), "the store must persist into the late-wired storage, not the no-op captured at import").toContain("amoled");
  });

  it("rehydrates already-created stores from the late-wired storage", async () => {
    const mod = await import("./theme");
    const storageMod = await import("./storage");
    const { storage } = memoryStorage({
      "evey-theme": JSON.stringify({ state: { theme: "light-blue" }, version: 0 }),
    });

    expect(mod.useThemeStore.getState().theme).not.toBe("light-blue");
    storageMod.setZustandStorage(storage);
    await new Promise((r) => setTimeout(r, 0));

    expect(mod.useThemeStore.getState().theme, "wiring the real storage must pull the persisted state back in").toBe("light-blue");
  });
});
