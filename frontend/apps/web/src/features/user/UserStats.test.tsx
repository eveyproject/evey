import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import React from "react";
import { UserStats } from "./UserStats";

// recharts mocked (its real SVG trips jsdom); this checks the component wires
// the chart lib in and renders its chrome (stat cards, titles, genre legend,
// most-played list) without throwing after the gifted-charts -> recharts swap.
vi.mock("recharts", () => {
  const Passthrough = ({ children }: { children?: React.ReactNode }) => React.createElement("div", null, children);
  return { BarChart: Passthrough, Bar: () => null, XAxis: () => null, YAxis: () => null, PieChart: Passthrough, Pie: Passthrough, Cell: () => null };
});

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key, i18n: { language: "en" } }),
}));
vi.mock("@/hooks/useCssVar", () => ({ useCssVar: (_v: string, fallback?: string) => fallback ?? "#000" }));
vi.mock("@tanstack/react-router", () => ({
  Link: ({ children }: { children?: React.ReactNode }) => React.createElement("a", null, children),
}));
vi.mock("@evey/store", () => ({ useAuthStore: (sel: (s: unknown) => unknown) => sel({ profileId: "p1" }) }));
vi.mock("lucide-react", () => ({ BarChart3: () => React.createElement("span") }));
vi.mock("@/components/ui/Spinner", () => ({ Spinner: () => React.createElement("div") }));
vi.mock("@/components/ui/EmptyState", () => ({ EmptyState: ({ title }: { title: string }) => React.createElement("div", null, title) }));
vi.mock("@/components/ui/FetchErrorState", () => ({ FetchErrorState: () => React.createElement("div") }));

const stats = {
  totalPlays: 42,
  totalWatchTimeMs: `${5 * 3600_000}`,
  uniqueItemsWatched: 10,
  genres: [{ genre: "Drama", totalTimeMs: `${3 * 3600_000}`, percent: 60 }],
  byMonth: [{ year: 2026, month: 8, totalTimeMs: `${2 * 3600_000}` }],
  mostPlayed: [{ mediaItemId: "m1", title: "The Film", playCount: 3, totalTimeMs: `${3600_000}` }],
};
vi.mock("@evey/api", () => ({
  useActivityServiceGetUserStats: () => ({ data: stats, isLoading: false, isError: false, refetch: vi.fn() }),
}));

describe("UserStats (recharts)", () => {
  it("renders stat cards, chart titles, the genre legend and most-played list", () => {
    render(<UserStats />);
    expect(screen.getByText("common.userStats.genreBreakdown")).toBeTruthy();
    expect(screen.getByText("common.userStats.monthlyActivity")).toBeTruthy();
    expect(screen.getByText("common.userStats.mostPlayed")).toBeTruthy();
    // Drama appears both in the donut legend and the "favorite genre" card.
    expect(screen.getAllByText("Drama").length).toBeGreaterThan(0);
    expect(screen.getByText("The Film")).toBeTruthy();
    expect(screen.getByText("42")).toBeTruthy();
  });
});
