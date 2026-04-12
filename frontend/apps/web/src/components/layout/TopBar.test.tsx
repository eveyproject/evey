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
      const state = { isAdmin: false, logout: vi.fn() };
      return selector ? selector(state) : state;
    },
    { getState: () => ({ isAdmin: false, logout: vi.fn() }) },
  ),
}));

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn().mockResolvedValue(undefined),
  userServiceLogout: vi.fn().mockResolvedValue(undefined),
  useUserServiceGetCurrentUser: () => ({
    data: { username: "gravity" },
  }),
  useUserServiceListProfiles: () => ({ data: { profiles: [] } }),
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

  it("renders user menu button with initial", () => {
    renderTopBar();
    expect(screen.getByRole("button", { name: "User menu" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "User menu" })).toHaveTextContent("G");
  });

  it("user menu opens on avatar click and shows menu items", async () => {
    const user = userEvent.setup();
    renderTopBar();
    expect(screen.queryByText("Settings")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "User menu" }));
    expect(screen.getByText("Settings")).toBeInTheDocument();
    expect(screen.getByText("Watch History")).toBeInTheDocument();
    expect(screen.getByText("Quick Connect")).toBeInTheDocument();
  });

  it("logout button is visible in user menu and calls logout + navigate", async () => {
    const user = userEvent.setup();
    renderTopBar();
    await user.click(screen.getByRole("button", { name: "User menu" }));
    const signOut = screen.getByTestId("logout-btn");
    expect(signOut).toBeInTheDocument();
    await user.click(signOut);
    expect(mockNavigate).toHaveBeenCalledWith({ to: "/login" });
  });

  it("user menu closes when Escape is pressed", async () => {
    const user = userEvent.setup();
    renderTopBar();
    await user.click(screen.getByRole("button", { name: "User menu" }));
    expect(screen.getByText("Settings")).toBeInTheDocument();
    await user.keyboard("{Escape}");
    expect(screen.queryByText("Settings")).not.toBeInTheDocument();
  });

  it("does not show Admin menu item when user is not admin", async () => {
    const user = userEvent.setup();
    renderTopBar();
    await user.click(screen.getByRole("button", { name: "User menu" }));
    expect(screen.queryByText("Admin")).not.toBeInTheDocument();
  });
});
