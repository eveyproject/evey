// The favorite-genre tile had no numberOfLines, unlike the "most played"
// title tile right below it - a long localized genre name overflowed the
// half-width card instead of being clipped with an ellipsis.
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, numberOfLines, ...p }: any) =>
    React.createElement("span", { "data-number-of-lines": numberOfLines, ...p }, children),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  Pressable: ({ children, onPress, ...p }: any) => React.createElement("button", { onClick: onPress, ...p }, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "loading", ...p }),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
  Dimensions: { get: () => ({ width: 390, height: 844 }) },
}));

vi.mock("expo-router", () => ({ Stack: { Screen: () => null }, router: { push: vi.fn() } }));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => React.createElement("span", {}) }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("react-native-gifted-charts", () => ({ PieChart: () => null, BarChart: () => null }));
vi.mock("@evey/store", () => ({ useAuthStore: (selector: (s: any) => unknown) => selector({ profileId: "p1" }) }));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", text: "#fff", textMuted: "#888", accent: "#f00", surface: "#111", border: "#333" },
    radii: { md: 8 },
    spacing: { sm: 8, md: 12, lg: 16, xl: 24 },
  }),
}));
vi.mock("../lib/chartPalette", () => ({ chartColorAt: () => "#f00" }));

const STATS = {
  totalPlays: 42,
  totalWatchTimeMs: "3600000",
  uniqueItemsWatched: 10,
  genres: [{ genre: "Documentaire historique franco-britannique", totalTimeMs: "1000", percent: 60 }],
  byMonth: [],
  mostPlayed: [{ title: "Some Show", playCount: 5 }],
};

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useActivityServiceGetUserStats: () => ({ data: STATS, isLoading: false, isError: false, refetch: vi.fn() }),
}));

import StatsScreen from "../../app/(auth)/settings/stats";

describe("user stats - favorite genre tile truncation", () => {
  it("clips a long favorite-genre name instead of letting it overflow the tile", () => {
    render(React.createElement(StatsScreen));

    // The genre name also appears in the breakdown legend below; only the
    // top favorite-genre tile is under test here.
    const [genreTileValue] = screen.getAllByText("Documentaire historique franco-britannique");
    expect(genreTileValue.getAttribute("data-number-of-lines")).toBe("1");
  });
});
