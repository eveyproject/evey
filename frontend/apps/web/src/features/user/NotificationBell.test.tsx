import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { NotificationBell } from "./NotificationBell";

const mockMutate = vi.fn();
const { notificationsRef, navigateSpy } = vi.hoisted(() => ({
  navigateSpy: vi.fn(),
  notificationsRef: {
    current: [
      { id: "1", title: "New movie added", message: "Some Movie was added", createdAt: new Date().toISOString() },
    ] as Record<string, unknown>[],
  },
}));

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to, ...rest }: { children: React.ReactNode; to: string; [k: string]: unknown }) => (
    <a href={to} {...rest}>{children}</a>
  ),
  useNavigate: () => navigateSpy,
}));

vi.mock("@evey/api", async (importOriginal) => ({
  // notificationLabel is a pure formatter - keep the REAL one so these tests
  // still assert the wording that reaches the screen.
  notificationLabel: (await importOriginal<typeof import("@evey/api")>()).notificationLabel,
  useNotificationServiceGetUnreadCount: () => ({ data: { count: 3 } }),
  useNotificationServiceListNotifications: () => ({
    data: { notifications: notificationsRef.current },
  }),
  useNotificationServiceMarkAllRead: () => ({ mutate: mockMutate }),
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
}

describe("NotificationBell", () => {
  beforeEach(() => {
    mockMutate.mockReset();
    navigateSpy.mockReset();
    notificationsRef.current = [
      { id: "1", title: "New movie added", message: "Some Movie was added", createdAt: new Date().toISOString() },
    ];
  });

  it("shows unread count badge", () => {
    render(<NotificationBell />, { wrapper: createWrapper() });
    expect(screen.getByText("3")).toBeInTheDocument();
  });

  it("opens dropdown on click", async () => {
    const user = userEvent.setup();
    render(<NotificationBell />, { wrapper: createWrapper() });
    await user.click(screen.getByRole("button", { name: "Notifications" }));
    expect(screen.getByText("Notifications")).toBeInTheDocument();
    expect(screen.getByText("New movie added")).toBeInTheDocument();
  });

  it("calls markAllRead on button click", async () => {
    const user = userEvent.setup();
    render(<NotificationBell />, { wrapper: createWrapper() });
    await user.click(screen.getByRole("button", { name: "Notifications" }));
    await user.click(screen.getByText("Mark all read"));
    expect(mockMutate).toHaveBeenCalledWith({ data: {} });
  });

  // The 2026-08-26 prod gap: media requests paged nobody. Now that the server
  // serves them in the regular feed, the bell must land the admin on the
  // moderation queue - these notifications carry no mediaItemId (the title is
  // not in the library yet), so without this mapping the entry is a dead row.
  it("links a media_request notification to the moderation queue", async () => {
    notificationsRef.current = [
      { id: "r1", type: "media_request", title: "New media request: Some Movie", createdAt: new Date().toISOString() },
    ];
    const user = userEvent.setup();
    render(<NotificationBell />, { wrapper: createWrapper() });
    await user.click(screen.getByRole("button", { name: "Notifications" }));

    await user.click(screen.getByText("New media request: Some Movie"));
    expect(navigateSpy).toHaveBeenCalledWith({ to: "/admin/media-requests" });
  });

  it("shows up to fifteen notifications in one scrollable box instead of five", async () => {
    notificationsRef.current = Array.from({ length: 20 }, (_, i) => ({
      id: `n${i}`,
      title: `Movie ${i} added`,
      message: "added",
      createdAt: new Date().toISOString(),
    }));
    render(<NotificationBell />, { wrapper: createWrapper() });
    await userEvent.click(screen.getByTestId("notification-bell"));
    const list = screen.getByTestId("notification-list");
    expect(list.querySelectorAll("button")).toHaveLength(15);
    // The list scrolls inside a capped box: the dropdown keeps its size
    // whatever the count.
    expect(list.className).toContain("max-h-96");
    expect(list.className).toContain("overflow-y-auto");
  });
});
