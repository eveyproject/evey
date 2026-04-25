import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { TopBar } from "./TopBar";

const mockNavigate = vi.fn();

vi.mock("@tanstack/react-router", () => ({
  useNavigate: () => mockNavigate,
  useRouterState: () => ({ location: { pathname: "/" } }),
  Link: ({ children, ...props }: { children: React.ReactNode; to: string }) => (
    <a href={props.to}>{children}</a>
  ),
}));

vi.mock("@evey/store", () => ({
  useAuthStore: Object.assign(
    (selector?: (s: Record<string, unknown>) => unknown) => {
      const state = { isAdmin: false, profileId: null, logout: vi.fn(), setProfile: vi.fn(), setTokens: vi.fn() };
      return selector ? selector(state) : state;
    },
    { getState: () => ({ isAdmin: false, profileId: null, logout: vi.fn(), setProfile: vi.fn(), setTokens: vi.fn() }) },
  ),
}));

let mockProfiles: Array<{ id: string; userId: string; name: string; isDefault: boolean }> = [];
vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn().mockResolvedValue(undefined),
  userServiceLogout: vi.fn().mockResolvedValue(undefined),
  userServiceRefreshToken: vi.fn().mockResolvedValue({ accessToken: "t" }),
  useUserServiceGetCurrentUser: () => ({
    data: { id: "u1", username: "gravity" },
  }),
  useUserServiceListProfiles: () => ({ data: { profiles: mockProfiles } }),
  useNotificationServiceGetUnreadCount: () => ({ data: { count: 3 } }),
  useNotificationServiceListNotifications: () => ({
    data: { notifications: [] },
  }),
  useNotificationServiceMarkAllRead: () => ({ mutate: vi.fn() }),
}));

vi.mock("@/features/search/SearchDropdown", () => ({
  SearchDropdown: () => null,
}));

function createTestQueryClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false } } });
}

function renderTopBar() {
  const qc = createTestQueryClient();
  return render(
    <QueryClientProvider client={qc}>
      <TopBar />
    </QueryClientProvider>,
  );
}

describe("TopBar", () => {
  beforeEach(() => {
    mockNavigate.mockReset();
    mockProfiles = [];
  });

  it("renders the search input", () => {
    renderTopBar();
    expect(screen.getByTestId("topbar-search-input")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("Search movies, series\u2026")).toBeInTheDocument();
  });

  it("Ctrl+K focuses the search input", async () => {
    const user = userEvent.setup();
    renderTopBar();
    const input = screen.getByTestId("topbar-search-input");
    expect(document.activeElement).not.toBe(input);
    await user.keyboard("{Control>}k{/Control}");
    expect(document.activeElement).toBe(input);
  });

  it("renders the notification bell", () => {
    renderTopBar();
    expect(screen.getByRole("button", { name: "Notifications" })).toBeInTheDocument();
  });

  it("renders user menu button with active profile initial", () => {
    mockProfiles = [{ id: "p1", userId: "u1", name: "Alice", isDefault: true }];
    renderTopBar();
    expect(screen.getByRole("button", { name: "User menu" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "User menu" })).toHaveTextContent("A");
  });

  it("user menu opens on avatar click and shows menu items", async () => {
    mockProfiles = [{ id: "p1", userId: "u1", name: "Alice", isDefault: true }];
    const user = userEvent.setup();
    renderTopBar();
    expect(screen.queryByText("Settings")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "User menu" }));
    expect(screen.getByText("Settings")).toBeInTheDocument();
    expect(screen.getByText("Watch History")).toBeInTheDocument();
    expect(screen.getByText("Quick Connect")).toBeInTheDocument();
  });

  it("logout button is visible in user menu and calls logout + navigate", async () => {
    mockProfiles = [{ id: "p1", userId: "u1", name: "Alice", isDefault: true }];
    const user = userEvent.setup();
    renderTopBar();
    await user.click(screen.getByRole("button", { name: "User menu" }));
    const signOut = screen.getByTestId("logout-btn");
    expect(signOut).toBeInTheDocument();
    await user.click(signOut);
    expect(mockNavigate).toHaveBeenCalledWith({ to: "/login" });
  });

  it("user menu closes when Escape is pressed", async () => {
    mockProfiles = [{ id: "p1", userId: "u1", name: "Alice", isDefault: true }];
    const user = userEvent.setup();
    renderTopBar();
    await user.click(screen.getByRole("button", { name: "User menu" }));
    expect(screen.getByText("Settings")).toBeInTheDocument();
    await user.keyboard("{Escape}");
    expect(screen.queryByText("Settings")).not.toBeInTheDocument();
  });

  it("does not show Admin menu item when user is not admin", async () => {
    mockProfiles = [{ id: "p1", userId: "u1", name: "Alice", isDefault: true }];
    const user = userEvent.setup();
    renderTopBar();
    await user.click(screen.getByRole("button", { name: "User menu" }));
    expect(screen.queryByText("Admin")).not.toBeInTheDocument();
  });

  it("renders unified user menu with one profile (no separate switcher button)", () => {
    mockProfiles = [{ id: "p1", userId: "u1", name: "Alice", isDefault: true }];
    renderTopBar();
    // Only one avatar button — the profile switcher and user menu are merged.
    expect(screen.getAllByTestId("profile-switcher-btn")).toHaveLength(1);
  });

  it("exposes a Switch profile entry that opens the picker modal", async () => {
    mockProfiles = [
      { id: "p1", userId: "u1", name: "Alice", isDefault: true },
      { id: "p2", userId: "u1", name: "Bob", isDefault: false },
    ];
    const user = userEvent.setup();
    renderTopBar();
    await user.click(screen.getByRole("button", { name: "User menu" }));
    const switchEntry = screen.getByTestId("profile-switcher-switch");
    expect(switchEntry).toBeInTheDocument();
    await user.click(switchEntry);
    expect(screen.getByTestId("profile-switcher-item-p2")).toBeInTheDocument();
  });
});
