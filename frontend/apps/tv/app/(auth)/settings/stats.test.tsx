import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

// Same numbers as web and mobile, drawn as proportional bars readable at
// three metres: genres sorted by time with the longest as the full bar and a
// visible floor for the rest; the favourite genre is the top one; watch time
// is hours and minutes; a failed request is a retry, not "nothing watched";
// the empty state keeps a focusable target so the D-pad can always leave.
vi.mock("react-native", () => ({
  View: ({ children, accessibilityLabel, style }: any) => {
    const flat = Array.isArray(style) ? Object.assign({}, ...style.filter(Boolean)) : style;
    return React.createElement("div", { "aria-label": accessibilityLabel, "data-width": flat?.width }, children);
  },
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  StyleSheet: { create: (s: any) => s },
}));
const router = vi.hoisted(() => ({ back: vi.fn(), push: vi.fn() }));
vi.mock("expo-router", () => ({ Stack: { Screen: () => null }, router }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string, fallback?: string) => fallback ?? k }) }));
const stats = vi.hoisted(() => ({ data: undefined as unknown, isLoading: false, isError: false, refetch: vi.fn(), args: null as any }));
vi.mock("@evey/api", () => ({
  useActivityServiceGetUserStats: (id: string, _p: unknown, o: any) => {
    stats.args = { id, enabled: o.query.enabled };
    return stats;
  },
}));
vi.mock("@evey/store", () => ({ useAuthStore: (sel: (s: { profileId: string | null }) => unknown) => sel({ profileId: "p1" }) }));
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", surface: "#111", text: "#fff", textMuted: "#888", border: "#333", accent: "#f00" } }),
}));
vi.mock("../../../src/components/FocusableCard", () => ({
  FocusableCard: ({ children, onPress, hasTVPreferredFocus, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, "data-focus": String(!!hasTVPreferredFocus) }, children),
}));
vi.mock("../../../src/components/ScreenState", () => ({
  ScreenState: ({ loading, error, onRetry }: any) =>
    React.createElement("div", { "data-testid": "state", "data-loading": String(loading), "data-error": String(error) }, React.createElement("button", { onClick: onRetry }, "retry")),
}));

import StatsScreen from "./stats";

beforeEach(() => {
  vi.clearAllMocks();
  stats.data = undefined;
  stats.isLoading = false;
  stats.isError = false;
});

describe("tv statistics screen", () => {
  it("renders the numbers, the genre bars in order and the most played rows", () => {
    stats.data = {
      totalPlays: 12, uniqueItemsWatched: 7, totalWatchTimeMs: String(5 * 3_600_000 + 7 * 60_000),
      genres: [{ genre: "Drama", totalTimeMs: "1800000" }, { genre: "Action", totalTimeMs: "3600000" }, { genre: "Tiny", totalTimeMs: "1000" }],
      mostPlayed: [{ mediaItemId: "m1", title: "Some Movie", playCount: 4 }, { title: "No id", playCount: 1 }],
    };
    render(<StatsScreen />);
    expect(stats.args).toEqual({ id: "p1", enabled: true });
    expect(screen.getByText("5h 7m")).toBeTruthy();
    expect(screen.getByText("12")).toBeTruthy();
    expect(screen.getByText("7")).toBeTruthy();
    // Favourite genre card + the bar row both name the top genre.
    expect(screen.getAllByText("Action")).toHaveLength(2);

    const rows = ["Action, 1h 0m", "Drama, 30m", "Tiny, 0m"].map((l) => screen.getByLabelText(l));
    const widths = rows.map((r) => (r.querySelector("[data-width]") as HTMLElement).getAttribute("data-width"));
    expect(widths).toEqual(["100%", "50%", "2%"]);

    fireEvent.click(screen.getByRole("button", { name: "Some Movie, 4 common.userStats.playsLabel" }));
    expect(router.push).toHaveBeenCalledWith("/media/m1");
    fireEvent.click(screen.getByRole("button", { name: "No id, 1 common.userStats.playsLabel" }));
    expect(router.push).toHaveBeenCalledTimes(1);
  });

  it("keeps a focusable way out of the empty state", () => {
    stats.data = { totalPlays: 0 };
    render(<StatsScreen />);
    expect(screen.getByText("Nothing watched yet")).toBeTruthy();
    const back = screen.getByRole("button", { name: "Back" });
    expect(back.getAttribute("data-focus")).toBe("true");
    fireEvent.click(back);
    expect(router.back).toHaveBeenCalledTimes(1);
  });

  it("a failed request is a retry, never the empty state", () => {
    stats.isError = true;
    render(<StatsScreen />);
    expect(screen.queryByText("Nothing watched yet")).toBeNull();
    expect(screen.getByTestId("state").getAttribute("data-error")).toBe("true");
    fireEvent.click(screen.getByText("retry"));
    expect(stats.refetch).toHaveBeenCalledTimes(1);
  });
});
