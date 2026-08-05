import React from "react";
import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockGetDashboard = vi.fn();
const mockGetSystemInfo = vi.fn();

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
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
  streamingBps: "12500000", // 12.5 Mbps measured
  streamingBytesTotal: "5368709120", // 5 GB served since start
  activeSessionsBps: "25000000", // 25 Mbps theoretical
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

  it("renders the bandwidth card with measured rate, theoretical rate and total served", async () => {
    mockGetDashboard.mockReturnValue({ data: dashData, isLoading: false, dataUpdatedAt: Date.now() });
    mockGetSystemInfo.mockReturnValue({ data: sysData, isLoading: false });
    const SystemHealthPage = await getSystemHealthPage();
    render(React.createElement(SystemHealthPage), { wrapper: createWrapper() });
    const card = screen.getByTestId("bandwidth-card");
    expect(card).toBeInTheDocument();
    expect(card).toHaveTextContent("12.5 Mbps"); // measured
    expect(card).toHaveTextContent("25.0 Mbps"); // theoretical (active sessions)
    expect(card).toHaveTextContent(/5\.\d GB/); // total served
  });

  it("shows a dash for streaming rate on the first poll (streamingBps = -1)", async () => {
    const firstPoll = { ...dashData, streamingBps: "-1" };
    mockGetDashboard.mockReturnValue({ data: firstPoll, isLoading: false, dataUpdatedAt: Date.now() });
    mockGetSystemInfo.mockReturnValue({ data: sysData, isLoading: false });
    const SystemHealthPage = await getSystemHealthPage();
    render(React.createElement(SystemHealthPage), { wrapper: createWrapper() });
    expect(screen.getByTestId("bandwidth-card")).toHaveTextContent("-");
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

  it("lists every library path sharing one physical disk", async () => {
    const grouped = {
      ...dashData,
      storage: [
        {
          path: "/mnt/disk1/movies",
          paths: ["/mnt/disk1/movies", "/mnt/disk1/tv"],
          usedBytes: "107374182400",
          totalBytes: "214748364800",
          freeBytes: "107374182400",
        },
      ],
    };
    mockGetDashboard.mockReturnValue({ data: grouped, isLoading: false, dataUpdatedAt: Date.now() });
    mockGetSystemInfo.mockReturnValue({ data: sysData, isLoading: false });
    const SystemHealthPage = await getSystemHealthPage();
    render(React.createElement(SystemHealthPage), { wrapper: createWrapper() });
    // Both libraries appear under the single disk card - no duplicated capacity.
    expect(screen.getByText("/mnt/disk1/movies")).toBeInTheDocument();
    expect(screen.getByText("/mnt/disk1/tv")).toBeInTheDocument();
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

  it("does not render the video capacity card when videoSessions is undefined", async () => {
    mockGetDashboard.mockReturnValue({ data: dashData, isLoading: false, dataUpdatedAt: Date.now() });
    mockGetSystemInfo.mockReturnValue({ data: sysData, isLoading: false });
    const SystemHealthPage = await getSystemHealthPage();
    render(React.createElement(SystemHealthPage), { wrapper: createWrapper() });
    expect(screen.queryByTestId("video-capacity-card")).not.toBeInTheDocument();
  });

  it("renders the video capacity card with active/limit ratio", async () => {
    const sysWithVideo = {
      ...sysData,
      videoSessions: {
        autoCapacity: 3,
        effectiveLimit: 3,
        active: 1,
        evidence: { height: 1080, fps: 42.5, requiredFps: 24, cpuBudget: 4, derived: true },
      },
    };
    mockGetDashboard.mockReturnValue({ data: dashData, isLoading: false, dataUpdatedAt: Date.now() });
    mockGetSystemInfo.mockReturnValue({ data: sysWithVideo, isLoading: false });
    const SystemHealthPage = await getSystemHealthPage();
    render(React.createElement(SystemHealthPage), { wrapper: createWrapper() });
    const card = screen.getByTestId("video-capacity-card");
    expect(card).toHaveTextContent("1 / 3 streams");
    expect(card).toHaveTextContent("Measured: 1080p at 42.5 fps for 24.0 required, 4 cores");
  });

  it("shows the auto-capacity note when the admin has reduced the effective limit", async () => {
    const sysWithVideo = {
      ...sysData,
      videoSessions: {
        autoCapacity: 3,
        effectiveLimit: 1,
        active: 1,
        evidence: { height: 1080, fps: 42.5, requiredFps: 24, cpuBudget: 4, derived: true },
      },
    };
    mockGetDashboard.mockReturnValue({ data: dashData, isLoading: false, dataUpdatedAt: Date.now() });
    mockGetSystemInfo.mockReturnValue({ data: sysWithVideo, isLoading: false });
    const SystemHealthPage = await getSystemHealthPage();
    render(React.createElement(SystemHealthPage), { wrapper: createWrapper() });
    const card = screen.getByTestId("video-capacity-card");
    expect(card).toHaveTextContent("1 / 1 streams");
    expect(card).toHaveTextContent("Auto capacity is 3 streams (reduced by admin override)");
  });

  it("shows an unmeasured-capacity warning when evidence.derived is false", async () => {
    const sysWithVideo = {
      ...sysData,
      videoSessions: {
        autoCapacity: 1,
        effectiveLimit: 1,
        active: 0,
        evidence: { height: 0, fps: 0, requiredFps: 0, cpuBudget: 0, derived: false },
      },
    };
    mockGetDashboard.mockReturnValue({ data: dashData, isLoading: false, dataUpdatedAt: Date.now() });
    mockGetSystemInfo.mockReturnValue({ data: sysWithVideo, isLoading: false });
    const SystemHealthPage = await getSystemHealthPage();
    render(React.createElement(SystemHealthPage), { wrapper: createWrapper() });
    const card = screen.getByTestId("video-capacity-card");
    expect(card).toHaveTextContent("Capacity not measured (no benchmark available), conservative fallback to 1");
  });
});
