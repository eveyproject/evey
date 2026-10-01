// Same defect as the watch history screen: the error and empty branches both
// required `!isLoading`, so the first fetch fell through to an empty
// FlatList - a blank screen indistinguishable from "no notifications".
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  FlatList: ({ data }: any) => React.createElement("div", { "data-testid": "notif-list" }, `items:${data.length}`),
  RefreshControl: () => null,
  Alert: { alert: vi.fn() },
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));

vi.mock("expo-router", () => ({ router: { push: vi.fn() }, Stack: { Screen: () => null } }));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => React.createElement("span", {}), glyphMap: {} }));
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
vi.mock("../components/Skeleton", () => ({
  Skeleton: () => React.createElement("div", { "data-testid": "skeleton" }),
}));

let listResult = { data: undefined as unknown, isLoading: true, isError: false, refetch: vi.fn() };
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useNotificationServiceListNotifications: () => listResult,
  useNotificationServiceMarkRead: () => ({ mutate: vi.fn() }),
  useNotificationServiceMarkAllRead: () => ({ mutate: vi.fn() }),
  useNotificationServiceDeleteAllNotifications: () => ({ mutate: vi.fn() }),
  getNotificationServiceListNotificationsQueryKey: () => ["notifications"],
  getNotificationServiceGetUnreadCountQueryKey: () => ["unread-count"],
}));

import NotificationsScreen from "../../app/(auth)/notifications";

beforeEach(() => {
  vi.clearAllMocks();
});

describe("notifications screen - loading state", () => {
  it("shows skeletons while loading, not the empty-notifications message", () => {
    listResult = { data: undefined, isLoading: true, isError: false, refetch: vi.fn() };
    render(React.createElement(NotificationsScreen));

    expect(screen.queryByText("common.noNotifications")).toBeNull();
    expect(screen.queryByTestId("notif-list")).toBeNull();
    expect(screen.getAllByTestId("skeleton").length).toBeGreaterThan(0);
  });
});
