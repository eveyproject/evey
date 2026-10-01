import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

// A tap marks an unread row read and leads where the notification points:
// a media request opens the admin queue (no title exists yet), a title opens
// the title; the row's own "mark read" never navigates; "mark all" exists
// only while something is unread; clearing everything is confirmed first;
// every write refreshes both the list and the badge; a failed fetch is a
// retry, not an empty inbox.
const alerts = vi.hoisted(() => ({ last: null as any }));
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  FlatList: ({ data, renderItem, keyExtractor, ListEmptyComponent }: any) =>
    React.createElement("div", null, (data ?? []).length === 0 ? ListEmptyComponent : (data as any[]).map((item) => React.createElement(React.Fragment, { key: keyExtractor(item) }, renderItem({ item })))),
  Pressable: ({ children, onPress, accessibilityRole, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: (e: any) => onPress?.(e), role: accessibilityRole, "aria-label": accessibilityLabel }, children),
  Alert: { alert: (title: string, message: string, buttons: any[]) => { alerts.last = { title, message, buttons }; } },
  RefreshControl: () => null,
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
const router = vi.hoisted(() => ({ push: vi.fn() }));
vi.mock("expo-router", () => ({
  router,
  Stack: { Screen: ({ options }: any) => React.createElement("div", { "data-testid": "header" }, options.headerRight?.()) },
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string, fallback?: string) => (typeof fallback === "string" ? fallback : k) }) }));
const api = vi.hoisted(() => ({
  list: { data: undefined as unknown, isLoading: false, isError: false, refetch: vi.fn() },
  markRead: vi.fn(), markReadOpts: null as any,
  markAll: vi.fn(), markAllOpts: null as any,
  deleteAll: vi.fn(), deleteAllOpts: null as any,
}));
vi.mock("@evey/api", () => ({
  notificationLabel: (_t: unknown, n: { title?: string }) => `label:${n.title}`,
  useNotificationServiceListNotifications: () => api.list,
  useNotificationServiceMarkRead: (o: any) => { api.markReadOpts = o.mutation; return { mutate: api.markRead }; },
  useNotificationServiceMarkAllRead: (o: any) => { api.markAllOpts = o.mutation; return { mutate: api.markAll }; },
  useNotificationServiceDeleteAllNotifications: (o: any) => { api.deleteAllOpts = o.mutation; return { mutate: api.deleteAll, isPending: false }; },
  getNotificationServiceListNotificationsQueryKey: () => ["notifs"],
  getNotificationServiceGetUnreadCountQueryKey: () => ["unread"],
}));
const invalidate = vi.hoisted(() => vi.fn());
vi.mock("@tanstack/react-query", () => ({ useQueryClient: () => ({ invalidateQueries: invalidate }) }));
const toast = vi.hoisted(() => ({ error: vi.fn() }));
vi.mock("@evey/store", () => ({ useDemoStore: (sel: (s: { demoMode: boolean }) => unknown) => sel({ demoMode: false }), isDemoMode: () => false, toast }));
vi.mock("../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", surface: "#111", surfaceRaised: "#222", text: "#fff", textMuted: "#888", accent: "#f00", border: "#333", danger: "#f33" }, radii: { md: 8 }, spacing: { sm: 8, md: 16, lg: 24 } }),
}));
vi.mock("../../src/components/LoadFailed", () => ({ LoadFailed: ({ onRetry }: any) => React.createElement("button", { onClick: onRetry }, "retry") }));
vi.mock("../../src/components/Skeleton", () => ({ Skeleton: () => React.createElement("div", { "data-testid": "skeleton" }) }));

import NotificationsScreen from "./notifications";

beforeEach(() => {
  vi.clearAllMocks();
  alerts.last = null;
  api.list = { data: undefined, isLoading: false, isError: false, refetch: vi.fn() };
});

describe("notifications screen (mobile)", () => {
  it("routes a tap by what the notification points at and marks it read; the row's mark-read never navigates", () => {
    api.list.data = {
      notifications: [
        { id: "n1", title: "req", type: "media_request" },
        { id: "n2", title: "movie", mediaItemId: "m1" },
        { id: "n3", title: "seen", readAt: "2026-09-15T00:00:00Z", mediaItemId: "m2" },
      ],
    };
    render(<NotificationsScreen />);
    fireEvent.click(screen.getByRole("button", { name: "label:req" }));
    expect(api.markRead).toHaveBeenCalledWith({ id: "n1" });
    expect(router.push).toHaveBeenCalledWith("/(auth)/admin/media-requests");

    fireEvent.click(screen.getByRole("button", { name: "label:seen" }));
    expect(api.markRead).toHaveBeenCalledTimes(1);
    expect(router.push).toHaveBeenCalledWith("/media/m2");

    const marks = screen.getAllByRole("button", { name: "common.notifications.markRead" });
    expect(marks).toHaveLength(2);
    fireEvent.click(marks[1]);
    expect(api.markRead).toHaveBeenCalledWith({ id: "n2" });
    expect(router.push).toHaveBeenCalledTimes(2);

    api.markReadOpts.onSuccess();
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["notifs"] });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["unread"] });
  });

  it("offers mark-all only while something is unread, and confirms before clearing everything", () => {
    api.list.data = { notifications: [{ id: "n1", title: "a" }] };
    const first = render(<NotificationsScreen />);
    fireEvent.click(screen.getByRole("button", { name: "common.markAllRead" }));
    expect(api.markAll).toHaveBeenCalledWith({ data: {} });
    fireEvent.click(screen.getByRole("button", { name: "Clear all" }));
    expect(api.deleteAll).not.toHaveBeenCalled();
    alerts.last.buttons.find((b: any) => b.style === "destructive").onPress();
    expect(api.deleteAll).toHaveBeenCalledWith({});
    api.deleteAllOpts.onError();
    expect(toast.error).toHaveBeenCalledWith("Failed to clear notifications");
    first.unmount();

    api.list.data = { notifications: [{ id: "n1", title: "a", readAt: "2026-09-15T00:00:00Z" }] };
    render(<NotificationsScreen />);
    expect(screen.queryByRole("button", { name: "common.markAllRead" })).toBeNull();
  });

  it("keeps loading, failure and an empty inbox apart", () => {
    api.list.isLoading = true;
    const first = render(<NotificationsScreen />);
    expect(screen.getAllByTestId("skeleton").length).toBeGreaterThan(0);
    first.unmount();

    api.list.isLoading = false;
    api.list.isError = true;
    const second = render(<NotificationsScreen />);
    expect(screen.queryByText("common.noNotifications")).toBeNull();
    fireEvent.click(screen.getByText("retry"));
    expect(api.list.refetch).toHaveBeenCalledTimes(1);
    second.unmount();

    api.list.isError = false;
    api.list.data = { notifications: [] };
    render(<NotificationsScreen />);
    expect(screen.getByText("common.noNotifications")).toBeTruthy();
  });
});
