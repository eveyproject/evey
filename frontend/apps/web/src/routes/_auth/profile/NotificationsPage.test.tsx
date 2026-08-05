import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (def: { component: React.ComponentType }) => def,
}));

const mockMarkRead = vi.fn();
const mockMarkAllRead = vi.fn();
const mockDeleteAll = vi.fn();

vi.mock("@evey/api", async (importOriginal) => ({
  // notificationLabel is a pure formatter - keep the REAL one so these tests
  // still assert the wording that reaches the screen.
  notificationLabel: (await importOriginal<typeof import("@evey/api")>()).notificationLabel,
  useNotificationServiceListNotifications: vi.fn(() => ({ data: undefined, isLoading: true })),
  useNotificationServiceMarkRead: vi.fn(() => ({ mutate: mockMarkRead, isPending: false })),
  useNotificationServiceMarkAllRead: vi.fn(() => ({ mutate: mockMarkAllRead, isPending: false })),
  useNotificationServiceDeleteAllNotifications: vi.fn(() => ({ mutate: mockDeleteAll, isPending: false })),
  getNotificationServiceListNotificationsQueryKey: vi.fn(() => ["notifications"]),
  getNotificationServiceGetUnreadCountQueryKey: vi.fn(() => ["notifications", "unread"]),
}));

vi.mock("@tanstack/react-query", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-query")>();
  return { ...actual, useQueryClient: () => ({ invalidateQueries: vi.fn() }) };
});

vi.mock("@/components/ui/Button", () => ({
  Button: ({ children, onClick, loading }: { children: React.ReactNode; onClick?: () => void; loading?: boolean; variant?: string }) =>
    React.createElement("button", { onClick, disabled: loading }, children),
}));

vi.mock("@/components/ui/EmptyState", () => ({
  EmptyState: ({ title }: { title: string }) => React.createElement("div", null, title),
}));

vi.mock("@/components/ui/Modal", () => ({
  Modal: ({ open, children, title }: { open: boolean; children: React.ReactNode; title?: string }) =>
    open ? React.createElement("div", { role: "dialog", "aria-label": title }, children) : null,
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

const mockNotifications = [
  { id: "n1", type: "system", title: "New update available", message: "Version 2.0 released", createdAt: new Date().toISOString(), readAt: undefined },
  { id: "n2", type: "library_new", title: "Library scanned", message: "Added 5 new items", createdAt: new Date().toISOString(), readAt: new Date().toISOString() },
];

describe("NotificationsPage", () => {
  beforeEach(() => { vi.clearAllMocks(); });

  it("shows loading text while fetching", async () => {
    const mod = await import("./notifications.lazy");
    const NotificationsPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(NotificationsPage), { wrapper: createWrapper() });
    expect(screen.getByText("Loading...")).toBeInTheDocument();
  });

  it("shows empty state when no notifications", async () => {
    const { useNotificationServiceListNotifications } = await import("@evey/api");
    vi.mocked(useNotificationServiceListNotifications).mockReturnValue({ data: { notifications: [] }, isLoading: false } as ReturnType<typeof useNotificationServiceListNotifications>);
    const mod = await import("./notifications.lazy");
    const NotificationsPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(NotificationsPage), { wrapper: createWrapper() });
    expect(screen.getByText("No notifications")).toBeInTheDocument();
  });

  it("renders notification titles", async () => {
    const { useNotificationServiceListNotifications } = await import("@evey/api");
    vi.mocked(useNotificationServiceListNotifications).mockReturnValue({ data: { notifications: mockNotifications as never }, isLoading: false } as ReturnType<typeof useNotificationServiceListNotifications>);
    const mod = await import("./notifications.lazy");
    const NotificationsPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(NotificationsPage), { wrapper: createWrapper() });
    expect(screen.getByText("New update available")).toBeInTheDocument();
    expect(screen.getByText("Library scanned")).toBeInTheDocument();
  });

  it("shows Mark all read button when unread notifications exist", async () => {
    const { useNotificationServiceListNotifications } = await import("@evey/api");
    vi.mocked(useNotificationServiceListNotifications).mockReturnValue({ data: { notifications: mockNotifications as never }, isLoading: false } as ReturnType<typeof useNotificationServiceListNotifications>);
    const mod = await import("./notifications.lazy");
    const NotificationsPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(NotificationsPage), { wrapper: createWrapper() });
    expect(screen.getByRole("button", { name: /Mark all read/i })).toBeInTheDocument();
  });

  it("shows Mark read button for unread notification", async () => {
    const { useNotificationServiceListNotifications } = await import("@evey/api");
    vi.mocked(useNotificationServiceListNotifications).mockReturnValue({ data: { notifications: mockNotifications as never }, isLoading: false } as ReturnType<typeof useNotificationServiceListNotifications>);
    const mod = await import("./notifications.lazy");
    const NotificationsPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(NotificationsPage), { wrapper: createWrapper() });
    expect(screen.getByRole("button", { name: /Mark read/i })).toBeInTheDocument();
  });

  it("Delete all button opens confirmation modal", async () => {
    const { useNotificationServiceListNotifications } = await import("@evey/api");
    vi.mocked(useNotificationServiceListNotifications).mockReturnValue({ data: { notifications: mockNotifications as never }, isLoading: false } as ReturnType<typeof useNotificationServiceListNotifications>);
    const mod = await import("./notifications.lazy");
    const NotificationsPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(NotificationsPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /Delete all/i }));
    await waitFor(() => expect(screen.getByRole("dialog", { name: /Delete all notifications/i })).toBeInTheDocument());
  });

  it("Cancel in delete modal closes dialog without calling API", async () => {
    const { useNotificationServiceListNotifications } = await import("@evey/api");
    vi.mocked(useNotificationServiceListNotifications).mockReturnValue({ data: { notifications: mockNotifications as never }, isLoading: false } as ReturnType<typeof useNotificationServiceListNotifications>);
    const mod = await import("./notifications.lazy");
    const NotificationsPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(NotificationsPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /Delete all/i }));
    await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: /Cancel/i }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(mockDeleteAll).not.toHaveBeenCalled();
  });
});
