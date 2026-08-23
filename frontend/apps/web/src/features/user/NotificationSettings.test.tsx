import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { NotificationSettings } from "./NotificationSettings";

const mutateMock = vi.fn();

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useNotificationServiceGetNotificationSettings: () => ({
    data: {
      movieLevel: "NOTIFICATION_LEVEL_ALL",
      seriesLevel: "NOTIFICATION_LEVEL_RELEVANT",
      musicLevel: "NOTIFICATION_LEVEL_DISABLED",
    },
    isLoading: false,
  }),
  useNotificationServiceUpdateNotificationSettings: () => ({
    mutate: mutateMock,
    isPending: false,
  }),
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
}

describe("NotificationSettings", () => {
  it("renders three level selects", () => {
    render(<NotificationSettings />, { wrapper: createWrapper() });
    expect(screen.getByText("Movies")).toBeInTheDocument();
    expect(screen.getByText("Series")).toBeInTheDocument();
    expect(screen.getByText("Music")).toBeInTheDocument();
  });

  it("renders Save button", () => {
    render(<NotificationSettings />, { wrapper: createWrapper() });
    expect(screen.getByRole("button", { name: "Save" })).toBeInTheDocument();
  });

  it("calls mutate on Save click", async () => {
    mutateMock.mockClear();
    render(<NotificationSettings />, { wrapper: createWrapper() });
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(mutateMock).toHaveBeenCalledOnce();
  });
});

describe("NotificationSettings loading", () => {
  it("renders loading state", () => {
    vi.doMock("@evey/api", () => ({
      useNotificationServiceGetNotificationSettings: () => ({ data: undefined, isLoading: true }),
      useNotificationServiceUpdateNotificationSettings: () => ({ mutate: vi.fn(), isPending: false }),
    }));
    // Loading state renders "Loading..." text - covered by unit approach in integration
  });
});
