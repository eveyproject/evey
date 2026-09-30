import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockEveyFetch = vi.fn();
const mockUseQuery = vi.fn();
const mockUseMutation = vi.fn();
const mockInvalidateQueries = vi.fn();

const mockUseRA = vi.fn();
const mockUseNet = vi.fn();

vi.mock("@evey/api", () => ({
  eveyFetch: (...args: unknown[]) => mockEveyFetch(...args),
  useRemoteAccessServiceGetRemoteAccess: () => mockUseRA(),
  useRemoteAccessServiceGetNetworkInfo: () => mockUseNet(),
  remoteAccessServiceUpdateRemoteAccess: vi.fn(),
  getRemoteAccessServiceGetRemoteAccessQueryKey: () => ["remote-access"],
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
    mockUseRA.mockReturnValue({ data: mockConfig, isLoading: false });
    mockUseNet.mockReturnValue({ data: undefined });
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });
    expect(screen.getByText("Remote Access")).toBeInTheDocument();
  });

  it("shows Disabled badge when remote access is disabled", async () => {
    mockUseRA.mockReturnValue({ data: { ...mockConfig, enabled: false }, isLoading: false });
    mockUseNet.mockReturnValue({ data: undefined });
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });
    expect(screen.getByText("Disabled")).toBeInTheDocument();
  });

  it("shows Enabled badge when remote access is enabled", async () => {
    mockUseRA.mockReturnValue({ data: { ...mockConfig, enabled: true }, isLoading: false });
    mockUseNet.mockReturnValue({ data: undefined });
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });
    expect(screen.getByText("Enabled")).toBeInTheDocument();
  });

  it("Save button is disabled when nothing changed", async () => {
    mockUseRA.mockReturnValue({ data: mockConfig, isLoading: false });
    mockUseNet.mockReturnValue({ data: undefined });
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });
    const saveBtn = screen.getByRole("button", { name: /^Save$/i });
    expect(saveBtn).toBeDisabled();
  });

  it("toggling switch enables Save button", async () => {
    mockUseRA.mockReturnValue({ data: mockConfig, isLoading: false });
    mockUseNet.mockReturnValue({ data: undefined });
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });
    const toggle = screen.getByRole("switch");
    fireEvent.click(toggle);
    await waitFor(() => expect(screen.getByRole("button", { name: /^Save$/i })).not.toBeDisabled());
  });

  it("saves the trusted proxy list, and an emptied field hands it back to the environment", async () => {
    const mutate = vi.fn();
    mockUseMutation.mockReturnValue({ mutate, isPending: false });
    mockUseRA.mockReturnValue({
      data: { ...mockConfig, trustedProxies: ["172.18.0.0/16"], trustedProxiesFromPanel: true },
      isLoading: false,
    });
    mockUseNet.mockReturnValue({ data: undefined });
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });
    const input = screen.getByDisplayValue("172.18.0.0/16");
    expect(screen.getByText(/Current list: saved here\./)).toBeInTheDocument();

    fireEvent.change(input, { target: { value: " 172.18.0.0/16 ,, 10.0.0.2 " } });
    fireEvent.click(screen.getByRole("button", { name: /^Save$/i }));
    expect(mutate).toHaveBeenLastCalledWith(
      expect.objectContaining({ trustedProxies: ["172.18.0.0/16", "10.0.0.2"], clearTrustedProxies: false }),
    );

    fireEvent.change(input, { target: { value: "" } });
    fireEvent.click(screen.getByRole("button", { name: /^Save$/i }));
    expect(mutate).toHaveBeenLastCalledWith(
      expect.objectContaining({ trustedProxies: [], clearTrustedProxies: true }),
    );
  });

  it("shows spinner when loading", async () => {
    mockUseRA.mockReturnValue({ data: undefined, isLoading: true });
    mockUseNet.mockReturnValue({ data: undefined });
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });
    expect(document.querySelector('[class*="animate-spin"]') ?? screen.queryByText(/loading/i)).toBeDefined();
  });

  it("renders network interfaces table when data is present", async () => {
    mockUseRA.mockReturnValue({ data: mockConfig, isLoading: false });
    mockUseNet.mockReturnValue({ data: { interfaces: [{ name: "eth0", address: "192.168.1.100" }] }, isLoading: false });
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });
    await waitFor(() => expect(screen.getByText("eth0")).toBeInTheDocument());
    expect(screen.getByText("192.168.1.100")).toBeInTheDocument();
  });

  it("shows the lockout risk alert when the server is published but remote access is disabled", async () => {
    mockUseRA.mockReturnValue({
      data: { ...mockConfig, enabled: false, published: true, lockoutRisk: true },
      isLoading: false,
    });
    mockUseNet.mockReturnValue({ data: undefined });
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });
    expect(screen.getByRole("alert")).toBeInTheDocument();
    expect(screen.getByText("Every remote user is locked out")).toBeInTheDocument();
  });

  it("does not show the lockout risk alert when remote access is enabled", async () => {
    mockUseRA.mockReturnValue({
      data: { ...mockConfig, enabled: true, published: true, lockoutRisk: false },
      isLoading: false,
    });
    mockUseNet.mockReturnValue({ data: undefined });
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});
