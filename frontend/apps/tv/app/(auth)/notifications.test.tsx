import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

// The list is asked without a limit (the server's default is the one place
// that number lives); tapping an unread row marks it read and opens its
// title, a read row only opens; "mark all read" exists only while something
// is unread; both refresh the list AND the badge count; a failed fetch is a
// retry, an empty list a plain notice.
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  FlatList: ({ data, renderItem, keyExtractor }: any) =>
    React.createElement("div", null, (data as any[]).map((item, index) => React.createElement(React.Fragment, { key: keyExtractor(item) }, renderItem({ item, index })))),
  StyleSheet: { create: (s: any) => s },
}));
const router = vi.hoisted(() => ({ push: vi.fn() }));
vi.mock("expo-router", () => ({ router }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string, fallback?: string) => (typeof fallback === "string" ? fallback : k) }) }));
const api = vi.hoisted(() => ({
  list: { data: undefined as unknown, isLoading: false, isError: false, refetch: vi.fn() },
  listArgs: undefined as unknown,
  markRead: vi.fn(), markReadOpts: null as any,
  markAll: vi.fn(), markAllOpts: null as any,
}));
vi.mock("@evey/api", () => ({
  notificationLabel: (_t: unknown, n: { title?: string }) => `label:${n.title}`,
  notificationMessage: (_t: unknown, n: { message?: string }) => n.message ?? "",
  useNotificationServiceListNotifications: (...a: unknown[]) => {
    api.listArgs = a;
    return api.list;
  },
  useNotificationServiceMarkRead: (o: any) => {
    api.markReadOpts = o.mutation;
    return { mutate: api.markRead };
  },
  useNotificationServiceMarkAllRead: (o: any) => {
    api.markAllOpts = o.mutation;
    return { mutate: api.markAll };
  },
  getNotificationServiceListNotificationsQueryKey: () => ["notifs"],
  getNotificationServiceGetUnreadCountQueryKey: () => ["unread"],
}));
const invalidate = vi.hoisted(() => vi.fn());
vi.mock("@tanstack/react-query", () => ({ useQueryClient: () => ({ invalidateQueries: invalidate }) }));
const toast = vi.hoisted(() => ({ error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast, useDemoStore: (sel: (s: { demoMode: boolean }) => unknown) => sel({ demoMode: false }) }));
vi.mock("../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", surface: "#111", surfaceRaised: "#222", text: "#fff", textMuted: "#888", accent: "#f00" }, radii: { md: 8 } }),
}));
vi.mock("../../src/components/FocusableCard", () => ({
  FocusableCard: ({ children, onPress, hasTVPreferredFocus, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, "data-focus": String(!!hasTVPreferredFocus) }, children),
}));
vi.mock("../../src/components/ScreenState", () => ({
  ScreenState: ({ loading, error, onRetry }: any) =>
    React.createElement("div", { "data-testid": "state", "data-loading": String(loading), "data-error": String(error) }, React.createElement("button", { onClick: onRetry }, "retry")),
}));

import NotificationsScreen from "./notifications";

beforeEach(() => {
  vi.clearAllMocks();
  api.list = { data: undefined, isLoading: false, isError: false, refetch: vi.fn() };
});

describe("tv notifications screen", () => {
  it("asks without a limit, marks an unread row read on tap and opens its title, and refreshes list and badge", () => {
    api.list.data = {
      notifications: [
        { id: "n1", title: "new_movie", mediaItemId: "m1", message: "Some Movie arrived" },
        { id: "n2", title: "old", readAt: "2026-09-15T00:00:00Z", mediaItemId: "m2" },
        { id: "n3", title: "no-media" },
      ],
    };
    render(<NotificationsScreen />);
    expect(api.listArgs).toEqual([]);
    expect(screen.getByRole("button", { name: "label:new_movie" }).getAttribute("data-focus")).toBe("true");
    expect(screen.getByText("Some Movie arrived")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "label:new_movie" }));
    expect(api.markRead).toHaveBeenCalledWith({ id: "n1" });
    expect(router.push).toHaveBeenCalledWith("/media/m1");

    fireEvent.click(screen.getByRole("button", { name: "label:old" }));
    expect(api.markRead).toHaveBeenCalledTimes(1);
    expect(router.push).toHaveBeenCalledWith("/media/m2");

    fireEvent.click(screen.getByRole("button", { name: "label:no-media" }));
    expect(api.markRead).toHaveBeenCalledWith({ id: "n3" });
    expect(router.push).toHaveBeenCalledTimes(2);

    fireEvent.click(screen.getByRole("button", { name: "common.notifications.markAllRead" }));
    expect(api.markAll).toHaveBeenCalledWith({ data: {} });
    api.markAllOpts.onSuccess();
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["notifs"] });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["unread"] });
    api.markReadOpts.onError();
    expect(toast.error).toHaveBeenCalledWith("Could not mark as read");
  });

  it("hides mark-all once everything is read, and says when there is nothing", () => {
    api.list.data = { notifications: [{ id: "n2", title: "old", readAt: "2026-09-15T00:00:00Z" }] };
    const { unmount } = render(<NotificationsScreen />);
    expect(screen.queryByRole("button", { name: "common.notifications.markAllRead" })).toBeNull();
    unmount();

    api.list.data = { notifications: [] };
    render(<NotificationsScreen />);
    expect(screen.getByText("common.noNotifications")).toBeTruthy();
  });

  it("a failed fetch is a retry, never an empty inbox", () => {
    api.list.isError = true;
    render(<NotificationsScreen />);
    expect(screen.queryByText("common.noNotifications")).toBeNull();
    fireEvent.click(screen.getByText("retry"));
    expect(api.list.refetch).toHaveBeenCalledTimes(1);
  });
});
