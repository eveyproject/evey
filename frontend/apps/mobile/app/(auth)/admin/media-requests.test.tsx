import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import React from "react";

// The queue used to render each request with no artwork at all - unlike the
// web admin requests page, which shows a poster thumbnail (or a type-icon
// fallback) next to every row. This proves the mobile queue now does too.

// --- React Native mock -----------------------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  TextInput: ({ value, onChangeText, placeholder }: any) =>
    React.createElement("input", {
      "aria-label": placeholder,
      value: value ?? "",
      onChange: (e: React.ChangeEvent<HTMLInputElement>) => onChangeText?.(e.target.value),
    }),
  Pressable: ({ children, onPress, disabled, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, disabled, "aria-label": accessibilityLabel, ...p }, children),
  Switch: ({ value, onValueChange }: any) =>
    React.createElement("button", {
      type: "button",
      role: "switch",
      "aria-checked": !!value,
      onClick: () => onValueChange?.(!value),
    }),
  FlatList: ({ data, renderItem, keyExtractor, ListHeaderComponent, ListEmptyComponent, ListFooterComponent }: any) =>
    React.createElement(
      "div",
      { "data-testid": "requests-list" },
      ListHeaderComponent ?? null,
      data.length === 0
        ? (ListEmptyComponent ?? null)
        : data.map((item: any) =>
            React.createElement(React.Fragment, { key: keyExtractor(item) }, renderItem({ item })),
          ),
      ListFooterComponent ?? null,
    ),
  Modal: ({ children, visible }: any) => (visible ? React.createElement("div", { "data-testid": "modal" }, children) : null),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "loading", ...p }),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));

// --- expo-image ------------------------------------------------------------------------
vi.mock("expo-image", () => ({
  Image: ({ source, ...p }: any) => React.createElement("img", { src: source?.uri, alt: "", ...p }),
}));

// --- expo-router ------------------------------------------------------------------------
vi.mock("expo-router", () => ({
  Stack: {
    Screen: ({ options }: any) => React.createElement("div", { "data-testid": "stack-screen", title: options?.title }),
  },
}));

// --- i18next ------------------------------------------------------------------------------
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (_key: string, fallback?: string) => fallback ?? _key }),
}));

// --- ThemeProvider --------------------------------------------------------------------
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: {
      bg: "#000", surface: "#111", surfaceRaised: "#222", text: "#fff",
      textMuted: "#888", accent: "#f00", error: "#e00", success: "#0f0", warning: "#ff0",
      border: "#333", onAccent: "#fff", onError: "#fff",
    },
    radii: { sm: 4, md: 8, lg: 12 },
  }),
}));

// --- @evey/store -------------------------------------------------------------------------
vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

// --- @tanstack/react-query -----------------------------------------------------------------
vi.mock("@tanstack/react-query", () => ({
  useQueryClient: () => ({ invalidateQueries: vi.fn() }),
  // The queue pages through useInfiniteQuery; the harness hands it one page.
  useInfiniteQuery: () => ({
    ...requestsQueryState,
    data: requestsQueryState.data ? { pages: [requestsQueryState.data] } : undefined,
    fetchNextPage: vi.fn(),
    hasNextPage: false,
    isFetchingNextPage: false,
  }),
}));

// --- @evey/api -----------------------------------------------------------------------------
let requestsQueryState = {
  data: {
    items: [
      { id: "req-1", title: "Some Movie", status: "pending", posterUrl: "https://image.example/poster-1.jpg", mediaType: "movie" },
      { id: "req-2", title: "Some Series", status: "pending", mediaType: "tv" },
    ],
  },
  isLoading: false,
  isError: false,
  refetch: vi.fn(),
};
vi.mock("@evey/api", async (importOriginal) => ({
  // The link, date and rating formatters are pure: keep the real ones so the
  // row renders what the operator sees.
  ...(await importOriginal<typeof import("@evey/api")>()),
  adminMediaRequestServiceListRequests: vi.fn(),
  useAdminMediaRequestServiceAnswerRequest: () => ({ mutate: vi.fn(), isPending: false }),
  useAdminMediaRequestServiceGetAccessSettings: () => ({ data: { overrides: [] }, isLoading: false, isError: false, refetch: vi.fn() }),
  useAdminMediaRequestServiceSetAccessDefault: () => ({ mutate: vi.fn(), isPending: false }),
  useAdminMediaRequestServiceSetUserAccess: () => ({ mutate: vi.fn(), isPending: false }),
  getAdminMediaRequestServiceListRequestsQueryKey: () => ["requests"],
  getAdminMediaRequestServiceGetAccessSettingsQueryKey: () => ["requests-access"],
  useUserServiceListUsers: () => ({ data: { users: [] } }),
  V1AccessMode: { ACCESS_MODE_ENABLED: "ACCESS_MODE_ENABLED", ACCESS_MODE_DISABLED: "ACCESS_MODE_DISABLED", ACCESS_MODE_DEFAULT: "ACCESS_MODE_DEFAULT" },
}));

// -------------------------------------------------------------------------------------
import AdminMediaRequestsScreen from "./media-requests";

beforeEach(() => {
  vi.clearAllMocks();
  requestsQueryState = {
    data: {
      items: [
        { id: "req-1", title: "Some Movie", status: "pending", posterUrl: "https://image.example/poster-1.jpg", mediaType: "movie" },
        { id: "req-2", title: "Some Series", status: "pending", mediaType: "tv" },
      ],
    },
    isLoading: false,
    isError: false,
    refetch: vi.fn(),
  };
});

describe("AdminMediaRequestsScreen - poster thumbnail", () => {
  it("renders the request poster when the request carries a posterUrl", () => {
    render(<AdminMediaRequestsScreen />);

    const img = screen.getByAltText("") as HTMLImageElement;
    expect(img.src).toBe("https://image.example/poster-1.jpg");
  });

  it("falls back to a type icon, not a broken image, when the request has no posterUrl", () => {
    render(<AdminMediaRequestsScreen />);

    // req-2 has no posterUrl and mediaType "tv" - the vector-icon mock renders
    // every icon as data-testid=`icon-${name}` regardless of family.
    expect(screen.getByTestId("icon-tv-outline")).toBeTruthy();
  });
});
