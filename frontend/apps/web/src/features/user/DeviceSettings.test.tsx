import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const toastSuccess = vi.fn();
const toastError = vi.fn();

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
}));

vi.mock("@evey/store", () => ({
  toast: { success: toastSuccess, error: toastError },
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

  it("renders a Revoke button for each device", async () => {
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockResolvedValue({ devices: mockDevices });

    const { DeviceSettings } = await import("./DeviceSettings");
    render(React.createElement(DeviceSettings), { wrapper: createWrapper() });

    const buttons = await screen.findAllByRole("button", { name: "Revoke" });
    expect(buttons).toHaveLength(2);
  });

  it("clicking Revoke opens confirmation modal", async () => {
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockResolvedValue({ devices: mockDevices });

    const { DeviceSettings } = await import("./DeviceSettings");
    render(React.createElement(DeviceSettings), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getAllByRole("button", { name: "Revoke" })).toHaveLength(2));

    fireEvent.click(screen.getAllByRole("button", { name: "Revoke" })[0]);

    await waitFor(() => {
      expect(screen.getByRole("dialog")).toBeInTheDocument();
    });
    expect(screen.getByText(/Revoke Device/i)).toBeInTheDocument();
  });

  it("confirming revoke calls DELETE and shows success toast", async () => {
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch)
      .mockResolvedValueOnce({ devices: mockDevices }) // GET
      .mockResolvedValueOnce({}) // DELETE
      .mockResolvedValueOnce({ devices: [mockDevices[1]] }); // re-fetch

    const { DeviceSettings } = await import("./DeviceSettings");
    render(React.createElement(DeviceSettings), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getAllByRole("button", { name: "Revoke" })).toHaveLength(2));

    fireEvent.click(screen.getAllByRole("button", { name: "Revoke" })[0]);
    await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());

    // Click the Revoke confirm button inside the modal
    const modalRevokeButtons = screen.getAllByRole("button", { name: "Revoke" });
    // The last Revoke button in the dialog is the confirm button
    fireEvent.click(modalRevokeButtons[modalRevokeButtons.length - 1]);

    await waitFor(() => {
      expect(vi.mocked(eveyFetch)).toHaveBeenCalledWith(
        expect.objectContaining({ method: "DELETE", url: expect.stringContaining("/api/v1/push/devices/") }),
      );
    });
    await waitFor(() => {
      expect(toastSuccess).toHaveBeenCalledWith("Device revoked");
    });
  });

  it("Cancel button closes modal without revoking", async () => {
    const { eveyFetch } = await import("@evey/api");
    vi.mocked(eveyFetch).mockResolvedValue({ devices: mockDevices });

    const { DeviceSettings } = await import("./DeviceSettings");
    render(React.createElement(DeviceSettings), { wrapper: createWrapper() });

    await waitFor(() => expect(screen.getAllByRole("button", { name: "Revoke" })).toHaveLength(2));
    fireEvent.click(screen.getAllByRole("button", { name: "Revoke" })[0]);
    await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
  });
});
