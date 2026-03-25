import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { AuthLayout } from "./AuthLayout";

let mockPathname = "/home";
let mockIsAdmin = false;

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to, ...rest }: { children: React.ReactNode; to: string; [k: string]: unknown }) => (
    <a href={to} {...rest}>{children}</a>
  ),
  useRouterState: () => ({ location: { pathname: mockPathname } }),
}));

vi.mock("@evey/store", () => ({
  useAuthStore: (selector: (s: { isAdmin: boolean }) => unknown) =>
    selector({ isAdmin: mockIsAdmin }),
}));

vi.mock("./Sidebar", () => ({
  Sidebar: ({ onNavigate }: { onNavigate?: () => void }) => (
    <nav data-testid="sidebar" onClick={onNavigate}>Sidebar</nav>
  ),
}));

vi.mock("./TopBar", () => ({
  TopBar: () => <div data-testid="topbar">TopBar</div>,
}));

vi.mock("@/hooks/useKeyboardShortcuts", () => ({
  useKeyboardShortcuts: () => ({ showHelp: false, setShowHelp: vi.fn() }),
}));

vi.mock("@/components/ui/KeyboardShortcutsHelp", () => ({
  KeyboardShortcutsHelp: ({ open }: { open: boolean }) =>
    open ? <div data-testid="keyboard-help">Help</div> : null,
}));

vi.mock("lucide-react", () => {
  const Icon = ({ size, ...rest }: { size?: number; [k: string]: unknown }) => <span data-size={size} {...rest} />;
  return { Home: Icon, Library: Icon, Heart: Icon, Settings: Icon, Search: Icon, Menu: Icon };
});

describe("AuthLayout", () => {
  beforeEach(() => {
    mockPathname = "/home";
    mockIsAdmin = false;
  });

  it("renders children", () => {
    render(<AuthLayout><div data-testid="child">Content</div></AuthLayout>);
    expect(screen.getByTestId("child")).toBeInTheDocument();
  });

  it("renders sidebar and topbar", () => {
    render(<AuthLayout><div>Content</div></AuthLayout>);
    expect(screen.getByTestId("sidebar")).toBeInTheDocument();
    expect(screen.getByTestId("topbar")).toBeInTheDocument();
  });

  it("renders bottom navigation on non-player routes", () => {
    render(<AuthLayout><div>Content</div></AuthLayout>);
    expect(screen.getByRole("navigation", { name: /bottom/i })).toBeInTheDocument();
    expect(screen.getByText("Home")).toBeInTheDocument();
    expect(screen.getByText("Libraries")).toBeInTheDocument();
    expect(screen.getByText("Search")).toBeInTheDocument();
    expect(screen.getByText("Favorites")).toBeInTheDocument();
  });

  it("renders only children on player route (no sidebar/topbar/nav)", () => {
    mockPathname = "/player/123";
    render(<AuthLayout><div data-testid="player">Player</div></AuthLayout>);
    expect(screen.getByTestId("player")).toBeInTheDocument();
    expect(screen.queryByTestId("sidebar")).not.toBeInTheDocument();
    expect(screen.queryByTestId("topbar")).not.toBeInTheDocument();
    expect(screen.queryByRole("navigation", { name: /bottom/i })).not.toBeInTheDocument();
  });

  it("shows admin link in bottom nav when user is admin", () => {
    mockIsAdmin = true;
    render(<AuthLayout><div>Content</div></AuthLayout>);
    expect(screen.getByText("Admin")).toBeInTheDocument();
  });

  it("hides admin link in bottom nav when user is not admin", () => {
    mockIsAdmin = false;
    render(<AuthLayout><div>Content</div></AuthLayout>);
    const nav = screen.getByRole("navigation", { name: /bottom/i });
    expect(nav).not.toHaveTextContent("Admin");
  });

  it("hamburger button opens mobile sidebar overlay", async () => {
    const user = userEvent.setup();
    render(<AuthLayout><div>Content</div></AuthLayout>);
    expect(screen.queryByRole("button", { name: /close menu/i })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /open menu/i }));
    expect(screen.getByRole("button", { name: /close menu/i })).toBeInTheDocument();
  });

  it("clicking overlay backdrop closes mobile sidebar", async () => {
    const user = userEvent.setup();
    render(<AuthLayout><div>Content</div></AuthLayout>);
    await user.click(screen.getByRole("button", { name: /open menu/i }));
    expect(screen.getByRole("button", { name: /close menu/i })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /close menu/i }));
    expect(screen.queryByRole("button", { name: /close menu/i })).not.toBeInTheDocument();
  });

  it("bottom nav highlights active route", () => {
    mockPathname = "/library";
    render(<AuthLayout><div>Content</div></AuthLayout>);
    const libraryLink = screen.getByText("Libraries").closest("a");
    expect(libraryLink).toHaveClass("text-[var(--accent)]");
  });

  it("bottom nav library link is active on /media/ routes", () => {
    mockPathname = "/media/42";
    render(<AuthLayout><div>Content</div></AuthLayout>);
    const libraryLink = screen.getByText("Libraries").closest("a");
    expect(libraryLink).toHaveClass("text-[var(--accent)]");
  });

  it("admin bottom nav link is active on /admin route", () => {
    mockIsAdmin = true;
    mockPathname = "/admin";
    render(<AuthLayout><div>Content</div></AuthLayout>);
    const adminLink = screen.getByText("Admin").closest("a");
    expect(adminLink).toHaveClass("text-[var(--accent)]");
  });

  it("admin bottom nav link is active on /admin/ sub-routes", () => {
    mockIsAdmin = true;
    mockPathname = "/admin/users";
    render(<AuthLayout><div>Content</div></AuthLayout>);
    const adminLink = screen.getByText("Admin").closest("a");
    expect(adminLink).toHaveClass("text-[var(--accent)]");
  });
});
