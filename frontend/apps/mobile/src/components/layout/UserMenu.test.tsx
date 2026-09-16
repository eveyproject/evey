import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import React from "react";

const api = vi.hoisted(() => ({ user: undefined as { id: string } | undefined, profiles: undefined as unknown, listArgs: null as any }));
vi.mock("@evey/api", () => ({
  useUserServiceGetCurrentUser: () => ({ data: api.user }),
  useUserServiceListProfiles: (id: string, o: any) => {
    api.listArgs = { id, enabled: o.query.enabled };
    return { data: api.profiles };
  },
}));
vi.mock("@evey/store", () => ({ useAuthStore: (sel: (s: { profileId: string }) => unknown) => sel({ profileId: "p2" }) }));
vi.mock("./ProfileSwitcher", () => ({
  ProfileSwitcher: ({ profiles, activeProfileId, size }: any) =>
    React.createElement("div", { "data-testid": "switcher", "data-active": activeProfileId, "data-size": size }, String(profiles.length)),
}));

import { UserMenu } from "./UserMenu";

beforeEach(() => {
  api.user = undefined;
  api.profiles = undefined;
});

// The topbar menu asks for the profile list only once the user is known,
// renders nothing until there is at least one profile, and hands the
// switcher the active profile and the asked size.
describe("UserMenu (mobile)", () => {
  it("holds the list query until the user is known and renders nothing without profiles", () => {
    const { container } = render(<UserMenu />);
    expect(api.listArgs).toEqual({ id: "", enabled: false });
    expect(container.firstChild).toBeNull();
  });

  it("hands the switcher the profiles, the active one and the size", () => {
    api.user = { id: "u1" };
    api.profiles = { profiles: [{ id: "p1" }, { id: "p2" }] };
    render(<UserMenu size={40} />);
    expect(api.listArgs).toEqual({ id: "u1", enabled: true });
    const sw = screen.getByTestId("switcher");
    expect(sw.textContent).toBe("2");
    expect(sw.getAttribute("data-active")).toBe("p2");
    expect(sw.getAttribute("data-size")).toBe("40");
  });
});
