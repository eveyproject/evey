import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { NotificationBell } from "./NotificationBell";

const mockMutate = vi.fn();

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to, ...rest }: { children: React.ReactNode; to: string; [k: string]: unknown }) => (
    <a href={to} {...rest}>{children}</a>
  ),
  useNavigate: () => vi.fn(),
}));

vi.mock("@evey/api", async (importOriginal) => ({
  // notificationLabel is a pure formatter - keep the REAL one so these tests
  // still assert the wording that reaches the screen.
  notificationLabel: (await importOriginal<typeof import("@evey/api")>()).notificationLabel,
  useNotificationServiceGetUnreadCount: () => ({ data: { count: 3 } }),
  useNotificationServiceListNotifications: () => ({
    data: {
      notifications: [
        { id: "1", title: "New movie added", message: "Some Movie was added", createdAt: new Date().toISOString() },
      ],
    },
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
});
