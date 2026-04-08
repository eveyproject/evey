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
