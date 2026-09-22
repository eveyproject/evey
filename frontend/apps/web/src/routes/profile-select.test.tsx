import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockNavigate = vi.fn();
const mockSetProfile = vi.fn();
const mockSetTokens = vi.fn();
const mockRefreshToken = vi.fn().mockResolvedValue({ accessToken: "new-token" });
const mockVerifyPin = vi.fn();
let mockProfiles: Array<Record<string, unknown>> = [];
let mockCurrentUser: { id?: string } | undefined = { id: "u1" };
let mockActiveProfileId: string | null = null;

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (def: Record<string, unknown>) => def,
  useNavigate: () => mockNavigate,
}));

vi.mock("@evey/api", () => ({
  useUserServiceGetCurrentUser: () => ({ data: mockCurrentUser, isLoading: false }),
  useUserServiceListProfiles: () => ({ data: { profiles: mockProfiles }, isLoading: false }),
  userServiceRefreshToken: (...args: unknown[]) => mockRefreshToken(...args),
  parentalServiceVerifyPin: (...args: unknown[]) => mockVerifyPin(...args),
}));

vi.mock("@evey/store", () => ({
  useAuthStore: Object.assign(
    (selector?: (s: Record<string, unknown>) => unknown) => {
      const state = { profileId: mockActiveProfileId };
      return selector ? selector(state) : state;
    },
    {
      getState: () => ({
        profileId: mockActiveProfileId,
        setProfile: mockSetProfile,
        setTokens: mockSetTokens,
      }),
    },
  ),
}));

type LazyRoute = { component: React.ComponentType };

async function getPage(): Promise<React.ComponentType> {
  const mod = await import("./profile-select.lazy");
  const route = mod.Route as unknown as LazyRoute;
  return route.component;
}

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.invalidateQueries = (vi.fn().mockResolvedValue(undefined)) as unknown as typeof qc.invalidateQueries;
  return async () => {
    const Page = await getPage();
    return render(
      <QueryClientProvider client={qc}>
        <Page />
      </QueryClientProvider>,
    );
  };
}

describe("ProfileSelect", () => {
  beforeEach(() => {
    mockNavigate.mockReset();
    mockSetProfile.mockReset();
    mockSetTokens.mockReset();
    mockRefreshToken.mockClear();
    mockRefreshToken.mockResolvedValue({ accessToken: "new-token" });
    mockVerifyPin.mockReset();
    mockProfiles = [];
    mockCurrentUser = { id: "u1" };
    mockActiveProfileId = null;
    try { localStorage.clear(); } catch { /* ignore */ }
    vi.resetModules();
  });

  it("redirects to /home when no kids profile exists", async () => {
    mockProfiles = [
      { id: "p1", name: "Alice", isDefault: true },
      { id: "p2", name: "Bob" },
    ];
    const r = await renderPage()();
    await waitFor(() => expect(mockNavigate).toHaveBeenCalledWith({ to: "/home" }));
    r.unmount();
  });

  it("renders picker when a kids profile exists", async () => {
    mockProfiles = [
      { id: "p1", name: "Alice", isDefault: true },
      { id: "p2", name: "Kids", isKids: true },
    ];
    await renderPage()();
    expect(await screen.findByTestId("profile-select-p1")).toBeInTheDocument();
    expect(screen.getByTestId("profile-select-p2")).toBeInTheDocument();
  });

  it("switches profile on click and navigates to /home", async () => {
    mockProfiles = [
      { id: "p1", name: "Alice", isDefault: true },
      { id: "p2", name: "Kids", isKids: true },
    ];
    mockActiveProfileId = "p1";
    await renderPage()();
    await userEvent.click(screen.getByTestId("profile-select-p2"));
    await waitFor(() => {
      expect(mockRefreshToken).toHaveBeenCalledWith({ profileId: "p2" });
      expect(mockNavigate).toHaveBeenCalledWith({ to: "/home" });
    });
  });

  it("requires PIN when switching from kids to non-kids", async () => {
    mockProfiles = [
      { id: "p1", name: "Alice" },
      { id: "p2", name: "Kids", isKids: true },
    ];
    mockActiveProfileId = "p2";
    mockVerifyPin.mockResolvedValue({ valid: true });
    await renderPage()();
    await userEvent.click(screen.getByTestId("profile-select-p1"));
    const input = await screen.findByTestId("profile-pin-prompt-input");
    await userEvent.type(input, "1234");
    await userEvent.click(screen.getByRole("button", { name: /confirm/i }));
    await waitFor(() => {
      // The profile being LEFT is asked first: verify-pin against a profile
      // that carries no PIN answers "valid" to any digits and mints a proof
      // the switch then refuses (@evey/client's verifyParentalPin).
      expect(mockVerifyPin).toHaveBeenNthCalledWith(1, "p2", { pin: "1234" });
      expect(mockRefreshToken).toHaveBeenCalledWith({ profileId: "p1" });
    });
  });

  it("shows error on wrong PIN", async () => {
    mockProfiles = [
      { id: "p1", name: "Alice" },
      { id: "p2", name: "Kids", isKids: true },
    ];
    mockActiveProfileId = "p2";
    mockVerifyPin.mockResolvedValue({ valid: false });
    await renderPage()();
    await userEvent.click(screen.getByTestId("profile-select-p1"));
    const input = await screen.findByTestId("profile-pin-prompt-input");
    await userEvent.type(input, "0000");
    await userEvent.click(screen.getByRole("button", { name: /confirm/i }));
    await waitFor(() => {
      expect(screen.getByTestId("profile-pin-prompt-error")).toBeInTheDocument();
      expect(mockRefreshToken).not.toHaveBeenCalled();
    });
  });
});
