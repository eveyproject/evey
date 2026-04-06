import React from "react";
import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockGetDashboard = vi.fn();
const mockGetSystemInfo = vi.fn();

vi.mock("@evey/api", () => ({
  useAdminSystemServiceGetDashboard: (...args: unknown[]) => mockGetDashboard(...args),
  useAdminSystemServiceGetSystemInfo: (...args: unknown[]) => mockGetSystemInfo(...args),
}));

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (opts: { component: React.ComponentType }) => opts,
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

async function getSystemHealthPage() {
  const mod = await import("./system-health.lazy");
  return (mod.Route as unknown as { component: React.ComponentType }).component;
}

const dashData = {
  cpuPercent: 45,
  memoryPercent: 60,
  activeStreams: 2,
  activeTranscodes: 1,
  totalMediaItems: 500,
  totalUsers: 5,
  storage: [
    { path: "/data/media", usedBytes: "107374182400", totalBytes: "214748364800", freeBytes: "107374182400" },
  ],
};

const sysData = {
  version: "1.2.3",
  os: "linux",
  arch: "amd64",
  goVersion: "go1.22",
  ffmpegVersion: "6.0",
  uptimeSeconds: "3661",
  serverId: "srv-abc",
  hwDevices: [],
};

describe("SystemHealthPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetDashboard.mockReturnValue({ data: null, isLoading: false, dataUpdatedAt: 0 });
    mockGetSystemInfo.mockReturnValue({ data: null, isLoading: false });
  });

  it("shows page heading", async () => {
    const SystemHealthPage = await getSystemHealthPage();
    render(React.createElement(SystemHealthPage), { wrapper: createWrapper() });
    expect(screen.getByText("System Health")).toBeInTheDocument();
  });

  it("renders CPU and memory metric cards", async () => {
    mockGetDashboard.mockReturnValue({ data: dashData, isLoading: false, dataUpdatedAt: Date.now() });
    mockGetSystemInfo.mockReturnValue({ data: sysData, isLoading: false });
    const SystemHealthPage = await getSystemHealthPage();
    render(React.createElement(SystemHealthPage), { wrapper: createWrapper() });
    expect(screen.getByText(/CPU Usage/i)).toBeInTheDocument();
    expect(screen.getByText(/Memory Usage/i)).toBeInTheDocument();
  });

  it("renders dashboard activity badges", async () => {
    mockGetDashboard.mockReturnValue({ data: dashData, isLoading: false, dataUpdatedAt: Date.now() });
    mockGetSystemInfo.mockReturnValue({ data: sysData, isLoading: false });
    const SystemHealthPage = await getSystemHealthPage();
    render(React.createElement(SystemHealthPage), { wrapper: createWrapper() });
    expect(screen.getByText(/Active Streams/i)).toBeInTheDocument();
    expect(screen.getByText(/Total Users/i)).toBeInTheDocument();
  });

  it("renders server info panel with version", async () => {
    mockGetDashboard.mockReturnValue({ data: dashData, isLoading: false, dataUpdatedAt: Date.now() });
    mockGetSystemInfo.mockReturnValue({ data: sysData, isLoading: false });
    const SystemHealthPage = await getSystemHealthPage();
    render(React.createElement(SystemHealthPage), { wrapper: createWrapper() });
    expect(screen.getByText("1.2.3")).toBeInTheDocument();
    expect(screen.getByText("Server Info")).toBeInTheDocument();
  });

  it("renders storage section with path", async () => {
    mockGetDashboard.mockReturnValue({ data: dashData, isLoading: false, dataUpdatedAt: Date.now() });
    mockGetSystemInfo.mockReturnValue({ data: sysData, isLoading: false });
    const SystemHealthPage = await getSystemHealthPage();
    render(React.createElement(SystemHealthPage), { wrapper: createWrapper() });
    expect(screen.getByText("/data/media")).toBeInTheDocument();
  });

  it("shows warning color style when usage >70%", async () => {
    const highUsage = { ...dashData, cpuPercent: 75, memoryPercent: 75 };
    mockGetDashboard.mockReturnValue({ data: highUsage, isLoading: false, dataUpdatedAt: Date.now() });
    mockGetSystemInfo.mockReturnValue({ data: sysData, isLoading: false });
    const SystemHealthPage = await getSystemHealthPage();
    const { container } = render(React.createElement(SystemHealthPage), { wrapper: createWrapper() });
    expect(container.innerHTML).toContain("color-warning");
  });

  it("shows danger color style when usage >85%", async () => {
    const criticalUsage = { ...dashData, cpuPercent: 90, memoryPercent: 90 };
    mockGetDashboard.mockReturnValue({ data: criticalUsage, isLoading: false, dataUpdatedAt: Date.now() });
    mockGetSystemInfo.mockReturnValue({ data: sysData, isLoading: false });
    const SystemHealthPage = await getSystemHealthPage();
    const { container } = render(React.createElement(SystemHealthPage), { wrapper: createWrapper() });
    expect(container.innerHTML).toContain("color-danger");
  });
});
