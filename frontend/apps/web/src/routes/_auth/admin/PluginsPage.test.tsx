import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (def: { component: React.ComponentType }) => def,
  Link: ({ children, to }: { children: React.ReactNode; to?: string }) =>
    React.createElement("a", { href: to }, children),
}));

const mockEnablePlugin = vi.fn();
const mockDisablePlugin = vi.fn();
const mockUninstallPlugin = vi.fn();
const mockInstallPlugin = vi.fn();
// Answers the page's runtimes query by default; the URL-preview tests
// override it per URL.
const mockEveyFetch = vi.fn().mockResolvedValue({ runtimes: [] });

vi.mock("@evey/api", () => ({
  eveyFetch: (...args: unknown[]) => mockEveyFetch(...args),
  eveyFetchRaw: vi.fn(),
  adminPluginServiceInstallPlugin: vi.fn(),
  apiErrorMessage: (err: Error, _t: unknown, fallback: string) => err?.message || fallback,
  NO_TIMEOUT: { timeout: 0 },
  useAdminPluginServiceListPlugins: vi.fn(() => ({ data: undefined, isLoading: true })),
  useAdminPluginServiceEnablePlugin: vi.fn(() => ({ mutate: mockEnablePlugin, isPending: false })),
  useAdminPluginServiceDisablePlugin: vi.fn(() => ({ mutate: mockDisablePlugin, isPending: false })),
  useAdminPluginServiceUninstallPlugin: vi.fn(() => ({ mutate: mockUninstallPlugin, isPending: false })),
  useAdminPluginServiceInstallPlugin: vi.fn(() => ({ mutate: mockInstallPlugin, isPending: false })),
  useAdminPluginServiceGetPluginConfig: vi.fn(() => ({ data: { values: {} }, isLoading: false })),
  useAdminPluginServiceUpdatePluginConfig: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
  useAdminPluginServiceGetPluginLogs: vi.fn(() => ({ data: undefined, isLoading: false })),
  useAdminPluginServiceGetPluginResourceLimits: vi.fn(() => ({ data: undefined, isLoading: false })),
  useAdminPluginServiceUpdatePluginResourceLimits: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
  getAdminPluginServiceGetPluginResourceLimitsQueryKey: vi.fn(() => ["plugin-resource-limits"]),
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
  useDemoStore: (sel: (s: { demoLocked: string[] }) => unknown) => sel({ demoLocked: [] }),
  useAuthStore: (selector: (s: { accessToken: string }) => unknown) => selector({ accessToken: "test-token" }),
}));

vi.mock("@tanstack/react-query", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-query")>();
  return { ...actual, useQueryClient: () => ({ invalidateQueries: vi.fn() }) };
});

vi.mock("@/components/ui/Spinner", () => ({
  Spinner: () => React.createElement("div", { "data-testid": "spinner" }),
}));

vi.mock("@/features/admin/PluginSecurityDashboard", () => ({
  PluginSecurityDashboard: () => null,
}));

vi.mock("@/components/ui/Modal", () => ({
  Modal: ({ open, children, title }: { open: boolean; children: React.ReactNode; title?: string }) =>
    open ? React.createElement("div", { role: "dialog", "aria-label": title }, children) : null,
}));

vi.mock("@/components/ui/Button", () => ({
  Button: ({ children, onClick, type, disabled, loading }: { children: React.ReactNode; onClick?: () => void; type?: string; disabled?: boolean; loading?: boolean; variant?: string }) =>
    React.createElement("button", { onClick, type, disabled: disabled || loading }, children),
}));

vi.mock("@/components/ui/Input", () => ({
  Input: ({ label, value, onChange, placeholder }: { label?: string; value: string; onChange: (e: React.ChangeEvent<HTMLInputElement>) => void; placeholder?: string; required?: boolean }) =>
    React.createElement("input", { "aria-label": label, value, onChange, placeholder }),
}));

vi.mock("@/components/ui/Card", () => ({
  Card: ({ children }: { children: React.ReactNode }) => React.createElement("div", { "data-testid": "card" }, children),
}));

