// The picker is shown at cold start - the one moment the profile list has not
// arrived yet, and on a plane never will. These tests pin what the cache keeps
// and, more importantly, WHEN the picker is worth showing at all.
import { describe, it, expect, vi, beforeEach } from "vitest";

const store: Record<string, string> = {};
vi.mock("expo-secure-store", () => ({
  getItemAsync: async (k: string) => store[k] ?? null,
  setItemAsync: async (k: string, v: string) => { store[k] = v; },
  deleteItemAsync: async (k: string) => { delete store[k]; },
}));

import {
  saveProfileCache,
  loadProfileCache,
  clearProfileCache,
  shouldPickProfileOnColdStart,
} from "./profileCache";

beforeEach(() => {
  for (const k of Object.keys(store)) delete store[k];
});

describe("profile cache", () => {
  it("keeps only what the picker draws, and normalises isKids", async () => {
    await saveProfileCache([
      { id: "p1", name: "Papa", avatarPath: "/a.jpg", isKids: false, preferences: { theme: "amoled" } } as never,
      { id: "p2", name: "Lou" } as never,
    ]);

    expect(await loadProfileCache()).toEqual([
      { id: "p1", name: "Papa", avatarPath: "/a.jpg", isKids: false },
      { id: "p2", name: "Lou", isKids: false },
    ]);
  });

  // An empty list is what a failed or not-yet-loaded query looks like. Writing
  // it would erase a good cache and send the next cold start past the picker.
  it("refuses to overwrite a good cache with an empty answer", async () => {
    await saveProfileCache([{ id: "p1", name: "Papa", isKids: false } as never]);
    await saveProfileCache([]);
    await saveProfileCache(undefined);

    expect(await loadProfileCache()).toHaveLength(1);
  });

  it("drops entries with no id rather than drawing a row that leads nowhere", async () => {
    await saveProfileCache([{ name: "ghost" } as never, { id: "p1", name: "Papa" } as never]);
    expect(await loadProfileCache()).toEqual([{ id: "p1", name: "Papa", isKids: false }]);
  });

  it("answers an empty list when nothing was ever written", async () => {
    expect(await loadProfileCache()).toEqual([]);
  });

  it("survives a corrupted value instead of throwing into the render path", async () => {
    store["evey:profiles_min"] = "{not json";
    expect(await loadProfileCache()).toEqual([]);
  });

  it("clears - the list describes one account on one server", async () => {
    await saveProfileCache([{ id: "p1", name: "Papa" } as never]);
    await clearProfileCache();
    expect(await loadProfileCache()).toEqual([]);
  });
});

// The rule the user asked for: the screen exists to stop somebody picking
// wrong. Where nobody can, it is pure friction and must not appear.
describe("shouldPickProfileOnColdStart", () => {
  const adult = { id: "p1", name: "Papa", isKids: false };
  const kid = { id: "p2", name: "Lou", isKids: true };

  it("asks when a household has a kids profile and more than one to choose from", () => {
    expect(shouldPickProfileOnColdStart([adult, kid])).toBe(true);
  });

  it("does not ask when no profile is a kids profile", () => {
    expect(shouldPickProfileOnColdStart([adult, { id: "p3", name: "Maman", isKids: false }])).toBe(false);
  });

  it("does not ask a single-profile account, even a kids one", () => {
    expect(shouldPickProfileOnColdStart([kid])).toBe(false);
    expect(shouldPickProfileOnColdStart([adult])).toBe(false);
  });

  it("does not ask when the cache is empty - never block on what we do not know", () => {
    expect(shouldPickProfileOnColdStart([])).toBe(false);
  });
});
