// A page load on /login with a live refresh cookie: the session is renewed and
// the visitor lands on the home, instead of being asked for a password while
// still signed in. Whether asking is right is authRefresh's call; the route
// only leaves the form alone when it was asked for or an SSO attempt just
// failed.
import { describe, it, expect, vi, beforeEach } from "vitest";

const renew = vi.hoisted(() => vi.fn(async () => false));

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  createFileRoute: () => (def: Record<string, unknown>) => def,
  redirect: (to: { to: string }) => to,
}));

vi.mock("@evey/store", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/store")>()),
  useAuthStore: { getState: () => ({ accessToken: null }) },
}));

vi.mock("@/lib/authRefresh", () => ({ renewOnLoginArrival: renew }));
vi.mock("@/lib/demoAutoLogin", () => ({
  demoAutoLoginAllowed: () => false,
  DEMO_USERNAME: "",
  DEMO_PASSWORD: "",
}));

type Search = { manual?: boolean; oidc_error?: string };
type Route = { beforeLoad: (a: { context: unknown; search: Search }) => Promise<void> };

async function load(search: Search = {}): Promise<string | null> {
  const route = (await import("./login")).Route as unknown as Route;
  try {
    await route.beforeLoad({ context: {}, search });
    return null;
  } catch (e) {
    return (e as { to: string }).to;
  }
}

describe("/login with no usable token in storage", () => {
  beforeEach(() => {
    renew.mockClear();
    renew.mockResolvedValue(false);
  });

  it("sends a renewed session to the home without the form", async () => {
    renew.mockResolvedValue(true);
    expect(await load()).toBe("/home");
  });

  it("shows the form when the session could not be renewed", async () => {
    expect(await load()).toBeNull();
    expect(renew).toHaveBeenCalledOnce();
  });

  it("leaves the form alone when it was asked for or an SSO attempt just failed", async () => {
    renew.mockResolvedValue(true);
    expect(await load({ manual: true })).toBeNull();
    expect(await load({ oidc_error: "denied" })).toBeNull();
    expect(renew).not.toHaveBeenCalled();
  });
});
