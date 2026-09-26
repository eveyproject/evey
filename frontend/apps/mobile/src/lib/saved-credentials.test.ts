import { describe, it, expect, vi, beforeEach } from "vitest";

const store = new Map<string, string>();
let broken = false;
vi.mock("expo-secure-store", () => ({
  isAvailableAsync: vi.fn(async () => !broken),
  getItemAsync: vi.fn(async (k: string) => {
    if (broken) throw new Error("no keystore");
    return store.get(k) ?? null;
  }),
  setItemAsync: vi.fn(async (k: string, v: string) => {
    if (broken) throw new Error("no keystore");
    store.set(k, v);
  }),
  deleteItemAsync: vi.fn(async (k: string) => {
    if (broken) throw new Error("no keystore");
    store.delete(k);
  }),
}));

import { canSaveCredentials, clearSavedCredentials, getSavedCredentials, saveCredentials } from "./saved-credentials";

// "Remember me" lives in the keystore, apart from the tokens, and never throws:
// without a keystore the login screen hides the checkbox, and a refused write
// is reported to the caller instead of passing for a saved login.
describe("saved credentials", () => {
  beforeEach(() => {
    store.clear();
    broken = false;
  });

  it("round-trips a login and clears it", async () => {
    expect(await getSavedCredentials()).toBeNull();
    expect(await canSaveCredentials()).toBe(true);
    expect(await saveCredentials("alice", "s3cret")).toBe(true);
    expect(await getSavedCredentials()).toEqual({ username: "alice", password: "s3cret" });
    await clearSavedCredentials();
    expect(await getSavedCredentials()).toBeNull();
    expect(store.size).toBe(0);
  });

  it("treats a half-saved pair as nothing", async () => {
    store.set("evey-saved-username", "alice");
    expect(await getSavedCredentials()).toBeNull();
  });

  it("never throws without a keystore", async () => {
    broken = true;
    await expect(canSaveCredentials()).resolves.toBe(false);
    await expect(saveCredentials("alice", "x")).resolves.toBe(false);
    await expect(getSavedCredentials()).resolves.toBeNull();
    await expect(clearSavedCredentials()).resolves.toBeUndefined();
  });
});
