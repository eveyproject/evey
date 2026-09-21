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

import { getRefreshToken, setRefreshToken, clearRefreshToken } from "./refresh-token";

beforeEach(() => {
  Object.keys(store).forEach((k) => delete store[k]);
});

describe("refresh-token", () => {
  it("returns null when no token stored", async () => {
    expect(await getRefreshToken()).toBeNull();
  });

  it("stores and retrieves token", async () => {
    await setRefreshToken("rt_abc123");
    expect(await getRefreshToken()).toBe("rt_abc123");
  });

  it("overwrites existing token", async () => {
    await setRefreshToken("old");
    await setRefreshToken("new");
    expect(await getRefreshToken()).toBe("new");
  });

  it("clears token", async () => {
    await setRefreshToken("rt_abc");
    await clearRefreshToken();
    expect(await getRefreshToken()).toBeNull();
  });

  it("clear is idempotent", async () => {
    await clearRefreshToken();
    expect(await getRefreshToken()).toBeNull();
  });
});
