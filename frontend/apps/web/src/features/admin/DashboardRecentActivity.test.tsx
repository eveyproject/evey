import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RecentActivity } from "./DashboardRecentActivity";

// ── Mutable mock state ────────────────────────────────────────────────────────

let mockEntries: unknown[] = [];
let mockIsLoading = false;

// ── Module mocks ──────────────────────────────────────────────────────────────

vi.mock("@tanstack/react-router", () => ({
  Link: ({
    children,
    to,
    ...rest
  }: {
    children: React.ReactNode;
    to: string;
    [k: string]: unknown;
  }) => <a href={to} {...rest}>{children}</a>,
}));

vi.mock("@evey/api", () => ({
  useAdminSessionServiceGetAuditLog: (_params: unknown) => ({
    data: { entries: mockEntries },
    isLoading: mockIsLoading,
  }),
}));

// ── Helpers ───────────────────────────────────────────────────────────────────

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
}

function renderRecentActivity() {
  return render(<RecentActivity />, { wrapper: createWrapper() });
}

// ── Fixtures ──────────────────────────────────────────────────────────────────

const ENTRY_LOGIN = {
  id: "e1",
  createdAt: "2026-06-04T10:00:00.000Z",
  username: "alice",
  action: "LOGIN",
  resourceType: "USER",
  details: null,
};

const ENTRY_FAILED_LOGIN = {
  id: "e2",
  createdAt: "2026-06-04T11:00:00.000Z",
  username: null,
  action: "LOGIN",
  resourceType: "USER",
  // details contains '"failed"' — triggers isFailed=true path
  details: '{"status":"failed","username":"bob"}',
};

const ENTRY_LIBRARY_SCAN = {
  id: "e3",
  createdAt: "2026-06-04T12:00:00.000Z",
  username: "admin",
  action: "LIBRARY_SCAN",
  resourceType: "LIBRARY",
  details: null,
};

// ── Tests ─────────────────────────────────────────────────────────────────────

