import React from "react";
import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockGetDashboard = vi.fn();
const mockGetSystemInfo = vi.fn();
const mockFetch = vi.fn();

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useAdminSystemServiceGetDashboard: (...args: unknown[]) => mockGetDashboard(...args),
  useAdminSystemServiceGetSystemInfo: (...args: unknown[]) => mockGetSystemInfo(...args),
  // The trends section reads the sampled history; empty here, it has its own test.
  useActivityServiceListServerMetrics: () => ({ data: { metrics: [] }, isLoading: false, isFetching: false, isError: false }),
  // The finer metrics and the background-tasks panel read plain JSON routes.
  eveyFetch: (...args: unknown[]) => mockFetch(...args),
}));

// recharts renders real SVG that jsdom cannot measure.
vi.mock("recharts", () => {
  const Passthrough = ({ children }: { children?: React.ReactNode }) => React.createElement("div", null, children);
  return {
    LineChart: Passthrough, Line: () => null, XAxis: () => null, YAxis: () => null,
    Tooltip: () => null, Legend: () => null, ReferenceLine: () => null, CartesianGrid: () => null,
  };
});

const metrics = {
  load: { avg1: 1.5, avg5: 1.2, avg15: 0.9, known: true, cpus: 8, cpuBudget: 4 },
  memory: { totalBytes: 16e9, availableBytes: 8e9, budgetBytes: 0 },
  process: { rssBytes: 312 * 1024 * 1024, heapInUseBytes: 120 * 1024 * 1024, heapSysBytes: 200 * 1024 * 1024, goroutines: 143, gcCount: 57, gcPauseTotalNs: 1e6 },
  forks: {
    count: 2, rssBytes: 1.5 * 1024 * 1024 * 1024, cpuPercent: 250,
    byClass: [
      { class: "production", count: 1, rssBytes: 1024 * 1024 * 1024, cpuPercent: 200 },
      { class: "scan", count: 1, rssBytes: 512 * 1024 * 1024, cpuPercent: 50 },
    ],
  },
};

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
  iowaitPercent: 12.3,
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
    mockFetch.mockImplementation(({ url }: { url: string }) =>
      Promise.resolve(url.includes("/system/metrics") ? metrics : []),
    );
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

  it("reads the load against the CPU budget, Evey's own weight and the media processes by class", async () => {
    mockGetDashboard.mockReturnValue({ data: dashData, isLoading: false, dataUpdatedAt: Date.now() });
    mockGetSystemInfo.mockReturnValue({ data: sysData, isLoading: false });
    const SystemHealthPage = await getSystemHealthPage();
    render(React.createElement(SystemHealthPage), { wrapper: createWrapper() });
    const load = await screen.findByTestId("load-card");
    expect(load).toHaveTextContent("1.50 / 1.20 / 0.90");
    // The cgroup budget (4), not the host's 8 cores, is the reference.
    expect(load).toHaveTextContent("4 CPUs available");
    const proc = screen.getByTestId("process-card");
    expect(proc).toHaveTextContent("312.0 MB");
    expect(proc).toHaveTextContent("143 goroutines");
    const forks = screen.getByTestId("forks-card");
    expect(forks).toHaveTextContent("2 · 1.5 GB · 250% CPU");
    expect(forks).toHaveTextContent("production: 1 · 1.0 GB · 200% CPU");
    expect(forks).toHaveTextContent("scan: 1 · 512.0 MB · 50% CPU");
    // The activity counters (streams, users) left for the dashboard.
    expect(screen.queryByText(/Total Users/i)).not.toBeInTheDocument();
    expect(screen.getByTestId("background-tasks-panel")).toBeInTheDocument();
  });

  it("shows the share of CPU time waiting on disks under the CPU gauge", async () => {
    mockGetDashboard.mockReturnValue({ data: dashData, isLoading: false, dataUpdatedAt: Date.now() });
    mockGetSystemInfo.mockReturnValue({ data: sysData, isLoading: false });
    const SystemHealthPage = await getSystemHealthPage();
    render(React.createElement(SystemHealthPage), { wrapper: createWrapper() });
    expect(screen.getByText("of which 12.3% waiting on disks")).toBeInTheDocument();
  });

  it("lists the machine's disks when the server exposes them, and hides the section otherwise", async () => {
    mockGetSystemInfo.mockReturnValue({ data: sysData, isLoading: false });
    mockGetDashboard.mockReturnValue({ data: dashData, isLoading: false, dataUpdatedAt: Date.now() });
    const SystemHealthPage = await getSystemHealthPage();
    const { unmount } = render(React.createElement(SystemHealthPage), { wrapper: createWrapper() });
    expect(screen.queryByTestId("disk-io-list")).not.toBeInTheDocument();
    unmount();

    const withDisks = {
      ...dashData,
      idleDiskAwaitMs: 7.5,
      saturationAwaitMs: 20,
      disks: [
        { label: "QEMU HARDDISK drive-scsi1", device: "sda", rotational: true, awaitMs: 38.2, utilPercent: 61, readBps: "73400320", writeBps: "0", saturated: true },
      ],
    };
    mockGetDashboard.mockReturnValue({ data: withDisks, isLoading: false, dataUpdatedAt: Date.now() });
    render(React.createElement(SystemHealthPage), { wrapper: createWrapper() });
    expect(screen.getByText("Disks (whole machine)")).toBeInTheDocument();
    const list = screen.getByTestId("disk-io-list");
    expect(list).toHaveTextContent("38.2 ms");
    expect(list).toHaveTextContent("Saturated");
    expect(list).toHaveTextContent("At rest 7.5 ms · saturated above 20 ms");
  });

  it("hosts the trends section under the live cards", async () => {
    mockGetDashboard.mockReturnValue({ data: dashData, isLoading: false, dataUpdatedAt: Date.now() });
    mockGetSystemInfo.mockReturnValue({ data: sysData, isLoading: false });
    const SystemHealthPage = await getSystemHealthPage();
    render(React.createElement(SystemHealthPage), { wrapper: createWrapper() });
    expect(screen.getByText("Trends")).toBeInTheDocument();
    expect(screen.getByTestId("system-trends")).toBeInTheDocument();
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
