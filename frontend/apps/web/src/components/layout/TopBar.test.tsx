import { render, screen, act } from "@testing-library/react";
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
  useUserServiceGetCurrentUser: () => ({
    data: { username: "gravity" },
  }),
  useNotificationServiceGetUnreadCount: () => ({ data: { count: 3 } }),
  useNotificationServiceListNotifications: () => ({
    data: { notifications: [] },
  }),
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
    expect(screen.getByPlaceholderText("Search movies, series...")).toBeInTheDocument();
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
});
