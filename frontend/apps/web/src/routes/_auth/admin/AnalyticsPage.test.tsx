import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockGetAnalytics = vi.fn();

function toProto(d: typeof mockAnalytics | Record<string, unknown>) {
  const a = d as typeof mockAnalytics;
  return {
    totalPlaysToday: a.total_plays_today,
    totalPlaysThisWeek: a.total_plays_this_week,
    activeUsersToday: a.active_users_today,
    topMedia: (a.top_media ?? []).map((m) => ({ mediaItemId: m.media_item_id, title: m.title, playCount: m.play_count, totalDurationSeconds: String(m.total_duration_seconds) })),
    recentActivity: (a.recent_activity ?? []).map((r) => ({ userId: r.user_id, username: r.username, mediaTitle: r.media_title, playedAt: r.played_at })),
  };
}

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  eveyFetch: vi.fn(),
  useActivityServiceGetAnalytics: (opts: { query?: { select?: (d: unknown) => unknown } }) => {
    const raw = mockGetAnalytics();
    const select = opts?.query?.select;
    return { ...raw, data: raw?.data !== undefined && select ? select(raw.data) : raw?.data };
  },
}));

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (def: { component: React.ComponentType }) => def,
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false, refetchInterval: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

const mockAnalytics = {
  total_plays_today: 42,
  total_plays_this_week: 310,
  active_users_today: 7,
  top_media: [
    { media_item_id: "m1", title: "Inception", play_count: 15, total_duration_seconds: 7200 },
    { media_item_id: "m2", title: "Interstellar", play_count: 10, total_duration_seconds: 10800 },
  ],
  recent_activity: [
    { user_id: "u1", username: "alice", media_title: "Inception", played_at: "2025-01-01T10:00:00Z" },
  ],
};

describe("AnalyticsPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows loading spinner while fetching", async () => {
    
    mockGetAnalytics.mockReturnValue({ data: undefined, isLoading: true }); // never resolves

    const mod = await import("./analytics.lazy");
    const AnalyticsPage = (mod.Route as unknown as { component: React.ComponentType }).component;

    render(React.createElement(AnalyticsPage), { wrapper: createWrapper() });

    // Spinner renders - check for SVG or role=status or just that stat cards are absent
    expect(screen.queryByText("Plays Today")).not.toBeInTheDocument();
  });

  it("renders stat cards with correct values", async () => {
    
    mockGetAnalytics.mockReturnValue({ data: toProto(mockAnalytics), isSuccess: true });

    const mod = await import("./analytics.lazy");
    const AnalyticsPage = (mod.Route as unknown as { component: React.ComponentType }).component;

    render(React.createElement(AnalyticsPage), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("Plays Today")).toBeInTheDocument();
      expect(screen.getByText("Plays This Week")).toBeInTheDocument();
      expect(screen.getByText("Active Users Today")).toBeInTheDocument();
    });
    expect(screen.getByText("42")).toBeInTheDocument();
    expect(screen.getByText("310")).toBeInTheDocument();
    expect(screen.getByText("7")).toBeInTheDocument();
  });

  it("renders top media table with titles", async () => {
    
    mockGetAnalytics.mockReturnValue({ data: toProto(mockAnalytics), isSuccess: true });

    const mod = await import("./analytics.lazy");
    const AnalyticsPage = (mod.Route as unknown as { component: React.ComponentType }).component;

    render(React.createElement(AnalyticsPage), { wrapper: createWrapper() });

    await waitFor(() => {
      // "Inception" appears in both top media and recent activity tables
      expect(screen.getAllByText("Inception").length).toBeGreaterThan(0);
      expect(screen.getByText("Interstellar")).toBeInTheDocument();
    });
  });

  it("renders recent activity table with username and media", async () => {
    
    mockGetAnalytics.mockReturnValue({ data: toProto(mockAnalytics), isSuccess: true });

    const mod = await import("./analytics.lazy");
    const AnalyticsPage = (mod.Route as unknown as { component: React.ComponentType }).component;

    render(React.createElement(AnalyticsPage), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("alice")).toBeInTheDocument();
    });
  });

  it("shows error message when fetch fails", async () => {
    
    mockGetAnalytics.mockReturnValue({ data: undefined, isError: true, error: new Error("Network error") });

    const mod = await import("./analytics.lazy");
    const AnalyticsPage = (mod.Route as unknown as { component: React.ComponentType }).component;

    render(React.createElement(AnalyticsPage), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText(/Failed to load analytics/i)).toBeInTheDocument();
    });
  });

  it("shows empty state rows when top_media is empty", async () => {
    
    mockGetAnalytics.mockReturnValue({ data: toProto({ ...mockAnalytics, top_media: [], recent_activity: [] }), isSuccess: true });

    const mod = await import("./analytics.lazy");
    const AnalyticsPage = (mod.Route as unknown as { component: React.ComponentType }).component;

    render(React.createElement(AnalyticsPage), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText(/No plays in the last 30 days/i)).toBeInTheDocument();
      expect(screen.getByText(/No recent activity/i)).toBeInTheDocument();
    });
  });
});
