import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockGetPlayHistory = vi.fn();
const mockGetServerStats = vi.fn();
const mockGetAuditLog = vi.fn();

vi.mock("@evey/api", () => ({
  useActivityServiceGetPlayHistory: (...args: unknown[]) => mockGetPlayHistory(...args),
  useActivityServiceGetServerStats: (...args: unknown[]) => mockGetServerStats(...args),
  useAdminSessionServiceGetAuditLog: (...args: unknown[]) => mockGetAuditLog(...args),
  eveyFetch: vi.fn(),
}));

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (opts: { component: React.ComponentType }) => opts,
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
  mediaTitle: "Breaking Bad S01E01",
  deviceName: "Chrome",
  positionMs: "1800000",
  durationMs: "3600000",
  startedAt: "2024-01-15T10:00:00Z",
};

describe("ActivityPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlayHistory.mockReturnValue({ data: { entries: [], pagination: { totalItems: 0 } }, isLoading: false });
    mockGetServerStats.mockReturnValue({ data: undefined, isLoading: false });
    mockGetAuditLog.mockReturnValue({ data: { entries: [], total: 0 }, isLoading: false });
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
    expect(screen.getByText(/No play history/i)).toBeInTheDocument();
  });

  it("renders play history entries", async () => {
    mockGetPlayHistory.mockReturnValue({
      data: { entries: [mockPlayEntry], pagination: { totalItems: 1 } },
      isLoading: false,
    });
    const ActivityPage = await getActivityPage();
    render(React.createElement(ActivityPage), { wrapper: createWrapper() });
    expect(screen.getByText("Breaking Bad S01E01")).toBeInTheDocument();
    expect(screen.getByText("Chrome")).toBeInTheDocument();
  });

  it("switching to Audit tab shows audit table", async () => {
    const ActivityPage = await getActivityPage();
    render(React.createElement(ActivityPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /Audit Log/i }));
    expect(screen.getByTestId("audit-table")).toBeInTheDocument();
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
    expect(screen.getByText("2h 0m")).toBeInTheDocument();
    expect(screen.getByText(/Library Size/i)).toBeInTheDocument();
    expect(screen.getByText("2.1 GB")).toBeInTheDocument();
  });

  it("Server Stats tab shows popular media with viewer counts and active users", async () => {
    mockGetServerStats.mockReturnValue({
      data: {
        totalPlays: 10,
        totalUsers: 2,
        totalMediaItems: 50,
        popularMedia: [{ mediaItemId: "mi-1", title: "Dune", playCount: 7, uniqueViewers: 3 }],
        activeUsers: [{ userId: "usr-1", username: "alice", playCount: 5, totalWatchTimeMs: "3600000" }],
      },
      isLoading: false,
    });
    const ActivityPage = await getActivityPage();
    render(React.createElement(ActivityPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /Server Stats/i }));
    expect(screen.getByText("Dune")).toBeInTheDocument();
    expect(screen.getByText(/3 viewer/i)).toBeInTheDocument();
    expect(screen.getByText("alice")).toBeInTheDocument();
  });

  it("renders export JSON button", async () => {
    const ActivityPage = await getActivityPage();
    render(React.createElement(ActivityPage), { wrapper: createWrapper() });
    expect(screen.getByText("JSON")).toBeInTheDocument();
  });
});
