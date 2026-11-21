import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { PluginCard } from "./PluginCard";

// A plugin that mounts admin.plugin.config draws its own config panel in
// place of the generated form; one that does not keeps the form.
const t = vi.hoisted(() => (k: string) => k);
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t }) }));
vi.mock("@tanstack/react-router", () => ({
  Link: ({ children }: { children: React.ReactNode }) => <a>{children}</a>,
  useRouterState: () => ({ location: { pathname: "/" } }),
}));
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useAdminPluginServiceGetPluginConfig: () => ({
    data: { configSchemaJson: JSON.stringify({ city: { type: "string", label: "City" } }), values: { city: "Paris" } },
    isLoading: false,
  }),
  useAdminPluginServiceUpdatePluginConfig: () => ({ mutate: vi.fn(), isPending: false }),
  useAdminPluginServiceGetPluginLogs: () => ({ data: undefined, isLoading: false }),
  eveyFetch: vi.fn().mockReturnValue(new Promise(() => {})),
}));
vi.mock("@evey/store", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/store")>()),
  toast: { success: vi.fn(), error: vi.fn() },
}));
const entries = vi.hoisted(() => ({ current: [] as { pluginId: string; name: string; entryPath: string; externalFetch: string[] }[] }));
vi.mock("@/features/plugins/usePluginSlot", () => ({
  usePluginSlotEntries: (slot: string) => (slot === "admin.plugin.config" ? entries.current : []),
}));
vi.mock("@/features/plugins/PluginIframe", () => ({
  PluginIframe: (p: { pluginId: string; slot: string; src: string }) => (
    <div data-testid="own-panel" data-plugin={p.pluginId} data-slot={p.slot} data-src={p.src} />
  ),
}));

function renderCard(id: string) {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <PluginCard plugin={{ id, name: "weather", version: "1", state: "PLUGIN_STATE_RUNNING", enabled: true }} onEnable={vi.fn()} onDisable={vi.fn()} onUninstall={vi.fn()} />
    </QueryClientProvider>,
  );
  fireEvent.click(screen.getByText("admin.plugin.config"));
}

describe("PluginCard own config panel", () => {
  it("mounts the plugin's admin.plugin.config module instead of the generated form", () => {
    entries.current = [{ pluginId: "p1", name: "weather", entryPath: "admin.js", externalFetch: [] }];
    renderCard("p1");
    const panel = screen.getByTestId("own-panel");
    expect(panel.getAttribute("data-slot")).toBe("admin.plugin.config");
    expect(panel.getAttribute("data-src")).toBe("admin.js");
    expect(screen.queryByLabelText("city")).toBeNull();
  });

  it("keeps the generated form when only another plugin has its own panel", () => {
    entries.current = [{ pluginId: "p2", name: "other", entryPath: "admin.js", externalFetch: [] }];
    renderCard("p1");
    expect(screen.queryByTestId("own-panel")).toBeNull();
    expect(screen.getByLabelText("city")).toBeTruthy();
  });
});
