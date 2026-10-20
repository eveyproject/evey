import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// The page's paginated tabs run real useInfiniteQuery over the RAW fetchers
// (infinite-scroll migration); only server stats still goes through the
// generated hook. Mock at the layer the page actually calls.
const mockGetPlayHistory = vi.fn();
const mockGetServerStats = vi.fn();
const mockGetAuditLog = vi.fn();

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  activityServiceGetPlayHistory: (...args: unknown[]) => mockGetPlayHistory(...args),
  adminSessionServiceGetAuditLog: (...args: unknown[]) => mockGetAuditLog(...args),
  activityServiceGetServerStats: vi.fn(),
  useActivityServiceGetServerStats: (...args: unknown[]) => mockGetServerStats(...args),
  // Resolves to an empty payload: a bare vi.fn() hands react-query
  // `undefined`, which it rejects with a warning on every render.
  eveyFetch: vi.fn().mockResolvedValue({}),
}));

// The page reads its tab + watcher filter from the URL and writes them back
// with navigate(), so the router mock has to serve both halves.
let mockSearch: { tab?: string; userId?: string; profileId?: string } = {};
const mockNavigate = vi.fn((to: { search?: typeof mockSearch }) => {
  mockSearch = { ...mockSearch, ...to.search };
});

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (opts: { component: React.ComponentType }) => ({
    ...opts,
    useSearch: () => mockSearch,
  }),
  useNavigate: () => mockNavigate,
}));

// The watcher <select> lists every account and profile; it has its own tests.
vi.mock("@/features/admin/WatcherFilter", () => ({
  WatcherFilter: () => React.createElement("div", { "data-testid": "watcher-filter" }),
}));

vi.mock("@/features/admin/AuditLogTable", () => ({
  AuditLogTable: () => React.createElement("div", { "data-testid": "audit-table" }, "AuditTable"),
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

async function getActivityPage() {
  const mod = await import("./activity.lazy");
  return (mod.Route as unknown as { component: React.ComponentType }).component;
}

const mockPlayEntry = {
  id: "pe-1",
  mediaTitle: "Some Series · S01E01 · Pilot",
  deviceName: "Chrome",
  positionMs: "1800000",
  durationMs: "3600000",
  startedAt: "2024-01-15T10:00:00Z",
};

describe("ActivityPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockSearch = {};
    mockGetPlayHistory.mockResolvedValue({ entries: [], pagination: { totalItems: 0 } });
    mockGetServerStats.mockReturnValue({ data: undefined, isLoading: false });
    mockGetAuditLog.mockResolvedValue({ entries: [], total: 0 });
  });

  it("renders Activity heading", async () => {
    const ActivityPage = await getActivityPage();
    render(React.createElement(ActivityPage), { wrapper: createWrapper() });
    expect(screen.getByText("Activity")).toBeInTheDocument();
  });

  it("renders tab buttons (Play History, Audit Log, Server Stats)", async () => {
    const ActivityPage = await getActivityPage();
    render(React.createElement(ActivityPage), { wrapper: createWrapper() });
    expect(screen.getByRole("button", { name: /Play History/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Audit Log/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Server Stats/i })).toBeInTheDocument();
  });

  it("shows Play History by default with empty state", async () => {
    const ActivityPage = await getActivityPage();
    render(React.createElement(ActivityPage), { wrapper: createWrapper() });
    // The infinite query resolves asynchronously - wait for the fetch to land.
    expect(await screen.findByText(/No play history/i)).toBeInTheDocument();
  });

  it("renders play history entries", async () => {
    mockGetPlayHistory.mockResolvedValue({ entries: [mockPlayEntry], pagination: { totalItems: 1 } });
    const ActivityPage = await getActivityPage();
    render(React.createElement(ActivityPage), { wrapper: createWrapper() });
    expect(await screen.findByText("Some Series · S01E01 · Pilot")).toBeInTheDocument();
    expect(screen.getByText("Chrome")).toBeInTheDocument();
  });

  it("switching to Audit tab shows audit table", async () => {
    const ActivityPage = await getActivityPage();
    render(React.createElement(ActivityPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /Audit Log/i }));
    expect(await screen.findByTestId("audit-table")).toBeInTheDocument();
  });

  it("switching to Server Stats tab renders all stat cards including watch time and storage", async () => {
    mockGetServerStats.mockReturnValue({
      data: {
        totalPlays: 100,
        totalUsers: 5,
        totalMediaItems: 500,
        totalPlayTimeMs: "7200000", // 2h
        totalBandwidthBytes: "1073741824", // 1 GB
        totalStorageBytes: "2147483648", // 2 GB
      },
      isLoading: false,
    });
    const ActivityPage = await getActivityPage();
    render(React.createElement(ActivityPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /Server Stats/i }));
    expect(screen.getByText("Total Plays")).toBeInTheDocument();
    expect(screen.getByText("100")).toBeInTheDocument();
    // Cards added with the real-stats feature.
    expect(screen.getByText(/Total Watch Time/i)).toBeInTheDocument();
    expect(screen.getByText("2h")).toBeInTheDocument();
    expect(screen.getByText(/Library Size/i)).toBeInTheDocument();
    expect(screen.getByText("2.0 GB")).toBeInTheDocument();
  });

  it("Server Stats tab shows popular media with viewer counts and active users", async () => {
    mockGetServerStats.mockReturnValue({
      data: {
        totalPlays: 10,
        totalUsers: 2,
        totalMediaItems: 50,
        popularMedia: [{ mediaItemId: "mi-1", title: "Some Movie", playCount: 7, uniqueViewers: 3 }],
        activeUsers: [{ userId: "usr-1", username: "alice", playCount: 5, totalWatchTimeMs: "3600000" }],
      },
      isLoading: false,
    });
    const ActivityPage = await getActivityPage();
    render(React.createElement(ActivityPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /Server Stats/i }));
    expect(screen.getByText("Some Movie")).toBeInTheDocument();
    expect(screen.getByText(/3 viewer/i)).toBeInTheDocument();
    expect(screen.getByText("alice")).toBeInTheDocument();
  });

  it("renders export JSON button", async () => {
    const ActivityPage = await getActivityPage();
    render(React.createElement(ActivityPage), { wrapper: createWrapper() });
    expect(screen.getByText("JSON")).toBeInTheDocument();
  });
});
