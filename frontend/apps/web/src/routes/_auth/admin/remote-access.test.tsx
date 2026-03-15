import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockEveyFetch = vi.fn();
const mockUseQuery = vi.fn();
const mockUseMutation = vi.fn();
const mockInvalidateQueries = vi.fn();

vi.mock("@evey/api/src/fetcher", () => ({
  eveyFetch: (...args: unknown[]) => mockEveyFetch(...args),
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (opts: { component: React.ComponentType }) => opts,
}));

vi.mock("@tanstack/react-query", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-query")>();
  return {
    ...actual,
    useQuery: (...args: unknown[]) => mockUseQuery(...args),
    useMutation: (...args: unknown[]) => mockUseMutation(...args),
    useQueryClient: () => ({ invalidateQueries: mockInvalidateQueries }),
  };
});

const mockConfig = {
  enabled: false,
  externalUrl: "https://evey.example.com",
  lanNetworks: ["192.168.0.0/16"],
  maxRemoteBitrateKbps: 0,
};

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

async function getPage() {
  const mod = await import("./remote-access.lazy");
  return (mod.Route as unknown as { component: React.ComponentType }).component;
}

describe("RemoteAccessPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseMutation.mockReturnValue({ mutate: vi.fn(), isPending: false });
  });

  it("renders Remote Access heading", async () => {
    mockUseQuery.mockReturnValue({ data: mockConfig, isLoading: false });
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });
    expect(screen.getByText("Remote Access")).toBeInTheDocument();
  });

  it("shows Disabled badge when remote access is disabled", async () => {
    mockUseQuery.mockReturnValue({ data: { ...mockConfig, enabled: false }, isLoading: false });
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });
    expect(screen.getByText("Disabled")).toBeInTheDocument();
  });

  it("shows Enabled badge when remote access is enabled", async () => {
    mockUseQuery.mockReturnValue({ data: { ...mockConfig, enabled: true }, isLoading: false });
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });
    expect(screen.getByText("Enabled")).toBeInTheDocument();
  });

  it("Save button is disabled when nothing changed", async () => {
    mockUseQuery.mockReturnValue({ data: mockConfig, isLoading: false });
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });
    const saveBtn = screen.getByRole("button", { name: /^Save$/i });
    expect(saveBtn).toBeDisabled();
  });

  it("toggling switch enables Save button", async () => {
    mockUseQuery.mockReturnValue({ data: mockConfig, isLoading: false });
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });
    const toggle = screen.getByRole("switch");
    fireEvent.click(toggle);
    await waitFor(() => expect(screen.getByRole("button", { name: /^Save$/i })).not.toBeDisabled());
  });

  it("shows spinner when loading", async () => {
    mockUseQuery.mockReturnValue({ data: undefined, isLoading: true });
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });
    expect(document.querySelector('[class*="animate-spin"]') ?? screen.queryByText(/loading/i)).toBeDefined();
  });

  it("renders network interfaces table when data is present", async () => {
    mockUseQuery.mockImplementation((opts: { queryKey: unknown[] }) => {
      const key = String(opts?.queryKey?.[0]);
      if (key.includes("network-info")) {
        return { data: { interfaces: [{ name: "eth0", address: "192.168.1.100" }] }, isLoading: false };
      }
      return { data: mockConfig, isLoading: false };
    });
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });
    await waitFor(() => expect(screen.getByText("eth0")).toBeInTheDocument());
    expect(screen.getByText("192.168.1.100")).toBeInTheDocument();
  });
});