vi.mock("@/components/ui/Badge", () => ({
  Badge: ({ children }: { children: React.ReactNode }) => React.createElement("span", null, children),
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

const mockPlugins = [
  { id: "p1", name: "Metadata Fetcher", version: "1.0.0", language: "Go", state: "PLUGIN_STATE_RUNNING", enabled: true, description: "Fetches metadata" },
  { id: "p2", name: "Subtitle Downloader", version: "0.5.0", language: "Python", state: "PLUGIN_STATE_STOPPED", enabled: false },
];

describe("PluginsPage", () => {
  beforeEach(() => { vi.clearAllMocks(); });

  it("shows spinner while loading", async () => {
    // Set, not inherited: clearAllMocks keeps the return value an earlier test
    // gave the mock, so in a shuffled run this test saw a loaded list.
    const { useAdminPluginServiceListPlugins } = await import("@evey/api");
    vi.mocked(useAdminPluginServiceListPlugins).mockReturnValue({ data: undefined, isLoading: true } as ReturnType<typeof useAdminPluginServiceListPlugins>);
    const mod = await import("./plugins.lazy");
    const PluginsPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(PluginsPage), { wrapper: createWrapper() });
    expect(screen.getByTestId("spinner")).toBeInTheDocument();
  });

  it("shows empty state when no plugins installed", async () => {
    const { useAdminPluginServiceListPlugins } = await import("@evey/api");
    vi.mocked(useAdminPluginServiceListPlugins).mockReturnValue({ data: { plugins: [] }, isLoading: false } as ReturnType<typeof useAdminPluginServiceListPlugins>);
    const mod = await import("./plugins.lazy");
    const PluginsPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(PluginsPage), { wrapper: createWrapper() });
    expect(screen.getByText("No plugins installed.")).toBeInTheDocument();
  });

  it("renders plugin names", async () => {
    const { useAdminPluginServiceListPlugins } = await import("@evey/api");
    vi.mocked(useAdminPluginServiceListPlugins).mockReturnValue({ data: { plugins: mockPlugins as never }, isLoading: false } as ReturnType<typeof useAdminPluginServiceListPlugins>);
    const mod = await import("./plugins.lazy");
    const PluginsPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(PluginsPage), { wrapper: createWrapper() });
    expect(screen.getByText("Metadata Fetcher")).toBeInTheDocument();
    expect(screen.getByText("Subtitle Downloader")).toBeInTheDocument();
  });

  it("Install Plugin button opens modal", async () => {
    const { useAdminPluginServiceListPlugins } = await import("@evey/api");
    vi.mocked(useAdminPluginServiceListPlugins).mockReturnValue({ data: { plugins: [] }, isLoading: false } as ReturnType<typeof useAdminPluginServiceListPlugins>);
    const mod = await import("./plugins.lazy");
    const PluginsPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(PluginsPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /Install Plugin/i }));
    await waitFor(() => expect(screen.getByRole("dialog", { name: /Install Plugin/i })).toBeInTheDocument());
  });

  it("plugin description is rendered", async () => {
    const { useAdminPluginServiceListPlugins } = await import("@evey/api");
    vi.mocked(useAdminPluginServiceListPlugins).mockReturnValue({ data: { plugins: mockPlugins as never }, isLoading: false } as ReturnType<typeof useAdminPluginServiceListPlugins>);
    const mod = await import("./plugins.lazy");
    const PluginsPage = (mod.Route as unknown as { component: React.ComponentType }).component;
    render(React.createElement(PluginsPage), { wrapper: createWrapper() });
    expect(screen.getByText("Fetches metadata")).toBeInTheDocument();
  });

  // Install-from-URL used to install blind ("URL installs skip the preview
  // step"): the static analysis WARNINGs were visible only once the plugin was
  // already running. The URL form now previews the source first.
  describe("install from URL", () => {
    async function openUrlForm() {
      const { useAdminPluginServiceListPlugins } = await import("@evey/api");
      vi.mocked(useAdminPluginServiceListPlugins).mockReturnValue({ data: { plugins: [] }, isLoading: false } as ReturnType<typeof useAdminPluginServiceListPlugins>);
      const mod = await import("./plugins.lazy");
      const PluginsPage = (mod.Route as unknown as { component: React.ComponentType }).component;
      render(React.createElement(PluginsPage), { wrapper: createWrapper() });
      fireEvent.click(screen.getByRole("button", { name: /Install Plugin/i }));
      fireEvent.click(screen.getByRole("button", { name: /From URL/i }));
      fireEvent.change(screen.getByLabelText("Source URL"), { target: { value: " https://example.com/p.zip#sha256:abc " } });
    }

    // The page also fetches the available runtimes through eveyFetch: answer
    // by URL so the preview stub is not consumed by that query.
    function stubPreview(impl: () => Promise<unknown>) {
      mockEveyFetch.mockImplementation((req: { url: string }) =>
        req.url === "/api/v1/admin/plugins/preview" ? impl() : Promise.resolve({ runtimes: [] }),
      );
    }

    it("previews the source and installs only after the admin confirms", async () => {
      stubPreview(() => Promise.resolve({
        name: "remote-plugin",
        version: "1.2.0",
        language: "Go",
        permissions: ["api.library"],
        capabilities: [],
        analysis: { safe: true, total_files_scanned: 3, issues: ["[WARNING] overreach: docker invoked at runtime"], urls: [] },
      }));
      await openUrlForm();
      expect(screen.queryByText(/skip the preview/i)).not.toBeInTheDocument();

      fireEvent.click(screen.getByRole("button", { name: /Preview & Install/i }));
      await waitFor(() => expect(screen.getByText("remote-plugin")).toBeInTheDocument());
      expect(mockEveyFetch).toHaveBeenCalledWith({
        url: "/api/v1/admin/plugins/preview",
        method: "POST",
        data: { source: "https://example.com/p.zip#sha256:abc" },
      });
      expect(screen.getByText("[WARNING] overreach: docker invoked at runtime")).toBeInTheDocument();
      expect(mockInstallPlugin).not.toHaveBeenCalled();

      fireEvent.click(screen.getByRole("button", { name: /Install Anyway/i }));
      expect(mockInstallPlugin).toHaveBeenCalledWith(
        { data: { source: "https://example.com/p.zip#sha256:abc", name: "" } },
        expect.anything(),
      );
    });

    it("a failed preview keeps the form and says why", async () => {
      stubPreview(() => Promise.reject(new Error("plugin preview failed: resolve plugin source: HTTP 404")));
      await openUrlForm();
      fireEvent.click(screen.getByRole("button", { name: /Preview & Install/i }));
      await waitFor(() => expect(screen.getByText(/HTTP 404/)).toBeInTheDocument());
      expect(screen.getByLabelText("Source URL")).toBeInTheDocument();
      expect(mockInstallPlugin).not.toHaveBeenCalled();
    });
  });
});
