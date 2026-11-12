import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { PluginAutoUpdate } from "./PluginAutoUpdate";

const follow = vi.fn();
const check = vi.fn();

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useAdminPluginServiceSetPluginUpdateFollow: () => ({ mutate: follow, isPending: false }),
  useAdminPluginServiceCheckPluginUpdate: () => ({ mutate: check, isPending: false }),
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
  useDemoStore: (sel: (s: { demoLocked: string[] }) => unknown) => sel({ demoLocked: [] }),
}));

function Wrapper({ children }: { children: React.ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
}

const repo = "https://github.com/eveyproject/evey-plugin-weather";

beforeEach(() => {
  follow.mockReset();
  check.mockReset();
});

describe("PluginAutoUpdate", () => {
  it("shows the followed repository with automatic updates on by default", () => {
    render(<PluginAutoUpdate plugin={{ id: "p1", updateSource: repo }} onReview={vi.fn()} />, { wrapper: Wrapper });
    expect(screen.getByText("github.com/eveyproject/evey-plugin-weather")).toBeInTheDocument();
    const toggle = screen.getByTestId("plugin-auto-update-switch");
    expect(toggle).toHaveAttribute("aria-checked", "true");
    expect(toggle).not.toBeDisabled();
  });

  it("turning the switch off saves the same repository with automatic updates off", async () => {
    render(<PluginAutoUpdate plugin={{ id: "p1", updateSource: repo, autoUpdate: true }} onReview={vi.fn()} />, { wrapper: Wrapper });
    await userEvent.click(screen.getByTestId("plugin-auto-update-switch"));
    expect(follow).toHaveBeenCalledWith({ pluginId: "p1", data: { updateSource: repo, autoUpdate: false } });
  });

  it("a plugin that follows nothing has the switch greyed and no check button", () => {
    render(<PluginAutoUpdate plugin={{ id: "p1", updateSource: "" }} onReview={vi.fn()} />, { wrapper: Wrapper });
    expect(screen.getByText("This plugin follows no repository.")).toBeInTheDocument();
    expect(screen.getByTestId("plugin-auto-update-switch")).toBeDisabled();
    expect(screen.queryByTestId("plugin-auto-update-check")).not.toBeInTheDocument();
  });

  it("sets the repository from the edit field", async () => {
    render(<PluginAutoUpdate plugin={{ id: "p1", updateSource: "", autoUpdate: true }} onReview={vi.fn()} />, { wrapper: Wrapper });
    await userEvent.click(screen.getByTestId("plugin-auto-update-repo-edit"));
    await userEvent.type(screen.getByTestId("plugin-auto-update-repo-input"), `  ${repo}  `);
    await userEvent.click(screen.getByTestId("plugin-auto-update-repo-save"));
    expect(follow).toHaveBeenCalledWith({ pluginId: "p1", data: { updateSource: repo, autoUpdate: true } });
  });

  it("a held release says so with its reason and offers a review", async () => {
    const onReview = vi.fn();
    render(
      <PluginAutoUpdate
        plugin={{ id: "p1", updateSource: repo, autoUpdate: true,
          updateState: { outcome: "held", latestVersion: "1.1.0", reason: "new entries requested: permission:network" } }}
        onReview={onReview}
      />,
      { wrapper: Wrapper },
    );
    expect(screen.getByTestId("plugin-auto-update-state")).toHaveTextContent(
      "Version 1.1.0 awaits your approval - new entries requested: permission:network");
    await userEvent.click(screen.getByTestId("plugin-auto-update-review"));
    expect(onReview).toHaveBeenCalledWith(repo);
  });

  it("a refused release is shown without a way to install it", () => {
    render(
      <PluginAutoUpdate
        plugin={{ id: "p1", updateSource: repo, updateState: { outcome: "refused", latestVersion: "1.0.2", reason: "the analysis refuses this code: shell_exec" } }}
        onReview={vi.fn()}
      />,
      { wrapper: Wrapper },
    );
    expect(screen.getByTestId("plugin-auto-update-state")).toHaveTextContent("Version 1.0.2 refused - the analysis refuses this code: shell_exec");
    expect(screen.queryByTestId("plugin-auto-update-review")).not.toBeInTheDocument();
  });

  it("check now asks the server for this plugin", async () => {
    render(<PluginAutoUpdate plugin={{ id: "p1", updateSource: repo }} onReview={vi.fn()} />, { wrapper: Wrapper });
    await userEvent.click(screen.getByTestId("plugin-auto-update-check"));
    expect(check).toHaveBeenCalledWith({ pluginId: "p1", data: {} });
  });
});
