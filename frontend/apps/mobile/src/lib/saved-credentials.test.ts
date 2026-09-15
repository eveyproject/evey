import { describe, it, expect, vi, beforeEach } from "vitest";

const store = new Map<string, string>();
let broken = false;
vi.mock("expo-secure-store", () => ({
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

import { clearSavedCredentials, getSavedCredentials, saveCredentials } from "./saved-credentials";

// "Remember me" lives in the keystore, apart from the tokens, and never throws:
// without a keystore the checkbox simply does not stick (documented as such -
// whether that silence is acceptable is the user's call, not this test's).
describe("saved credentials", () => {
  beforeEach(() => {
    store.clear();
    broken = false;
  });

  it("round-trips a login and clears it", async () => {
    expect(await getSavedCredentials()).toBeNull();
    await saveCredentials("alice", "s3cret");
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
    await expect(saveCredentials("alice", "x")).resolves.toBeUndefined();
    await expect(getSavedCredentials()).resolves.toBeNull();
    await expect(clearSavedCredentials()).resolves.toBeUndefined();
  });
});
