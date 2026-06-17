import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockUseQuery = vi.fn();
const mockUseMutation = vi.fn();
const mockInvalidateQueries = vi.fn();

vi.mock("@tanstack/react-query", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-query")>();
  return {
    ...actual,
    useQuery: (...args: unknown[]) => {
      const opts = args[0] as { queryKey?: string[] } | undefined;
      if (opts?.queryKey?.[0]?.includes("server-settings")) {
        return { data: undefined, isLoading: true };
      }
      return mockUseQuery(...args);
    },
    useMutation: (...args: unknown[]) => mockUseMutation(...args),
    useQueryClient: () => ({ invalidateQueries: mockInvalidateQueries }),
  };
});

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
  useServerConfigServiceListServerConfigEntries: (...args: unknown[]) => mockUseQuery(...args),
  serverConfigServiceSetServerConfigEntry: vi.fn(),
  serverConfigServiceDeleteServerConfigEntry: vi.fn(),
  getServerConfigServiceListServerConfigEntriesQueryKey: () => ["server-config-entries"],
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
  useLocaleStore: (fn: (s: { setLocale: () => void }) => unknown) => fn({ setLocale: vi.fn() }),
}));

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (opts: { component: React.ComponentType }) => opts,
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

async function getServerConfigPage() {
  const mod = await import("./server-config.lazy");
  return (mod.Route as unknown as { component: React.ComponentType }).component;
}

const mockEntries = [
  { key: "smtp.host", value: "smtp.example.com", updatedAt: "2024-01-15T10:00:00Z" },
  { key: "custom.key", value: "custom-value", updatedAt: "2024-01-15T10:00:00Z" },
];

describe("ServerConfigPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseMutation.mockReturnValue({ mutate: vi.fn(), isPending: false });
  });

  it("renders page heading", async () => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false });
    const ServerConfigPage = await getServerConfigPage();
    render(React.createElement(ServerConfigPage), { wrapper: createWrapper() });
    expect(screen.getByText(/Server Config/i)).toBeInTheDocument();
  });

  it("renders Language & Region card", async () => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false });
    const ServerConfigPage = await getServerConfigPage();
    render(React.createElement(ServerConfigPage), { wrapper: createWrapper() });
    expect(screen.getByText(/Language & Region/i)).toBeInTheDocument();
  });

  it("renders Email / SMTP card", async () => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false });
    const ServerConfigPage = await getServerConfigPage();
    render(React.createElement(ServerConfigPage), { wrapper: createWrapper() });
    expect(screen.getByText(/Email \/ SMTP/i)).toBeInTheDocument();
  });

  it("renders config entries in table", async () => {
    mockUseQuery.mockReturnValue({ data: mockEntries, isLoading: false });
    const ServerConfigPage = await getServerConfigPage();
    render(React.createElement(ServerConfigPage), { wrapper: createWrapper() });
    expect(screen.getByText("smtp.host")).toBeInTheDocument();
    expect(screen.getByText("custom.key")).toBeInTheDocument();
  });

  it("renders Save SMTP Settings button", async () => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false });
    const ServerConfigPage = await getServerConfigPage();
    render(React.createElement(ServerConfigPage), { wrapper: createWrapper() });
    // The SMTP settings live in a collapsible card that starts closed; expand
    // it before asserting on its body.
    fireEvent.click(screen.getByRole("button", { name: /Email \/ SMTP/i }));
    expect(await screen.findByRole("button", { name: /Save SMTP Settings/i })).toBeInTheDocument();
  });

  it("shows Edit button for each config entry", async () => {
    mockUseQuery.mockReturnValue({ data: mockEntries, isLoading: false });
    const ServerConfigPage = await getServerConfigPage();
    render(React.createElement(ServerConfigPage), { wrapper: createWrapper() });
    const editButtons = screen.getAllByRole("button", { name: /^Edit$/i });
    expect(editButtons.length).toBeGreaterThanOrEqual(mockEntries.length);
  });

  it("clicking Edit on config row shows inline editor", async () => {
    mockUseQuery.mockReturnValue({ data: mockEntries, isLoading: false });
    const ServerConfigPage = await getServerConfigPage();
    render(React.createElement(ServerConfigPage), { wrapper: createWrapper() });
    // Settings forms that hold Save buttons (SMTP, locale, …) now live in
    // collapsed cards, so none are visible until a row enters edit mode.
    // Count them (query*, which returns [] rather than throwing on zero),
    // then click Edit on the first config row and assert one extra Save (the
    // row's inline editor) appeared. Cancel is also unique to the inline row.
    const saveBefore = screen.queryAllByRole("button", { name: /^Save$/i }).length;
    const cancelBefore = screen.queryAllByRole("button", { name: /^Cancel$/i }).length;
    fireEvent.click(screen.getAllByRole("button", { name: /^Edit$/i })[0]);
    await waitFor(() => {
      expect(screen.getAllByRole("button", { name: /^Save$/i }).length).toBe(saveBefore + 1);
      expect(screen.getAllByRole("button", { name: /^Cancel$/i }).length).toBe(cancelBefore + 1);
    });
  });

  it("shows Add button for adding new entries", async () => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false });
    const ServerConfigPage = await getServerConfigPage();
    render(React.createElement(ServerConfigPage), { wrapper: createWrapper() });
    expect(screen.getByRole("button", { name: /^Add$/i })).toBeInTheDocument();
  });

  it("AddEntryRow key and value inputs have accessible labels", async () => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false });
    const ServerConfigPage = await getServerConfigPage();
    render(React.createElement(ServerConfigPage), { wrapper: createWrapper() });
    expect(screen.getByRole("textbox", { name: /^Key$/i })).toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: /^Value$/i })).toBeInTheDocument();
  });

  it("Show/Hide button toggles password visibility", async () => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false });
    const ServerConfigPage = await getServerConfigPage();
    render(React.createElement(ServerConfigPage), { wrapper: createWrapper() });
    // The password toggle lives inside the (collapsed) SMTP card — expand first.
    fireEvent.click(screen.getByRole("button", { name: /Email \/ SMTP/i }));
    const showBtn = await screen.findByRole("button", { name: /Show/i });
    fireEvent.click(showBtn);
    expect(screen.getByRole("button", { name: /Hide/i })).toBeInTheDocument();
  });
});