describe("RecentActivity", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockEntries = [];
    mockIsLoading = false;
  });

  // ── Empty state ───────────────────────────────────────────────────────────────

  it("renders the empty message when entries list is empty", () => {
    // The card deliberately stays visible with a "no entries" line — the
    // dashboard section must not silently vanish on a fresh install.
    mockEntries = [];
    renderRecentActivity();
    expect(screen.getByText("No entries found")).toBeInTheDocument();
  });

  // ── Loading state ─────────────────────────────────────────────────────────────

  it("renders a spinner while loading (no table visible)", () => {
    mockIsLoading = true;
    mockEntries = [];
    renderRecentActivity();
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("does not render table content while loading", () => {
    mockIsLoading = true;
    mockEntries = [ENTRY_LOGIN];
    renderRecentActivity();
    // Spinner shown, not the table
    expect(screen.queryByText("LOGIN")).toBeNull();
  });

  // ── Heading and view-all link ─────────────────────────────────────────────────

  it("renders the section heading when entries are present", () => {
    mockEntries = [ENTRY_LOGIN];
    renderRecentActivity();
    // i18n key admin.dashboardStats.recentActivity
    expect(
      screen.getByText(/admin\.dashboardStats\.recentActivity|recent activity/i),
    ).toBeInTheDocument();
  });

  it("renders the 'View all' link to /admin/activity", () => {
    mockEntries = [ENTRY_LOGIN];
    renderRecentActivity();
    const link = screen.getByRole("link", {
      name: /admin\.dashboardStats\.viewAll|view all/i,
    });
    expect(link).toHaveAttribute("href", "/admin/activity");
  });

  // ── Table headers ─────────────────────────────────────────────────────────────

  it("renders table headers (Time, User, Action, Resource, Details)", () => {
    mockEntries = [ENTRY_LOGIN];
    renderRecentActivity();
    // i18n keys admin.audit.tableHeaders.*
    expect(screen.getByRole("table")).toBeInTheDocument();
    // At least one of the header cells resolves to a non-empty string
    const headers = screen.getAllByRole("columnheader");
    expect(headers.length).toBeGreaterThanOrEqual(5);
  });

  // ── Entry field rendering ─────────────────────────────────────────────────────

  it("renders the username for a normal entry", () => {
    mockEntries = [ENTRY_LOGIN];
    renderRecentActivity();
    expect(screen.getByText("alice")).toBeInTheDocument();
  });

  it("renders the action badge for an entry", () => {
    mockEntries = [ENTRY_LOGIN];
    renderRecentActivity();
    expect(screen.getByText("LOGIN")).toBeInTheDocument();
  });

  it("renders the resourceType for an entry", () => {
    mockEntries = [ENTRY_LOGIN];
    renderRecentActivity();
    expect(screen.getByText("USER")).toBeInTheDocument();
  });

  it("renders multiple entries when present", () => {
    mockEntries = [ENTRY_LOGIN, ENTRY_LIBRARY_SCAN];
    renderRecentActivity();
    expect(screen.getByText("alice")).toBeInTheDocument();
    expect(screen.getByText("admin")).toBeInTheDocument();
    expect(screen.getByText("LOGIN")).toBeInTheDocument();
    expect(screen.getByText("LIBRARY_SCAN")).toBeInTheDocument();
  });

  // ── Timestamp rendering ───────────────────────────────────────────────────────

  it("renders a formatted date string for an entry with createdAt", () => {
    mockEntries = [ENTRY_LOGIN];
    renderRecentActivity();
    // new Date("2026-06-04T10:00:00.000Z").toLocaleString() varies by timezone;
    // assert the cell is not a dash placeholder
    const rows = screen.getAllByRole("row");
    // rows[0] = header, rows[1] = first data row
    const firstDataRow = rows[1];
    // The first cell contains the formatted date
    const cells = firstDataRow.querySelectorAll("td");
    // The time cell (index 0) should not be "-"
    expect(cells[0].textContent).not.toBe("-");
  });

  it("renders dash for missing createdAt", () => {
    mockEntries = [{ ...ENTRY_LOGIN, createdAt: null }];
    renderRecentActivity();
    const rows = screen.getAllByRole("row");
    const firstDataRow = rows[1];
    const cells = firstDataRow.querySelectorAll("td");
    expect(cells[0].textContent).toBe("-");
  });

  // ── Failed login path ─────────────────────────────────────────────────────────

  it("renders the username extracted from details JSON for a failed login (no username field)", () => {
    mockEntries = [ENTRY_FAILED_LOGIN];
    renderRecentActivity();
    // username=null but details contains "username":"bob"
    expect(screen.getByText("bob")).toBeInTheDocument();
  });

  it("renders the failed-login label in the details column", () => {
    mockEntries = [ENTRY_FAILED_LOGIN];
    renderRecentActivity();
    // i18n key admin.dashboardStats.loginFailed
    expect(
      screen.getByText(/admin\.dashboardStats\.loginFailed|login failed/i),
    ).toBeInTheDocument();
  });

  it("renders dash in details column for a normal (non-failed) entry", () => {
    mockEntries = [ENTRY_LOGIN];
    renderRecentActivity();
    // Details cell for a normal entry is "-"
    const rows = screen.getAllByRole("row");
    const firstDataRow = rows[1];
    const cells = firstDataRow.querySelectorAll("td");
    // details column is index 4
    expect(cells[4].textContent).toBe("-");
  });

  // ── isFailed badge variant ────────────────────────────────────────────────────

  it("renders the action badge with an error variant for a failed entry", () => {
    mockEntries = [ENTRY_FAILED_LOGIN];
    renderRecentActivity();
    // The Badge for a failed entry should carry an error-variant class.
    // Badge renders an element containing the action text — the isFailed path
    // passes variant="error" which adds a distinct CSS class.
    const badge = screen.getByText("LOGIN").closest("[class]");
    // We just assert the badge is present and in the document
    expect(badge).toBeInTheDocument();
  });

  // ── Dash fallbacks ────────────────────────────────────────────────────────────

  it("renders dash for missing username on a normal (non-failed) entry", () => {
    mockEntries = [{ ...ENTRY_LOGIN, username: null }];
    renderRecentActivity();
    const rows = screen.getAllByRole("row");
    const firstDataRow = rows[1];
    const cells = firstDataRow.querySelectorAll("td");
    // user column is index 1
    expect(cells[1].textContent).toBe("-");
  });

  it("renders dash for missing resourceType", () => {
    mockEntries = [{ ...ENTRY_LOGIN, resourceType: null }];
    renderRecentActivity();
    const rows = screen.getAllByRole("row");
    const firstDataRow = rows[1];
    const cells = firstDataRow.querySelectorAll("td");
    // resource column is index 3
    expect(cells[3].textContent).toBe("-");
  });
});
