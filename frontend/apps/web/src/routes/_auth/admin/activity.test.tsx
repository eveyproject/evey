import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockGetPlayHistory = vi.fn();
const mockGetLoginHistory = vi.fn();
const mockGetServerStats = vi.fn();

vi.mock("@evey/api", () => ({
  useActivityServiceGetPlayHistory: (...args: unknown[]) => mockGetPlayHistory(...args),
  useActivityServiceGetLoginHistory: (...args: unknown[]) => mockGetLoginHistory(...args),
  useActivityServiceGetServerStats: (...args: unknown[]) => mockGetServerStats(...args),
}));

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (opts: { component: React.ComponentType }) => opts,
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

const mockLoginEntry = {
  id: "le-1",
  username: "alice",
  ipAddress: "192.168.1.10",
  success: true,
  userAgent: "Mozilla/5.0",
  timestamp: "2024-01-15T10:00:00Z",
};

describe("ActivityPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPlayHistory.mockReturnValue({ data: { entries: [], pagination: { totalItems: 0 } }, isLoading: false });
    mockGetLoginHistory.mockReturnValue({ data: { entries: [], pagination: { totalItems: 0 } }, isLoading: false });
    mockGetServerStats.mockReturnValue({ data: undefined, isLoading: false });
  });

  it("renders Activity heading", async () => {
    const ActivityPage = await getActivityPage();
    render(React.createElement(ActivityPage), { wrapper: createWrapper() });
    expect(screen.getByText("Activity")).toBeInTheDocument();
  });

  it("renders tab buttons", async () => {
    const ActivityPage = await getActivityPage();
    render(React.createElement(ActivityPage), { wrapper: createWrapper() });
    expect(screen.getByRole("button", { name: /Play History/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Login History/i })).toBeInTheDocument();
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

  it("switching to Login History tab shows login entries", async () => {
    mockGetLoginHistory.mockReturnValue({
      data: { entries: [mockLoginEntry], pagination: { totalItems: 1 } },
      isLoading: false,
    });
    const ActivityPage = await getActivityPage();
    render(React.createElement(ActivityPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /Login History/i }));
    expect(screen.getByText("alice")).toBeInTheDocument();
    expect(screen.getByText("192.168.1.10")).toBeInTheDocument();
  });

  it("login entry shows OK badge for successful login", async () => {
    mockGetLoginHistory.mockReturnValue({
      data: { entries: [mockLoginEntry], pagination: { totalItems: 1 } },
      isLoading: false,
    });
    const ActivityPage = await getActivityPage();
    render(React.createElement(ActivityPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /Login History/i }));
    expect(screen.getByText("OK")).toBeInTheDocument();
  });

  it("login entry shows Failed badge for failed login", async () => {
    const failedLogin = { ...mockLoginEntry, success: false };
    mockGetLoginHistory.mockReturnValue({
      data: { entries: [failedLogin], pagination: { totalItems: 1 } },
      isLoading: false,
    });
    const ActivityPage = await getActivityPage();
    render(React.createElement(ActivityPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /Login History/i }));
    expect(screen.getByText("Failed")).toBeInTheDocument();
  });

  it("switching to Server Stats tab renders stat cards", async () => {
    mockGetServerStats.mockReturnValue({
      data: { totalPlays: 100, totalUsers: 5, totalMediaItems: 500, totalBandwidthBytes: "1073741824" },
      isLoading: false,
    });
    const ActivityPage = await getActivityPage();
    render(React.createElement(ActivityPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /Server Stats/i }));
    expect(screen.getByText("Total Plays")).toBeInTheDocument();
    expect(screen.getByText("100")).toBeInTheDocument();
    expect(screen.getByText("Total Users")).toBeInTheDocument();
    expect(screen.getByText("5")).toBeInTheDocument();
  });
});
