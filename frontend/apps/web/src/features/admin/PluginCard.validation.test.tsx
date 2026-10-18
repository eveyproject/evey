import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { PluginCard } from "./PluginCard";

// A refused config field reads in the admin's language: the messages were
// English literals, shown untranslated whatever the locale.
const t = vi.hoisted(() => (k: string, o?: Record<string, unknown>) => (o && typeof o === "object" ? `${k}(${Object.values(o).join(",")})` : k));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t }) }));
vi.mock("@tanstack/react-router", () => ({
  Link: ({ children }: { children: React.ReactNode }) => <a>{children}</a>,
  useRouterState: () => ({ location: { pathname: "/" } }),
}));
const update = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useAdminPluginServiceGetPluginConfig: () => ({
    data: { configSchemaJson: JSON.stringify({ port: { type: "integer", min: 1 } }), values: { port: "5" } },
    isLoading: false,
  }),
  useAdminPluginServiceUpdatePluginConfig: () => ({ mutate: update, isPending: false }),
  useAdminPluginServiceGetPluginLogs: () => ({ data: undefined, isLoading: false }),
  eveyFetch: vi.fn().mockReturnValue(new Promise(() => {})),
}));
vi.mock("@evey/store", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/store")>()),
  toast: { success: vi.fn(), error: vi.fn() },
}));

describe("PluginCard config validation", () => {
  it("shows the translated bound error and does not save", () => {
    const qc = new QueryClient();
    render(
      <QueryClientProvider client={qc}>
        <PluginCard plugin={{ id: "p1", name: "P", version: "1", state: "PLUGIN_STATE_RUNNING", enabled: true }} onEnable={vi.fn()} onDisable={vi.fn()} onUninstall={vi.fn()} />
      </QueryClientProvider>,
    );
    fireEvent.click(screen.getByText("admin.plugin.config"));
    fireEvent.change(screen.getByLabelText("port"), { target: { value: "0" } });
    fireEvent.click(screen.getByText("common.save"));
    expect(screen.getByText("admin.plugins.validation.min(port,1)")).toBeTruthy();
    expect(update).not.toHaveBeenCalled();
  });
});
