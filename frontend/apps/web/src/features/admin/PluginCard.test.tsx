import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { PluginCard } from "./PluginCard";

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to, className }: { children: React.ReactNode; to: string; className?: string }) => (
    <a href={to} className={className}>{children}</a>
  ),
  useRouterState: () => ({ location: { pathname: "/" } }),
}));

vi.mock("@evey/api", () => ({
  useAdminPluginServiceGetPluginConfig: () => ({ data: undefined, isLoading: false }),
  useAdminPluginServiceUpdatePluginConfig: () => ({ mutate: vi.fn(), isPending: false }),
  useAdminPluginServiceGetPluginLogs: () => ({ data: undefined, isLoading: false }),
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

function Wrapper({ children }: { children: React.ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
}

const basePlugin = {
  id: "p1",
  name: "My Plugin",
  version: "1.0.0",
  language: "go",
  description: "A test plugin",
  state: "PLUGIN_STATE_RUNNING" as const,
  enabled: true,
};

describe("PluginCard", () => {
  it("renders plugin name and version", () => {
    render(
      <PluginCard
        plugin={basePlugin as never}
        onEnable={vi.fn()}
        onDisable={vi.fn()}
        onUninstall={vi.fn()}
      />,
      { wrapper: Wrapper },
    );
    expect(screen.getByText("My Plugin")).toBeInTheDocument();
    expect(screen.getByText("v1.0.0 - go")).toBeInTheDocument();
  });

  it("renders description when present", () => {
    render(
      <PluginCard
        plugin={basePlugin as never}
        onEnable={vi.fn()}
        onDisable={vi.fn()}
        onUninstall={vi.fn()}
      />,
      { wrapper: Wrapper },
    );
    expect(screen.getByText("A test plugin")).toBeInTheDocument();
  });

  it("shows Disable button when enabled", () => {
    render(
      <PluginCard
        plugin={basePlugin as never}
        onEnable={vi.fn()}
        onDisable={vi.fn()}
        onUninstall={vi.fn()}
      />,
      { wrapper: Wrapper },
    );
    expect(screen.getByRole("button", { name: "Disable" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Enable" })).not.toBeInTheDocument();
  });

  it("shows Enable button when disabled", () => {
    render(
      <PluginCard
        plugin={{ ...basePlugin, enabled: false } as never}
        onEnable={vi.fn()}
        onDisable={vi.fn()}
        onUninstall={vi.fn()}
      />,
      { wrapper: Wrapper },
    );
    expect(screen.getByRole("button", { name: "Enable" })).toBeInTheDocument();
  });

  it("calls onDisable when Disable clicked", async () => {
    const onDisable = vi.fn();
    render(
      <PluginCard
        plugin={basePlugin as never}
        onEnable={vi.fn()}
        onDisable={onDisable}
        onUninstall={vi.fn()}
      />,
      { wrapper: Wrapper },
    );
    await userEvent.click(screen.getByRole("button", { name: "Disable" }));
    expect(onDisable).toHaveBeenCalledOnce();
  });

  it("calls onEnable when Enable clicked", async () => {
    const onEnable = vi.fn();
    render(
      <PluginCard
        plugin={{ ...basePlugin, enabled: false } as never}
        onEnable={onEnable}
        onDisable={vi.fn()}
        onUninstall={vi.fn()}
      />,
      { wrapper: Wrapper },
    );
    await userEvent.click(screen.getByRole("button", { name: "Enable" }));
    expect(onEnable).toHaveBeenCalledOnce();
  });

  it("calls onUninstall when Uninstall clicked", async () => {
    const onUninstall = vi.fn();
    render(
      <PluginCard
        plugin={basePlugin as never}
        onEnable={vi.fn()}
        onDisable={vi.fn()}
        onUninstall={onUninstall}
      />,
      { wrapper: Wrapper },
    );
    await userEvent.click(screen.getByRole("button", { name: "Uninstall" }));
    expect(onUninstall).toHaveBeenCalledOnce();
  });

  it("shows Running badge for running state", () => {
    render(
      <PluginCard
        plugin={basePlugin as never}
        onEnable={vi.fn()}
        onDisable={vi.fn()}
        onUninstall={vi.fn()}
      />,
      { wrapper: Wrapper },
    );
    expect(screen.getByText("Running")).toBeInTheDocument();
  });

  it("shows Error badge for error state", () => {
    render(
      <PluginCard
        plugin={{ ...basePlugin, state: "PLUGIN_STATE_ERROR" } as never}
        onEnable={vi.fn()}
        onDisable={vi.fn()}
        onUninstall={vi.fn()}
      />,
      { wrapper: Wrapper },
    );
    expect(screen.getByText("Error")).toBeInTheDocument();
  });
});
