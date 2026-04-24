import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockNavigate = vi.fn();
const mockRefreshToken = vi.fn().mockResolvedValue({ accessToken: "new-token" });
const mockSetProfile = vi.fn();
const mockSetTokens = vi.fn();
const mockInvalidateQueries = vi.fn().mockResolvedValue(undefined);

vi.mock("@tanstack/react-router", () => ({
  useNavigate: () => mockNavigate,
}));

vi.mock("@evey/api", () => ({
  userServiceRefreshToken: (...args: unknown[]) => mockRefreshToken(...args),
}));

vi.mock("@evey/store", () => ({
  useAuthStore: Object.assign(
    (selector?: (s: Record<string, unknown>) => unknown) => {
      const state = { profileId: "p1" };
      return selector ? selector(state) : state;
    },
    {
      getState: () => ({
        profileId: "p1",
        setProfile: mockSetProfile,
        setTokens: mockSetTokens,
      }),
    },
  ),
}));

import { ProfileSwitcher } from "./ProfileSwitcher";

function renderSwitcher(activeId: string = "p1") {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.invalidateQueries = mockInvalidateQueries as unknown as typeof qc.invalidateQueries;
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
    expect(screen.getByText(/Alice/)).toBeInTheDocument();
    expect(screen.getByText(/\(You\)/)).toBeInTheDocument();
    expect(screen.getByText("Bob")).toBeInTheDocument();
    expect(screen.getByText("Kids")).toBeInTheDocument();
  });

  it("closes menu on Escape", async () => {
    const user = userEvent.setup();
    renderSwitcher();
    await user.click(screen.getByTestId("profile-switcher-btn"));
    expect(screen.getByTestId("profile-switcher-menu")).toBeInTheDocument();
    await user.keyboard("{Escape}");
    expect(screen.queryByTestId("profile-switcher-menu")).not.toBeInTheDocument();
  });

  it("switches profile: refreshToken with profileId + setTokens + localStorage + invalidate", async () => {
    const user = userEvent.setup();
    renderSwitcher();
    await user.click(screen.getByTestId("profile-switcher-btn"));
    await user.click(screen.getByTestId("profile-switcher-item-p2"));
    await waitFor(() => expect(mockRefreshToken).toHaveBeenCalledTimes(1));
    expect(mockRefreshToken).toHaveBeenCalledWith({ profileId: "p2" });
    expect(localStorage.getItem("evey:last_profile_id")).toBe("p2");
    expect(mockSetTokens).toHaveBeenCalledWith("new-token");
    expect(mockInvalidateQueries).toHaveBeenCalledTimes(1);
    expect(mockInvalidateQueries).toHaveBeenCalledWith();
  });

  it("does not call setTokens if no accessToken returned", async () => {
    mockRefreshToken.mockResolvedValueOnce({});
    const user = userEvent.setup();
    renderSwitcher();
    await user.click(screen.getByTestId("profile-switcher-btn"));
    await user.click(screen.getByTestId("profile-switcher-item-p2"));
    await waitFor(() => expect(mockRefreshToken).toHaveBeenCalled());
    expect(mockSetTokens).not.toHaveBeenCalled();
  });

  it("manage profiles button navigates to /profile/manage", async () => {
    const user = userEvent.setup();
    renderSwitcher();
    await user.click(screen.getByTestId("profile-switcher-btn"));
    await user.click(screen.getByTestId("profile-switcher-manage"));
    expect(mockNavigate).toHaveBeenCalledWith({ to: "/profile/manage" });
  });

  it("falls back to isDefault when activeProfileId has no match", () => {
    renderSwitcher("unknown-id");
    const btn = screen.getByTestId("profile-switcher-btn");
    expect(btn).toHaveTextContent("A");
  });
});
