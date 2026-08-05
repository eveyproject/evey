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

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useLibraryServiceListMediaItems: (params: { type?: string }) => {
    if (params?.type === "MEDIA_ITEM_TYPE_MOVIE") {
      return { data: { pagination: { totalItems: 42 } } };
    }
    if (params?.type === "MEDIA_ITEM_TYPE_SERIES") {
      return { data: { pagination: { totalItems: 7 } } };
    }
    return { data: undefined };
  },
  useAdminSessionServiceGetAuditLog: () => ({ data: { entries: [] }, isLoading: false }),
  // The OnlineNow presence widget and the restart button mount inside the
  // dashboard tree.
  usePresenceServiceListOnline: () => ({ data: { online: [] }, isLoading: false }),
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
  it("renders stat cards with correct values", () => {
    render(<DashboardStats info={baseInfo as never} />, { wrapper: createWrapper() });
    expect(screen.getByText("42")).toBeInTheDocument();
    expect(screen.getByText("7")).toBeInTheDocument();
    expect(screen.getByText("5")).toBeInTheDocument();
    expect(screen.getByText("2")).toBeInTheDocument();
    expect(screen.getByText("34.5%")).toBeInTheDocument();
    expect(screen.getByText("61.2%")).toBeInTheDocument();
    expect(screen.getByText("1")).toBeInTheDocument();
    expect(screen.getByText("120")).toBeInTheDocument();
    // Measured throughput (real served rate) and theoretical rate (sum of
    // active source bitrates) - for a transcode the two diverge, which is the
    // point: measured is the truth, theoretical is the source-rate upper bound.
    expect(screen.getByText("8.0 Mbps")).toBeInTheDocument();
    expect(screen.getByText("42.6 Mbps")).toBeInTheDocument();
  });

  it("shows an em dash for measured throughput before the first sample", () => {
    render(<DashboardStats info={{ ...baseInfo, streamingBps: "-1" } as never} />, { wrapper: createWrapper() });
    expect(screen.getByText("-")).toBeInTheDocument();
  });

  it("renders storage section when storage data present", () => {
    const infoWithStorage = {
      ...baseInfo,
      storage: [
        { path: "/mnt/media", usedBytes: "500000000", totalBytes: "1000000000" },
      ],
    };
    render(<DashboardStats info={infoWithStorage as never} />, { wrapper: createWrapper() });
    expect(screen.getByText("Storage")).toBeInTheDocument();
    expect(screen.getByText("/mnt/media")).toBeInTheDocument();
  });
});
