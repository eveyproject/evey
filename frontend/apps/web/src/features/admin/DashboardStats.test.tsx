import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { DashboardStats } from "./DashboardStats";

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to, ...rest }: { children: React.ReactNode; to: string; [k: string]: unknown }) => (
    <a href={to} {...rest}>{children}</a>
  ),
}));

vi.mock("@tanstack/react-query", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-query")>();
  return {
    ...actual,
    useQueryClient: () => ({ invalidateQueries: vi.fn() }),
  };
});

// Mutable so a test can make the count query FAIL. A module-level literal
// could only ever describe the happy path, which is how "0 movies" came to be
// indistinguishable from "I could not count".
const counts = vi.hoisted(() => ({
  movies: { data: { pagination: { totalItems: 42 } } } as Record<string, unknown>,
  series: { data: { pagination: { totalItems: 7 } } } as Record<string, unknown>,
  online: { data: { online: [] }, isLoading: false } as Record<string, unknown>,
}));

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  // The background-tasks panel polls its own JSON route.
  eveyFetch: () => Promise.resolve([]),
  useLibraryServiceListMediaItems: (params: { type?: string }) => {
    if (params?.type === "MEDIA_ITEM_TYPE_MOVIE") {
      return counts.movies;
    }
    if (params?.type === "MEDIA_ITEM_TYPE_SERIES") {
      return counts.series;
    }
    return { data: undefined };
  },
  // The OnlineNow presence widget and the restart button mount inside the
  // dashboard tree.
  usePresenceServiceListOnline: () => counts.online,
  useAdminSystemServiceRestartServer: () => ({ mutate: () => {}, isPending: false }),
  useAdminSessionServiceListActiveSessions: () => ({ data: { sessions: [] }, isLoading: false }),
  useAdminSessionServiceKillSession: () => ({ mutate: vi.fn(), isPending: false }),
  useAdminSessionServiceSendSessionCommand: () => ({ mutate: vi.fn(), isPending: false }),
  getAdminSessionServiceListActiveSessionsQueryKey: () => ["/api/v1/admin/sessions"],
  useLibraryServiceListLibraries: () => ({ data: { libraries: [] } }),
  useLibraryServiceScanLibrary: () => ({ mutateAsync: vi.fn() }),
  useActivityServiceGetServerStats: () => ({ data: { popularMedia: [] } }),
  useActivityServiceGetAnalytics: () => ({ data: { dailyPlays: [], hourlyPlays: [] } }),
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
  useAuthStore: (fn: (s: { accessToken: string }) => unknown) => fn({ accessToken: "test-token" }),
  useDemoStore: (fn: (s: { demoLocked: string[] }) => unknown) => fn({ demoLocked: [] }),
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
}

const baseInfo = {
  totalUsers: 5,
  activeStreams: 2,
  cpuPercent: 34.5,
  memoryPercent: 61.2,
  activeTranscodes: 1,
  totalMediaItems: 120,
  streamingBps: "8000000", // 8 Mbps measured (real served rate)
  activeSessionsBps: "42600000", // 42.6 Mbps theoretical (sum of source bitrates)
  storage: [],
};

describe("DashboardStats", () => {
  it("renders the live tiles and folds the inventory into one line", () => {
    render(<DashboardStats info={baseInfo as never} />, { wrapper: createWrapper() });
    // Four live tiles: streams (with the transcode count as a sub-line),
    // throughput (with the theoretical rate), CPU, memory.
    expect(screen.getByText("2")).toBeInTheDocument();
    expect(screen.getByText("1 transcoded")).toBeInTheDocument();
    expect(screen.getByText("34.5%")).toBeInTheDocument();
    expect(screen.getByText("61.2%")).toBeInTheDocument();
    // Measured throughput (real served rate) and theoretical rate (sum of
    // active source bitrates) - for a transcode the two diverge, which is the
    // point: measured is the truth, theoretical is the source-rate upper bound.
    expect(screen.getByText("8.0 Mbps")).toBeInTheDocument();
    expect(screen.getByText("theoretical 42.6 Mbps")).toBeInTheDocument();
    // Movies, series, media and users are inventory, not activity: one line,
    // not four tiles.
    expect(screen.getByTestId("dashboard-inventory")).toHaveTextContent("42 movies · 7 series · 120 media items · 5 users");
  });

  it("hosts the background-tasks panel and the quick actions side by side", () => {
    render(<DashboardStats info={baseInfo as never} />, { wrapper: createWrapper() });
    expect(screen.getByTestId("background-tasks-panel")).toBeInTheDocument();
    expect(screen.getByText("Quick Actions")).toBeInTheDocument();
  });

  it("shows an em dash for measured throughput before the first sample", () => {
    render(<DashboardStats info={{ ...baseInfo, streamingBps: "-1" } as never} />, { wrapper: createWrapper() });
    expect(screen.getByText("-")).toBeInTheDocument();
  });

  it("does not report zero movies when the count could not be read", () => {
    counts.movies = { data: undefined, isError: true };
    try {
      render(<DashboardStats info={baseInfo as never} />, { wrapper: createWrapper() });
      // "0 movies" is what an admin sees after a wiped library. A failed count
      // must not be able to say that.
      expect(screen.getByTestId("dashboard-inventory")).toHaveTextContent("- movies · 7 series");
    } finally {
      counts.movies = { data: { pagination: { totalItems: 42 } } };
    }
  });

  it("does not report an empty server when presence could not be read", () => {
    counts.online = { data: undefined, isError: true, error: new Error("down"), refetch: vi.fn() };
    try {
      render(<DashboardStats info={baseInfo as never} />, { wrapper: createWrapper() });
      // An admin watching for suspicious connections must not read a failed
      // presence call as "nobody is connected".
      expect(screen.getByTestId("fetch-error-state")).toBeInTheDocument();
    } finally {
      counts.online = { data: { online: [] }, isLoading: false };
    }
  });

  it("adds a disks tile only when the machine exposes its disks", () => {
    const { unmount } = render(<DashboardStats info={baseInfo as never} />, { wrapper: createWrapper() });
    expect(screen.queryByTestId("disks-stat-card")).not.toBeInTheDocument();
    unmount();
    const withDisks = {
      ...baseInfo,
      disks: [
        { label: "QEMU HARDDISK drive-scsi1", device: "sda", awaitMs: 6.1, utilPercent: 100, saturated: false },
        { label: "QEMU HARDDISK drive-scsi2", device: "sdc", awaitMs: 38.2, utilPercent: 61, saturated: true },
      ],
    };
    render(<DashboardStats info={withDisks as never} />, { wrapper: createWrapper() });
    // The disk that waits, not the one that is busiest, and its verdict.
    expect(screen.getByTestId("disks-stat-card")).toHaveTextContent("38.2 ms");
    expect(screen.getByTestId("disks-stat-card")).toHaveTextContent("Saturated · QEMU HARDDISK drive-scsi2");
  });

  it("leaves storage to the System Health page", () => {
    const infoWithStorage = {
      ...baseInfo,
      storage: [
        { path: "/mnt/media", usedBytes: "500000000", totalBytes: "1000000000" },
      ],
    };
    render(<DashboardStats info={infoWithStorage as never} />, { wrapper: createWrapper() });
    expect(screen.queryByText("/mnt/media")).not.toBeInTheDocument();
  });
});
