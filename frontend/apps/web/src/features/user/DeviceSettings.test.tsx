import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

const mockDevices = [
  { token: "tok_abc123xyz789", platform: "ios", createdAt: "2025-06-01T10:00:00Z" },
  { token: "tok_def456uvw012", platform: "android", createdAt: "2025-07-15T12:00:00Z" },
];

describe("DeviceSettings", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows loading state initially", async () => {
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockReturnValue(new Promise(() => {})); // never resolves

    const { DeviceSettings } = await import("./DeviceSettings");
    render(React.createElement(DeviceSettings), { wrapper: createWrapper() });

    expect(screen.getByText(/Loading/i)).toBeInTheDocument();
  });

  it("shows 'No registered devices' when device list is empty", async () => {
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockResolvedValue({ devices: [] });

    const { DeviceSettings } = await import("./DeviceSettings");
    render(React.createElement(DeviceSettings), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText(/No registered devices/i)).toBeInTheDocument();
    });
  });

  it("renders device list with platform labels and truncated tokens", async () => {
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockResolvedValue({ devices: mockDevices });

    const { DeviceSettings } = await import("./DeviceSettings");
    render(React.createElement(DeviceSettings), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(screen.getByText("iOS")).toBeInTheDocument();
      expect(screen.getByText("Android")).toBeInTheDocument();
    });
    // truncateToken: first 6 chars + ellipsis + last 6 chars
    // "tok_abc123xyz789" → "tok_ab…xyz789"
    expect(screen.getByText("tok_ab…xyz789")).toBeInTheDocument();
  });

  it("offers no revoke action - the list is read-only", async () => {
    // Native device registration (FCM-era) was removed server-side; the old
    // revoke button called an endpoint that no longer exists.
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockResolvedValue({ devices: mockDevices });

    const { DeviceSettings } = await import("./DeviceSettings");
    render(React.createElement(DeviceSettings), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getByText("iOS")).toBeInTheDocument());
    expect(screen.queryByRole("button", { name: "Revoke" })).not.toBeInTheDocument();
  });
});
