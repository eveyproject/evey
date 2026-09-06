// "Who is watching" on the web: asked once per DOCUMENT LOAD, which is this
// platform's equivalent of the cold start the phone and the television use.
// The risk it answers is a browser left open on an adult profile and picked up
// by a child - so the one thing that must NOT happen is asking again on every
// in-app navigation, which would make it a nag people learn to click through.
import { describe, it, expect, vi, beforeEach } from "vitest";

const authState = { accessToken: "", profileId: "p1" as string | null };
const cached = vi.hoisted(() => ({ profiles: [] as Array<{ id: string; name: string; isKids: boolean }> }));

vi.mock("@evey/store", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/store")>()),
  useAuthStore: { getState: () => authState },
  // shouldPickProfileOnColdStart stays REAL: the rule is shared with the
  // devices and is covered by its own tests in @evey/store.
  loadProfileCache: () => Promise.resolve(cached.profiles),
}));

vi.mock("@/lib/authRefresh", () => ({
  isTokenExpired: () => false,
  refreshTokenUnderLock: async () => true,
}));

const adult = { id: "p1", name: "Papa", isKids: false };
const kid = { id: "p2", name: "Lou", isKids: true };

async function beforeLoad(pathname: string): Promise<string | null> {
  // A fresh module per call: the once-per-document flag lives at module scope,
  // which is exactly the behaviour under test.
  const mod = await import("./_auth");
  const fn = (mod.Route as unknown as { options: { beforeLoad: (a: unknown) => Promise<void> } }).options.beforeLoad;
  try {
    await fn({ location: { pathname } });
    return null;
  } catch (thrown) {
    // TanStack has moved the destination between `.to` and `.options.to`
    // across versions; read both rather than pin the test to one.
    const r = thrown as { to?: string; options?: { to?: string } };
    return r?.to ?? r?.options?.to ?? "threw";
  }
}

beforeEach(() => {
  vi.resetModules();
  authState.accessToken = "live-token";
  authState.profileId = "p1";
  cached.profiles = [];
});

describe("web cold start - who is watching", () => {
  it("sends a household with a kids profile to the picker on a fresh load", async () => {
    cached.profiles = [adult, kid];
    expect(await beforeLoad("/home")).toBe("/profile-select");
  });

  it("asks once per document load, never again on in-app navigation", async () => {
    cached.profiles = [adult, kid];
    const mod = await import("./_auth");
    const fn = (mod.Route as unknown as { options: { beforeLoad: (a: unknown) => Promise<void> } }).options.beforeLoad;

    await expect(fn({ location: { pathname: "/home" } })).rejects.toBeTruthy();
    // Same module instance = same document. Every later navigation passes.
    await expect(fn({ location: { pathname: "/library" } })).resolves.toBeUndefined();
    await expect(fn({ location: { pathname: "/settings" } })).resolves.toBeUndefined();
  });

  it("lets a household with no kids profile straight through", async () => {
    cached.profiles = [adult, { id: "p3", name: "Maman", isKids: false }];
    expect(await beforeLoad("/home")).toBeNull();
  });

  it("lets a single-profile account straight through", async () => {
    cached.profiles = [kid];
    expect(await beforeLoad("/home")).toBeNull();
  });

  it("does not redirect the picker to itself", async () => {
    cached.profiles = [adult, kid];
    expect(await beforeLoad("/profile-select")).toBeNull();
  });

  it("stays out of the way when no profile is chosen yet", async () => {
    cached.profiles = [adult, kid];
    authState.profileId = null;
    expect(await beforeLoad("/home")).toBeNull();
  });
});
