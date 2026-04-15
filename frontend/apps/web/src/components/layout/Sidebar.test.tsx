import { renderWithQueryClient } from "@/test/test-utils";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { Sidebar } from "./Sidebar";

let mockPathname = "/home";
let mockIsAdmin = false;

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to, onClick }: { children: React.ReactNode; to: string; onClick?: () => void }) => (
    <a href={to} onClick={onClick}>{children}</a>
  ),
  useRouterState: () => ({ location: { pathname: mockPathname } }),
}));

vi.mock("@evey/store", () => ({
  useAuthStore: (selector: (s: { isAdmin: boolean }) => unknown) =>
    selector({ isAdmin: mockIsAdmin }),
}));

vi.mock("@/features/admin/AdminNav", () => ({
  AdminNav: ({ collapsed }: { collapsed?: boolean }) => (
    <div data-testid="admin-nav" data-collapsed={collapsed}>Admin Nav</div>
  ),
}));

vi.mock("lucide-react", () => {
  const Icon = ({ size }: { size?: number }) => <span data-size={size} />;
  return {
    Home: Icon, Library: Icon, Heart: Icon, Music2: Icon, Images: Icon,
    Settings: Icon, PanelLeftClose: Icon, PanelLeft: Icon, ArrowLeft: Icon,
    Headphones: Icon,
  };
});

describe("Sidebar", () => {
  beforeEach(() => {
    mockPathname = "/home";
    mockIsAdmin = false;
    localStorage.clear();
  });

  it("renders main navigation links (collapsed by default)", () => {
    renderWithQueryClient(<Sidebar />);
    const links = screen.getAllByRole("link");
    const hrefs = links.map((l) => l.getAttribute("href"));
    expect(hrefs).toContain("/home");
    expect(hrefs).toContain("/library");
    expect(hrefs).toContain("/favorites");
    expect(hrefs).toContain("/music");
    expect(hrefs).toContain("/photos");
  });

  it("shows Admin link when user is admin", () => {
    mockIsAdmin = true;
    renderWithQueryClient(<Sidebar />);
    const links = screen.getAllByRole("link");
    const hrefs = links.map((l) => l.getAttribute("href"));
    expect(hrefs).toContain("/admin");
  });

  it("does not show Admin link for non-admin", () => {
    mockIsAdmin = false;
    renderWithQueryClient(<Sidebar />);
    expect(screen.queryByRole("link", { name: /^admin$/i })).not.toBeInTheDocument();
  });

  it("shows Evey logo link when expanded", () => {
    renderWithQueryClient(<Sidebar onNavigate={vi.fn()} />);
    expect(screen.getByRole("link", { name: "Evey" })).toBeInTheDocument();
  });

  it("renders AdminNav when on admin route and user is admin", () => {
    mockPathname = "/admin/users";
    mockIsAdmin = true;
    renderWithQueryClient(<Sidebar />);
    expect(screen.getByTestId("admin-nav")).toBeInTheDocument();
  });

  it("shows Back to app link in admin mode", () => {
    mockPathname = "/admin";
    mockIsAdmin = true;
    renderWithQueryClient(<Sidebar />);
    expect(screen.getByRole("link", { name: /back to app/i })).toBeInTheDocument();
  });

  it("toggles collapse on button click", async () => {
    const user = userEvent.setup();
    renderWithQueryClient(<Sidebar />);
    const nav = screen.getByRole("navigation");
    expect(nav).toHaveClass("w-16");
    await user.click(screen.getByRole("button", { name: /expand sidebar/i }));
    expect(nav).toHaveClass("w-56");
  });

  it("calls onNavigate when a nav link is clicked (mobile)", async () => {
    const onNavigate = vi.fn();
    const user = userEvent.setup();
    renderWithQueryClient(<Sidebar onNavigate={onNavigate} />);
    await user.click(screen.getByRole("link", { name: /home/i }));
    expect(onNavigate).toHaveBeenCalled();
  });

  it("does not show collapse button in mobile mode", () => {
    renderWithQueryClient(<Sidebar onNavigate={vi.fn()} />);
    expect(screen.queryByRole("button", { name: /collapse/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /expand/i })).not.toBeInTheDocument();
  });
});
