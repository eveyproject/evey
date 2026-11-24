import { renderWithQueryClient } from "@/test/test-utils";
import { screen } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { useDemoStore } from "@evey/store";
import { AdminNav } from "./AdminNav";

let mockPathname = "/admin";
let mockPluginPages: { pluginId: string; name: string }[] = [];

vi.mock("@/features/plugins/usePluginSlot", () => ({
  usePluginSlotEntries: () => mockPluginPages,
}));

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to, style, ...rest }: { children: React.ReactNode; to: string; style?: React.CSSProperties; [k: string]: unknown }) => (
    <a href={to} style={style} data-testid={`link-${to}`} {...rest}>{children}</a>
  ),
  useRouterState: () => ({ location: { pathname: mockPathname } }),
}));

vi.mock("lucide-react", () => {
  const Icon = ({ size: _size, ...props }: { size?: number; [k: string]: unknown }) => <span {...props} />;
  return {
    LayoutDashboard: Icon,
    PlayCircle: Icon,
    Globe: Icon,
    Library: Icon,
    Tags: Icon,
    HardDrive: Icon,
    Users: Icon,
    Puzzle: Icon,
    Webhook: Icon,
    BarChart3: Icon,
    TrendingUp: Icon,
    FileText: Icon,
    Database: Icon,
    Cpu: Icon,
    SlidersHorizontal: Icon,
    Film: Icon,
    ScrollText: Icon,
    ChevronDown: Icon,
    Sliders: Icon,
    Rows3: Icon,
    Key: Icon,
    Image: Icon,
    Gauge: Icon,
    ScanSearch: Icon,
    Inbox: Icon,
    Share2: Icon,
  };
});

describe("AdminNav", () => {
  it("shows no plugin pages section while no plugin page is installed", () => {
    mockPathname = "/admin";
    mockPluginPages = [];
    renderWithQueryClient(<AdminNav />);
    expect(screen.queryByTestId("admin-nav-plugin-pages")).toBeNull();
  });

  it("puts the plugin pages between Users & Access and Integrations", () => {
    mockPathname = "/admin";
    mockPluginPages = [{ pluginId: "plg-0123456789abcdef01234567", name: "Librarian" }];
    renderWithQueryClient(<AdminNav />);
    const pages = screen.getByTestId("admin-nav-plugin-pages");
    expect(pages).toHaveTextContent("Librarian");
    const before = screen.getByText("Users & Access");
    const after = screen.getByText("Integrations");
    expect(before.compareDocumentPosition(pages) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(pages.compareDocumentPosition(after) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    mockPluginPages = [];
  });

  it("renders all nav sections", () => {
    mockPathname = "/admin";
    renderWithQueryClient(<AdminNav />);
    expect(screen.getByText("Dashboard & System")).toBeInTheDocument();
    expect(screen.getByText("Media")).toBeInTheDocument();
    expect(screen.getByText("Users & Access")).toBeInTheDocument();
    expect(screen.getByText("Integrations")).toBeInTheDocument();
    expect(screen.getByText("Monitoring")).toBeInTheDocument();
    expect(screen.getByText("Maintenance")).toBeInTheDocument();
  });

  it("highlights active section", () => {
    mockPathname = "/admin/users";
    renderWithQueryClient(<AdminNav />);
    const usersLink = screen.getByTestId("link-/admin/users");
    expect(usersLink.style.backgroundColor).toBe("var(--accent)");
    expect(usersLink.style.color).toBe("white");
  });

  it("section links have correct href", () => {
    mockPathname = "/admin";
    renderWithQueryClient(<AdminNav />);
    expect(screen.getByTestId("link-/admin")).toHaveAttribute("href", "/admin");
    expect(screen.getByTestId("link-/admin/sessions")).toHaveAttribute("href", "/admin/sessions");
    expect(screen.getByTestId("link-/admin/users")).toHaveAttribute("href", "/admin/users");
    expect(screen.getByTestId("link-/admin/webhooks")).toHaveAttribute("href", "/admin/webhooks");
  });

  it("does not render removed pages (logs, tasks)", () => {
    mockPathname = "/admin";
    renderWithQueryClient(<AdminNav />);
    expect(screen.queryByText("Logs")).not.toBeInTheDocument();
    expect(screen.queryByText("Tasks")).not.toBeInTheDocument();
  });

  it("hides labels when collapsed", () => {
    mockPathname = "/admin";
    renderWithQueryClient(<AdminNav collapsed />);
    expect(screen.queryByText("Dashboard")).not.toBeInTheDocument();
    expect(screen.queryByText("Dashboard & System")).not.toBeInTheDocument();
  });

  // The server refuses every log route on the demos, so the entry would only
  // lead to a refusal: it is not offered at all there.
  it("hides the Logs entry when the server lists the log routes as locked", () => {
    mockPathname = "/admin";
    useDemoStore.getState().setDemoLocked(["GET /api/v1/admin/logs"]);
    try {
      renderWithQueryClient(<AdminNav />);
      expect(screen.queryByTestId("link-/admin/logs")).not.toBeInTheDocument();
      expect(screen.getByTestId("link-/admin/activity")).toBeInTheDocument();
    } finally {
      useDemoStore.getState().setDemoLocked([]);
    }
  });

  it("offers the Logs entry off the demo", () => {
    mockPathname = "/admin";
    renderWithQueryClient(<AdminNav />);
    expect(screen.getByTestId("link-/admin/logs")).toBeInTheDocument();
  });
});
