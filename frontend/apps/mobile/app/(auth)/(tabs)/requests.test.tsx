import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

// mineLoading was declared but never read: the "Mes demandes" tab rendered its
// ListEmptyComponent ("no requests") for the whole span of the initial fetch,
// before the request finally answered.

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  TextInput: (p: any) => React.createElement("input", p),
  FlatList: ({ data, renderItem, keyExtractor, ListEmptyComponent }: any) =>
    React.createElement(
      "div",
      null,
      (data ?? []).length === 0
        ? (ListEmptyComponent ?? null)
        : data.map((item: any, i: number) =>
            React.createElement(React.Fragment, { key: keyExtractor ? keyExtractor(item, i) : i }, renderItem({ item, index: i })),
          ),
    ),
  TouchableOpacity: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "spinner", ...p }),
}));
vi.mock("expo-image", () => ({ Image: (p: any) => React.createElement("img", { alt: "", ...p }) }));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: (p: any) => React.createElement("span", p), glyphMap: {} }));
vi.mock("expo-router", () => ({ useRouter: () => ({ push: vi.fn() }) }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock("@tanstack/react-query", () => ({ useQueryClient: () => ({ invalidateQueries: vi.fn() }) }));
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: {
      bg: "#000", text: "#fff", textMuted: "#888", accent: "#f00", onAccent: "#fff",
      surface: "#111", surfaceRaised: "#222", border: "#333", success: "#0f0", danger: "#f00", warning: "#ff0",
    },
    radii: { sm: 4, md: 8, lg: 12 },
    spacing: { sm: 4, md: 8, lg: 12, xl: 16 },
  }),
}));
vi.mock("../../../src/components/Skeleton", () => ({ Skeleton: () => React.createElement("div", { "data-testid": "skeleton" }) }));
vi.mock("../../../src/components/LoadFailed", () => ({ LoadFailed: () => React.createElement("div", { "data-testid": "load-failed" }) }));
vi.mock("@evey/store", () => ({ toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() }, useDemoStore: (sel: (s: { demoMode: boolean }) => unknown) => sel({ demoMode: false }) }));

let mineLoading = false;
let mineError = false;
const refetchMine = vi.fn();

vi.mock("@evey/api", () => ({
  useMediaRequestServiceGetRequestPolicy: () => ({ data: { canRequest: true }, isLoading: false, isError: false, refetch: vi.fn() }),
  useMediaRequestServiceSearchTitles: () => ({ data: undefined, isFetching: false, isError: false, refetch: vi.fn() }),
  useMediaRequestServiceCreateRequest: () => ({ mutate: vi.fn(), isPending: false }),
  useMediaRequestServiceListMyRequests: () => ({
    data: { items: [] },
    isLoading: mineLoading,
    isError: mineError,
    refetch: refetchMine,
  }),
  getMediaRequestServiceListMyRequestsQueryKey: () => ["mine"],
}));

import RequestsScreen from "./requests";

beforeEach(() => {
  vi.clearAllMocks();
  mineLoading = false;
  mineError = false;
});

describe("RequestsScreen - 'Mes demandes' tab, loading vs empty", () => {
  it("shows a loading indicator, not 'no requests', while the request is in flight", () => {
    mineLoading = true;
    render(<RequestsScreen />);
    fireEvent.click(screen.getByText("requests.tabMine"));

    expect(screen.getAllByTestId("skeleton").length).toBeGreaterThan(0);
    expect(screen.queryByText("requests.noRequests")).toBeNull();
  });

  it("shows the empty message once the request has genuinely settled empty", () => {
    render(<RequestsScreen />);
    fireEvent.click(screen.getByText("requests.tabMine"));

    expect(screen.getByText("requests.noRequests")).toBeTruthy();
  });
});
