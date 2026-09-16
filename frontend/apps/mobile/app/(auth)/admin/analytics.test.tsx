import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

// The three counters, the top-media bars scaled by WATCH TIME (a title
// replayed briefly many times must not outrank one watched once for hours),
// the recent activity rows, empty notices per section, and a retry when the
// server did not answer instead of a dead failure block.
vi.mock("react-native", () => ({
  View: ({ children, style }: any) => {
    const flat = Array.isArray(style) ? Object.assign({}, ...style.filter(Boolean)) : style;
    return React.createElement("div", { "data-width": flat?.width }, children);
  },
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  RefreshControl: () => null,
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: ({ name }: any) => React.createElement("i", { "data-icon": name }) }));
vi.mock("expo-router", () => ({ Stack: { Screen: () => null } }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
const q = vi.hoisted(() => ({ data: undefined as unknown, isLoading: false, refetch: vi.fn(), opts: null as any }));
vi.mock("@evey/api", () => ({
  useActivityServiceGetAnalytics: (o: any) => {
    q.opts = o.query;
    return q;
  },
  formatDurationSec: (s?: string) => (s && s !== "0" ? `${Math.round(Number(s) / 60)}min` : ""),
}));
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", surface: "#111", text: "#fff", textMuted: "#888", accent: "#f00", border: "#333" }, radii: { md: 8 } }),
}));
vi.mock("../../../src/components/LoadFailed", () => ({
  LoadFailed: ({ onRetry, message }: any) => React.createElement("button", { onClick: onRetry }, message),
}));

import AdminAnalyticsScreen from "./analytics";

beforeEach(() => {
  vi.clearAllMocks();
  q.data = undefined;
  q.isLoading = false;
});

describe("admin analytics screen (mobile)", () => {
  it("shows the counters, bars by watch time and the recent rows, polling once a minute", () => {
    q.data = {
      totalPlaysToday: 3, totalPlaysThisWeek: 21, activeUsersToday: 2,
      topMedia: [
        { mediaItemId: "a", title: "Long film", playCount: 1, totalDurationSeconds: "7200" },
        { mediaItemId: "b", title: "Short clip", playCount: 9, totalDurationSeconds: "1800" },
        { mediaItemId: "c", playCount: 0 },
      ],
      recentActivity: [{ userId: "u1", username: "alice", mediaTitle: "Long film", playedAt: "2026-09-16T04:00:00Z" }],
    };
    render(<AdminAnalyticsScreen />);
    expect(q.opts.refetchInterval).toBe(60_000);
    expect(q.opts.refetchIntervalInBackground).toBe(false);
    for (const n of ["3", "21", "2"]) expect(screen.getByText(n)).toBeTruthy();

    const widths = Array.from(document.querySelectorAll("[data-width]")).map((el) => el.getAttribute("data-width")).filter((w) => w && w.endsWith("%"));
    expect(widths).toEqual(["100%", "25%", "0%"]);
    expect(screen.getByText("120min")).toBeTruthy();
    expect(screen.getByText("0min")).toBeTruthy();
    expect(screen.getByText("9")).toBeTruthy();
    expect(screen.getByText("alice")).toBeTruthy();
    expect(screen.getByText(new Date("2026-09-16T04:00:00Z").toLocaleString())).toBeTruthy();
    expect(screen.queryByText("admin.analytics.noPlays")).toBeNull();
  });

  it("says when a section is empty", () => {
    q.data = { totalPlaysToday: 0, topMedia: [], recentActivity: [] };
    render(<AdminAnalyticsScreen />);
    expect(screen.getByText("admin.analytics.noPlays")).toBeTruthy();
    expect(screen.getByText("admin.analytics.noActivity")).toBeTruthy();
  });

  it("offers a retry when the server did not answer, and an hourglass while loading", () => {
    const failed = render(<AdminAnalyticsScreen />);
    fireEvent.click(screen.getByText("admin.analytics.failedLoad"));
    expect(q.refetch).toHaveBeenCalledTimes(1);
    failed.unmount();

    q.isLoading = true;
    render(<AdminAnalyticsScreen />);
    expect(document.querySelector('[data-icon="hourglass-outline"]')).toBeTruthy();
    expect(screen.queryByText("admin.analytics.failedLoad")).toBeNull();
  });
});
