import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockNavigate = vi.fn();
vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (def: { component: React.ComponentType }) => def,
  useNavigate: () => mockNavigate,
}));

// The list is paged through the real useInfiniteQuery; the fetcher is what the
// tests script. A pending promise keeps the page in its loading state.
const mockList = vi.fn<(...args: unknown[]) => Promise<{ notifications: unknown[] }>>(() => new Promise(() => {}));
const mockMarkRead = vi.fn();
const mockMarkAllRead = vi.fn();
const mockDeleteAll = vi.fn();

vi.mock("@evey/api", async (importOriginal) => ({
  // notificationLabel is a pure formatter - keep the REAL one so these tests
  // still assert the wording that reaches the screen.
  notificationLabel: (await importOriginal<typeof import("@evey/api")>()).notificationLabel,
  notificationMessage: (await importOriginal<typeof import("@evey/api")>()).notificationMessage,
  notificationServiceListNotifications: (...args: unknown[]) => mockList(...args),
  useNotificationServiceMarkRead: vi.fn(() => ({ mutate: mockMarkRead, isPending: false })),
  useNotificationServiceMarkAllRead: vi.fn(() => ({ mutate: mockMarkAllRead, isPending: false })),
  useNotificationServiceDeleteAllNotifications: vi.fn(() => ({ mutate: mockDeleteAll, isPending: false })),
  getNotificationServiceListNotificationsQueryKey: vi.fn(() => ["notifications"]),
  apiErrorMessage: (err: { message?: string }, _t: unknown, fallback: string) => err?.message ?? fallback,
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

vi.mock("@/components/ui/Spinner", () => ({
  Spinner: () => React.createElement("div", { "data-testid": "spinner" }),
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
  beforeEach(() => {
    vi.clearAllMocks();
    mockList.mockImplementation(() => new Promise(() => {}));
  });

  it("shows loading text while fetching", async () => {
    const mod = await import("./notifications.lazy");
    const NotificationsPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(NotificationsPage), { wrapper: createWrapper() });
    expect(screen.getByText("Loading...")).toBeInTheDocument();
  });

  it("shows empty state when no notifications", async () => {
    mockList.mockResolvedValue({ notifications: [] });
    const mod = await import("./notifications.lazy");
    const NotificationsPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(NotificationsPage), { wrapper: createWrapper() });
    expect(await screen.findByText("No notifications")).toBeInTheDocument();
  });

  it("renders notification titles", async () => {
    mockList.mockResolvedValue({ notifications: mockNotifications as never });
    const mod = await import("./notifications.lazy");
    const NotificationsPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(NotificationsPage), { wrapper: createWrapper() });
    expect(await screen.findByText("New update available")).toBeInTheDocument();
    expect(screen.getByText("Library scanned")).toBeInTheDocument();
  });

  it("shows Mark all read button when unread notifications exist", async () => {
    mockList.mockResolvedValue({ notifications: mockNotifications as never });
    const mod = await import("./notifications.lazy");
    const NotificationsPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(NotificationsPage), { wrapper: createWrapper() });
    expect(await screen.findByRole("button", { name: /Mark all read/i })).toBeInTheDocument();
  });

  it("shows Mark read button for unread notification", async () => {
    mockList.mockResolvedValue({ notifications: mockNotifications as never });
    const mod = await import("./notifications.lazy");
    const NotificationsPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(NotificationsPage), { wrapper: createWrapper() });
    expect(await screen.findByRole("button", { name: /Mark read/i })).toBeInTheDocument();
  });

  it("Delete all button opens confirmation modal", async () => {
    mockList.mockResolvedValue({ notifications: mockNotifications as never });
    const mod = await import("./notifications.lazy");
    const NotificationsPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(NotificationsPage), { wrapper: createWrapper() });
    fireEvent.click(await screen.findByRole("button", { name: /Delete all/i }));
    await waitFor(() => expect(screen.getByRole("dialog", { name: /Delete all notifications/i })).toBeInTheDocument());
  });

  it("Cancel in delete modal closes dialog without calling API", async () => {
    mockList.mockResolvedValue({ notifications: mockNotifications as never });
    const mod = await import("./notifications.lazy");
    const NotificationsPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(NotificationsPage), { wrapper: createWrapper() });
    fireEvent.click(await screen.findByRole("button", { name: /Delete all/i }));
    await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: /Cancel/i }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(mockDeleteAll).not.toHaveBeenCalled();
  });

  it("opens the title a notification is about and marks it read on the way", async () => {
    mockList.mockResolvedValue({ notifications: [
      { id: "n3", type: "new_movie", title: "New movie", message: "x", mediaItemId: "mi-9", createdAt: new Date().toISOString() },
      { id: "n4", type: "system", title: "Backup done", message: "y", createdAt: new Date().toISOString() },
    ] });
    const mod = await import("./notifications.lazy");
    const NotificationsPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(NotificationsPage), { wrapper: createWrapper() });

    const row = await screen.findByTestId("notification-row-n3");
    expect(row).toHaveAttribute("role", "link");
    fireEvent.click(row);
    expect(mockNavigate).toHaveBeenCalledWith({ to: "/media/mi-9" });
    expect(mockMarkRead).toHaveBeenCalledWith({ id: "n3" });

    // A notification with nowhere to go stays a plain row.
    expect(screen.getByTestId("notification-row-n4")).not.toHaveAttribute("role");
  });

  // The page used to ask for nothing and get the server's 200-row cap, past
  // which older notifications were silently gone. It now pages by 50 and a
  // full page means there may be more.
  it("asks for the first page and fetches the next one when the sentinel is reached", async () => {
    const page = (start: number, n: number) =>
      Array.from({ length: n }, (_, i) => ({ id: `n${start + i}`, type: "system", title: `Notice ${start + i}`, message: "m", createdAt: new Date().toISOString() }));
    mockList.mockImplementation((params: unknown) => {
      const offset = (params as { offset: number }).offset;
      return Promise.resolve({ notifications: offset === 0 ? page(0, 50) : page(50, 3) });
    });
    let observed: ((entries: { isIntersecting: boolean }[]) => void) | undefined;
    class FakeObserver {
      constructor(cb: (entries: { isIntersecting: boolean }[]) => void) { observed = cb; }
      observe() {}
      disconnect() {}
    }
    vi.stubGlobal("IntersectionObserver", FakeObserver);
    try {
      const mod = await import("./notifications.lazy");
      const NotificationsPage = (mod.Route as unknown as { component: React.ComponentType }).component;
      render(React.createElement(NotificationsPage), { wrapper: createWrapper() });
      expect(await screen.findByText("Notice 49")).toBeInTheDocument();
      expect(mockList).toHaveBeenCalledWith({ limit: 50, offset: 0 }, expect.anything());
      expect(screen.queryByText("Notice 50")).not.toBeInTheDocument();

      observed?.([{ isIntersecting: true }]);
      expect(await screen.findByText("Notice 52")).toBeInTheDocument();
      expect(mockList).toHaveBeenCalledWith({ limit: 50, offset: 50 }, expect.anything());
      // The short second page is the end: nothing more is requested.
      observed?.([{ isIntersecting: true }]);
      await waitFor(() => expect(mockList).toHaveBeenCalledTimes(2));
    } finally {
      vi.unstubAllGlobals();
    }
  });
});

