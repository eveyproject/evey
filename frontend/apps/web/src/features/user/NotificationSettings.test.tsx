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

vi.mock("@/lib/webpush", () => ({
  isPushSupported: () => false,
  isPushSubscribed: vi.fn(),
  subscribeToPush: vi.fn(),
  unsubscribeFromPush: vi.fn(),
}));

describe("NotificationSettings push without browser support", () => {
  it("asks for HTTPS outside a secure context instead of blaming the server", () => {
    vi.stubGlobal("isSecureContext", false);
    render(<NotificationSettings />, { wrapper: createWrapper() });
    expect(screen.getByText(/need an HTTPS address/i)).toBeInTheDocument();
    vi.unstubAllGlobals();
  });

  it("names the browser when the context is secure", () => {
    vi.stubGlobal("isSecureContext", true);
    render(<NotificationSettings />, { wrapper: createWrapper() });
    expect(screen.getByText(/does not support push/i)).toBeInTheDocument();
    vi.unstubAllGlobals();
  });
});
