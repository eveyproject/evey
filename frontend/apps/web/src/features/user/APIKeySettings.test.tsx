import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const toastSuccess = vi.fn();
const toastError = vi.fn();
const mockMutateAsync = vi.fn();

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useUserServiceListAPIKeys: vi.fn(() => ({ data: { apiKeys: [] }, isLoading: false })),
  useUserServiceCreateAPIKey: vi.fn(() => ({ mutateAsync: mockMutateAsync, isPending: false })),
  useUserServiceRevokeAPIKey: vi.fn(() => ({ mutateAsync: mockMutateAsync, isPending: false })),
  getUserServiceListAPIKeysQueryKey: vi.fn(() => ["user", "api-keys"]),
}));

vi.mock("@evey/store", () => ({
  toast: { success: toastSuccess, error: toastError },
}));

vi.mock("@tanstack/react-query", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-query")>();
  return { ...actual, useQueryClient: () => ({ invalidateQueries: vi.fn() }) };
});

vi.mock("@tanstack/react-router", () => ({
  Link: ({ to, children, ...props }: { to: string; children: React.ReactNode }) =>
    React.createElement("a", { href: to, ...props }, children),
}));

const mockKeys = [
  { id: "key1", name: "Home Assistant", key: "evey_abcdef1234", createdAt: "2025-01-01T00:00:00Z", lastUsedAt: "2025-06-15T12:00:00Z" },
  { id: "key2", name: "Backup Sync", key: "evey_xyz9876543", createdAt: "2025-03-10T00:00:00Z", lastUsedAt: undefined },
];

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

async function setupMock(keys: typeof mockKeys | [] = [], loading = false) {
  const api = await import("@evey/api");
  vi.mocked(api.useUserServiceListAPIKeys).mockReturnValue({ data: { apiKeys: keys as never }, isLoading: loading } as ReturnType<typeof api.useUserServiceListAPIKeys>);
}

describe("APIKeySettings", () => {
  beforeEach(() => { vi.clearAllMocks(); });

  it("shows loading state", async () => {
    await setupMock([], true);
    const { APIKeySettings } = await import("./APIKeySettings");
    render(React.createElement(APIKeySettings), { wrapper: createWrapper() });
    expect(screen.getByText(/Loading/i)).toBeInTheDocument();
  });

  it("shows empty state when no keys", async () => {
    await setupMock([]);
    const { APIKeySettings } = await import("./APIKeySettings");
    render(React.createElement(APIKeySettings), { wrapper: createWrapper() });
    expect(screen.getByText(/No API keys yet/i)).toBeInTheDocument();
  });

  it("renders key list with masked keys", async () => {
    await setupMock(mockKeys);
    const { APIKeySettings } = await import("./APIKeySettings");
    render(React.createElement(APIKeySettings), { wrapper: createWrapper() });
    expect(screen.getByText("Home Assistant")).toBeInTheDocument();
    expect(screen.getByText("Backup Sync")).toBeInTheDocument();
    expect(screen.getByText(/evey_••••••••1234/)).toBeInTheDocument();
  });

  it("New key button opens create modal", async () => {
    await setupMock([]);
    const { APIKeySettings } = await import("./APIKeySettings");
    render(React.createElement(APIKeySettings), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /New key/i }));
    await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());
    expect(screen.getByText("New API Key")).toBeInTheDocument();
  });

  it("empty name shows validation error", async () => {
    await setupMock([]);
    const { APIKeySettings } = await import("./APIKeySettings");
    render(React.createElement(APIKeySettings), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /New key/i }));
    await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: /^Create$/i }));
    await waitFor(() => expect(screen.getByText(/Name is required/i)).toBeInTheDocument());
  });

  it("Revoke button opens confirmation modal", async () => {
    await setupMock(mockKeys);
    const { APIKeySettings } = await import("./APIKeySettings");
    render(React.createElement(APIKeySettings), { wrapper: createWrapper() });
    await waitFor(() => expect(screen.getAllByRole("button", { name: /Revoke/i })).toHaveLength(2));
    fireEvent.click(screen.getAllByRole("button", { name: /Revoke/i })[0]);
    await waitFor(() => expect(screen.getByText(/Revoke API Key/i)).toBeInTheDocument());
  });

  it("Cancel in revoke modal does not call API", async () => {
    await setupMock(mockKeys);
    const { APIKeySettings } = await import("./APIKeySettings");
    render(React.createElement(APIKeySettings), { wrapper: createWrapper() });
    await waitFor(() => expect(screen.getAllByRole("button", { name: /Revoke/i })).toHaveLength(2));
    fireEvent.click(screen.getAllByRole("button", { name: /Revoke/i })[0]);
    await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: /Cancel/i }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(mockMutateAsync).not.toHaveBeenCalled();
  });
});
