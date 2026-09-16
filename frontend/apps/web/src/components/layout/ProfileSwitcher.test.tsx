import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockNavigate = vi.fn();
const mockRefreshToken = vi.fn().mockResolvedValue({ accessToken: "new-token" });
const mockEveyFetch = vi.fn().mockResolvedValue({});
const mockLogout = vi.fn();
const mockSetProfile = vi.fn();
const mockSetTokens = vi.fn();
const mockInvalidateQueries = vi.fn().mockResolvedValue(undefined);
const mockClear = vi.fn();

vi.mock("@tanstack/react-router", () => ({
  useNavigate: () => mockNavigate,
}));

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  userServiceRefreshToken: (...args: unknown[]) => mockRefreshToken(...args),
  eveyFetch: (...args: unknown[]) => mockEveyFetch(...args),
}));

vi.mock("@evey/store", () => ({
  useAuthStore: Object.assign(
    (selector?: (s: Record<string, unknown>) => unknown) => {
      const state = { profileId: "p1", logout: mockLogout };
      return selector ? selector(state) : state;
    },
    {
      getState: () => ({
        profileId: "p1",
        accessToken: "live-access",
        setProfile: mockSetProfile,
        setTokens: mockSetTokens,
        logout: mockLogout,
      }),
    },
  ),
}));

import { ProfileSwitcher } from "./ProfileSwitcher";

function renderSwitcher(activeId: string = "p1") {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.invalidateQueries = mockInvalidateQueries as unknown as typeof qc.invalidateQueries;
  qc.clear = mockClear as unknown as typeof qc.clear;
  const profiles = [
    { id: "p1", userId: "u1", name: "Alice", isDefault: true },
    { id: "p2", userId: "u1", name: "Bob", isDefault: false },
    { id: "p3", userId: "u1", name: "Kids", isDefault: false },
  ];
  return render(
    <QueryClientProvider client={qc}>
      <ProfileSwitcher profiles={profiles} activeProfileId={activeId} />
    </QueryClientProvider>,
  );
}

describe("ProfileSwitcher", () => {
  beforeEach(() => {
    mockNavigate.mockReset();
    mockRefreshToken.mockClear();
    mockRefreshToken.mockResolvedValue({ accessToken: "new-token" });
    mockSetProfile.mockReset();
    mockSetTokens.mockReset();
    mockInvalidateQueries.mockClear();
    mockClear.mockClear();
    localStorage.clear();
  });

  it("renders the trigger button with active profile initial", () => {
    renderSwitcher();
    const btn = screen.getByTestId("profile-switcher-btn");
    expect(btn).toBeInTheDocument();
    expect(btn).toHaveTextContent("A");
  });

  it("opens the dropdown menu on click", async () => {
    const user = userEvent.setup();
    renderSwitcher();
    expect(screen.queryByTestId("profile-switcher-menu")).not.toBeInTheDocument();
    await user.click(screen.getByTestId("profile-switcher-btn"));
    expect(screen.getByTestId("profile-switcher-menu")).toBeInTheDocument();
    // The dropdown only exposes a "Switch profile" entry now; the actual
    // profile list lives in the modal opened from that entry.
    expect(screen.getByTestId("profile-switcher-switch")).toBeInTheDocument();
  });

  it("closes menu on Escape", async () => {
    const user = userEvent.setup();
    renderSwitcher();
    await user.click(screen.getByTestId("profile-switcher-btn"));
    expect(screen.getByTestId("profile-switcher-menu")).toBeInTheDocument();
    await user.keyboard("{Escape}");
    expect(screen.queryByTestId("profile-switcher-menu")).not.toBeInTheDocument();
  });

  it("switches profile via the modal: refreshToken + setTokens + localStorage + cache clear", async () => {
    const user = userEvent.setup();
    renderSwitcher();
    await user.click(screen.getByTestId("profile-switcher-btn"));
    await user.click(screen.getByTestId("profile-switcher-switch"));
    await user.click(screen.getByTestId("profile-switcher-item-p2"));
    await waitFor(() => expect(mockRefreshToken).toHaveBeenCalledTimes(1));
    expect(mockRefreshToken).toHaveBeenCalledWith({ profileId: "p2" });
    expect(localStorage.getItem("evey:last_profile_id")).toBe("p2");
    expect(mockSetTokens).toHaveBeenCalledWith("new-token");
    expect(mockClear).toHaveBeenCalledTimes(1);
    expect(mockInvalidateQueries).not.toHaveBeenCalled();
  });

  it("does not call setTokens if no accessToken returned", async () => {
    mockRefreshToken.mockResolvedValueOnce({});
    const user = userEvent.setup();
    renderSwitcher();
    await user.click(screen.getByTestId("profile-switcher-btn"));
    await user.click(screen.getByTestId("profile-switcher-switch"));
    await user.click(screen.getByTestId("profile-switcher-item-p2"));
    await waitFor(() => expect(mockRefreshToken).toHaveBeenCalled());
    expect(mockSetTokens).not.toHaveBeenCalled();
  });

  // The revoke used to be sent AFTER logout() emptied the store: the fetcher
  // read the token late, the request left unauthenticated, the server never
  // ended the session. The token is now captured first and carried explicitly.
  it("sign out sends the server revoke with the access token it had before clearing the store", async () => {
    const user = userEvent.setup();
    renderSwitcher();
    await user.click(screen.getByTestId("profile-switcher-btn"));
    await user.click(screen.getByTestId("logout-btn"));
    expect(mockEveyFetch).toHaveBeenCalledWith(expect.objectContaining({
      url: "/api/v1/auth/logout",
      method: "POST",
      headers: { Authorization: "Bearer live-access" },
    }));
    expect(mockLogout).toHaveBeenCalled();
    expect(mockNavigate).toHaveBeenCalledWith({ to: "/login" });
  });

  it("manage profiles button navigates to /profile/manage", async () => {
    const user = userEvent.setup();
    renderSwitcher();
    await user.click(screen.getByTestId("profile-switcher-btn"));
    await user.click(screen.getByTestId("profile-switcher-manage"));
    expect(mockNavigate).toHaveBeenCalledWith({ to: "/profile/settings" });
  });

  it("falls back to isDefault when activeProfileId has no match", () => {
    renderSwitcher("unknown-id");
    const btn = screen.getByTestId("profile-switcher-btn");
    expect(btn).toHaveTextContent("A");
  });
});
