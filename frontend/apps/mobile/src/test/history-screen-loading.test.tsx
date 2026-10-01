// Both the error and the empty branches required `!isLoading`, so while the
// first page was in flight the screen fell through to a SectionList with no
// sections - a blank screen indistinguishable from "no watch history".
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  SectionList: ({ sections }: any) =>
    React.createElement("div", { "data-testid": "section-list" }, `sections:${sections.length}`),
  RefreshControl: () => null,
  Alert: { alert: vi.fn() },
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));

vi.mock("../components/Skeleton", () => ({
  Skeleton: () => React.createElement("div", { "data-testid": "skeleton" }),
}));

vi.mock("expo-image", () => ({ Image: (p: any) => React.createElement("img", { ...p, alt: "" }) }));
vi.mock("expo-router", () => ({ router: { push: vi.fn() }, Stack: { Screen: () => null } }));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => React.createElement("span", {}) }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key) }),
}));
vi.mock("@tanstack/react-query", () => ({ useQueryClient: () => ({ invalidateQueries: vi.fn() }) }));
vi.mock("@evey/store", () => ({ useDemoStore: (sel: (s: { demoMode: boolean }) => unknown) => sel({ demoMode: false }), isDemoMode: () => false, toast: { error: vi.fn(), success: vi.fn() } }));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", surface: "#111", surfaceRaised: "#222", text: "#fff", textMuted: "#888", accent: "#f00", border: "#333" },
    radii: { sm: 4, md: 8 },
    spacing: { sm: 8, md: 12, lg: 16, xl: 24 },
  }),
}));
vi.mock("../hooks/useServerUrl", () => ({ useServerUrl: () => "http://test-server" }));
vi.mock("../hooks/useAuthenticatedImage", () => ({ useAuthenticatedImage: () => undefined }));
vi.mock("../components/ProgressBar", () => ({ ProgressBar: () => null }));

let historyResult = { data: undefined as unknown, isLoading: true, isFetching: false, isError: false, refetch: vi.fn() };
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  usePlaybackServiceGetWatchHistory: () => historyResult,
  usePlaybackServiceMarkAsUnwatched: () => ({ mutate: vi.fn(), isPending: false }),
  usePlaybackServiceClearWatchHistory: () => ({ mutate: vi.fn() }),
  getPlaybackServiceGetWatchHistoryQueryKey: () => ["history"],
}));

import HistoryScreen from "../../app/(auth)/history";

beforeEach(() => {
  vi.clearAllMocks();
});

describe("history screen - loading state", () => {
  it("shows skeletons while loading, not the empty-history message", () => {
    historyResult = { data: undefined, isLoading: true, isFetching: false, isError: false, refetch: vi.fn() };
    render(React.createElement(HistoryScreen));

    expect(screen.queryByText("No watch history")).toBeNull();
    expect(screen.queryByTestId("section-list")).toBeNull();
    expect(screen.getAllByTestId("skeleton").length).toBeGreaterThan(0);
  });

  it("shows the empty-history message once loading has actually finished with no items", () => {
    historyResult = { data: { items: [] }, isLoading: false, isFetching: false, isError: false, refetch: vi.fn() };
    render(React.createElement(HistoryScreen));

    expect(screen.getByText("No watch history")).toBeTruthy();
  });
});
