import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

// The genre pie is at most eight slices, in the SERVER's order and with the
// server's percentages (it alone knows every stored spelling); the monthly
// bars cover the last twelve months with a zero for a silent month; loading,
// failure (retry) and "nothing watched" are three screens; most played opens
// the title.
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
  Pressable: ({ children, onPress, accessibilityRole }: any) => React.createElement("button", { onClick: onPress, role: accessibilityRole }, children),
  Dimensions: { get: () => ({ width: 400 }) },
  StyleSheet: { create: (s: any) => s },
}));
const charts = vi.hoisted(() => ({ pie: null as any, bars: null as any }));
vi.mock("react-native-gifted-charts", () => ({
  PieChart: ({ data }: any) => { charts.pie = data; return React.createElement("div", { "data-testid": "pie" }); },
  BarChart: ({ data, maxValue }: any) => { charts.bars = { data, maxValue }; return React.createElement("div", { "data-testid": "bars" }); },
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
const router = vi.hoisted(() => ({ push: vi.fn() }));
vi.mock("expo-router", () => ({ Stack: { Screen: () => null }, router }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
const stats = vi.hoisted(() => ({ data: undefined as unknown, isLoading: false, isError: false, refetch: vi.fn(), args: null as any }));
vi.mock("@evey/api", () => ({
  useActivityServiceGetUserStats: (id: string, _p: unknown, o: any) => {
    stats.args = { id, enabled: o.query.enabled };
    return stats;
  },
}));
vi.mock("@evey/store", () => ({ useAuthStore: (sel: (s: { profileId: string | null }) => unknown) => sel({ profileId: "p1" }) }));
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", surface: "#111", text: "#fff", textMuted: "#888", accent: "#f00", border: "#333" }, radii: { md: 8 }, spacing: { sm: 8, md: 16, lg: 24 } }),
}));
vi.mock("../../../src/lib/chartPalette", () => ({ chartColorAt: (_t: unknown, i: number) => `c${i}` }));
vi.mock("../../../src/components/LoadFailed", () => ({ LoadFailed: ({ onRetry }: any) => React.createElement("button", { onClick: onRetry }, "retry") }));

import StatsScreen from "./stats";

beforeEach(() => {
  vi.clearAllMocks();
  stats.data = undefined;
  stats.isLoading = false;
  stats.isError = false;
  charts.pie = null;
  charts.bars = null;
});

describe("stats screen (mobile)", () => {
  it("renders the numbers, at most eight genre slices in time order, twelve monthly bars, and opens a most-played title", () => {
    const now = new Date();
    const genres = Array.from({ length: 10 }, (_, i) => ({ genre: `G${i}`, totalTimeMs: String((10 - i) * 60_000), percent: 10 - i }));
    stats.data = {
      totalPlays: 12, uniqueItemsWatched: 5, totalWatchTimeMs: String(3 * 3_600_000 + 5 * 60_000),
      genres: [genres[1], genres[0], ...genres.slice(2)],
      byMonth: [{ year: now.getFullYear(), month: now.getMonth() + 1, totalTimeMs: String(2 * 3_600_000) }],
      mostPlayed: [{ mediaItemId: "m1", title: "Heat", playCount: 4, totalTimeMs: String(90 * 60_000) }, { title: "no id", playCount: 1 }],
    };
    render(<StatsScreen />);
    expect(stats.args).toEqual({ id: "p1", enabled: true });
    expect(screen.getByText("3h 5m")).toBeTruthy();
    expect(screen.getByText("12")).toBeTruthy();
    expect(screen.getByText("5")).toBeTruthy();
    expect(screen.getAllByText("G0")).toHaveLength(2);

    expect(charts.pie).toHaveLength(8);
    expect(charts.pie.map((d: { text: string }) => d.text)).toEqual(["G0", "G1", "G2", "G3", "G4", "G5", "G6", "G7"]);
    expect(charts.pie[0].color).toBe("c0");
    expect(screen.getByText("10%")).toBeTruthy();

    expect(charts.bars.data).toHaveLength(12);
    expect(charts.bars.data.at(-1).value).toBe(2);
    expect(charts.bars.data[0].value).toBe(0);
    expect(charts.bars.maxValue).toBe(Math.ceil(2 * 1.1));

    expect(screen.getByText("1.5h")).toBeTruthy();
    fireEvent.click(screen.getByText("Heat").closest("button")!);
    expect(router.push).toHaveBeenCalledWith("/media/m1");
    fireEvent.click(screen.getByText("no id").closest("button")!);
    expect(router.push).toHaveBeenCalledTimes(1);
  });

  it("keeps loading, failure and nothing-watched apart", () => {
    stats.isLoading = true;
    const first = render(<StatsScreen />);
    expect(screen.getByTestId("spinner")).toBeTruthy();
    first.unmount();

    stats.isLoading = false;
    stats.isError = true;
    const second = render(<StatsScreen />);
    fireEvent.click(screen.getByText("retry"));
    expect(stats.refetch).toHaveBeenCalledTimes(1);
    expect(screen.queryByText("common.userStats.noData")).toBeNull();
    second.unmount();

    stats.isError = false;
    stats.data = { totalPlays: 0 };
    render(<StatsScreen />);
    expect(screen.getByText("common.userStats.noData")).toBeTruthy();
    expect(screen.queryByTestId("pie")).toBeNull();
  });
});
