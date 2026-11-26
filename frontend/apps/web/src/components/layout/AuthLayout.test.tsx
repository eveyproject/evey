import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useEffect } from "react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { AuthLayout } from "./AuthLayout";

let mockPathname = "/home";
// The route the outlet renders: it lags the address by one render on a navigation.
let mockRouteId = "/_auth/home";

// The lock has its own tests; here it would need a router and a query client.
vi.mock("@/features/profile/ProfileLockOverlay", () => ({ ProfileLockOverlay: () => null }));

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to, ...rest }: { children: React.ReactNode; to: string; [k: string]: unknown }) => (
    <a href={to} {...rest}>{children}</a>
  ),
  useRouterState: () => ({ location: { pathname: mockPathname }, matches: [{ routeId: "/_auth" }, { routeId: mockRouteId }] }),
}));

vi.mock("./Sidebar", () => ({
  Sidebar: ({ onNavigate }: { onNavigate?: () => void }) => (
    <nav data-testid="sidebar" onClick={onNavigate}>Sidebar</nav>
  ),
}));

vi.mock("./TopBar", () => ({
  TopBar: ({ leading }: { leading?: React.ReactNode }) => <div data-testid="topbar">{leading}TopBar</div>,
}));

vi.mock("./OfflineBanner", () => ({
  OfflineBanner: () => <div data-testid="offline-banner-mount" />,
}));

vi.mock("@/features/music/GlobalAudioPlayer", () => ({
  GlobalAudioPlayer: () => null,
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
  return { Menu: Icon };
});

describe("AuthLayout", () => {
  beforeEach(() => {
    mockPathname = "/home";
    mockRouteId = "/_auth/home";
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

  it("mounts the offline banner", () => {
    render(<AuthLayout><div>Content</div></AuthLayout>);
    expect(screen.getByTestId("offline-banner-mount")).toBeInTheDocument();
  });

  it("renders only children on player route (no sidebar/topbar)", () => {
    mockPathname = "/player/123";
    mockRouteId = "/_auth/player/$mediaId";
    render(<AuthLayout><div data-testid="player">Player</div></AuthLayout>);
    expect(screen.getByTestId("player")).toBeInTheDocument();
    expect(screen.queryByTestId("sidebar")).not.toBeInTheDocument();
    expect(screen.queryByTestId("topbar")).not.toBeInTheDocument();
  });

  // Leaving the player: the address is already the next page while the outlet
  // still renders the player. Switching the layout on the address moved the
  // player into <main>, which mounted it a second time and started a second
  // play session.
  it("keeps the player where it is while the address has already left it", () => {
    mockPathname = "/player/123";
    mockRouteId = "/_auth/player/$mediaId";
    let mounts = 0;
    function Player() {
      useEffect(() => { mounts += 1; }, []);
      return <div data-testid="player">Player</div>;
    }
    const { rerender } = render(<AuthLayout><Player /></AuthLayout>);
    mockPathname = "/media/123";
    rerender(<AuthLayout><Player /></AuthLayout>);
    expect(mounts).toBe(1);
    expect(screen.queryByTestId("topbar")).not.toBeInTheDocument();
  });

  it("keeps the page where it is while the address has already reached the player", () => {
    mockPathname = "/media/123";
    mockRouteId = "/_auth/media/$mediaId";
    let mounts = 0;
    function Page() {
      useEffect(() => { mounts += 1; }, []);
      return <div>Page</div>;
    }
    const { rerender } = render(<AuthLayout><Page /></AuthLayout>);
    mockPathname = "/player/123";
    rerender(<AuthLayout><Page /></AuthLayout>);
    expect(mounts).toBe(1);
    expect(screen.getByTestId("topbar")).toBeInTheDocument();
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
});
