import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const eveyFetchMock = vi.fn();
vi.mock("@evey/api", () => ({
  eveyFetch: (...args: unknown[]) => eveyFetchMock(...args),
  apiErrorMessage: (e: Error) => e.message,
}));
vi.mock("@evey/store", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

import { ServerSettingsGroup, type ServerSettingField } from "./serverSettings";

const sv = <T,>(value: T, def: T, source: "override" | "default" = "default", extra: Record<string, unknown> = {}) => ({ value, default: def, source, ...extra });

function settings(overrides: Record<string, unknown> = {}) {
  return {
    serverName: sv("Evey", "Evey"),
    logLevel: sv("info", "info"),
    loginRateLimit: sv(10, 10),
    globalRateLimit: sv(300, 300),
    libraryWatcherEnabled: sv(true, true, "default", { requiresRestart: true }),
    periodicScanInterval: sv("2m", "2m", "default", { requiresRestart: true }),
    skipPreviewEnabled: sv(false, false),
    introAutoSkipEnabled: sv(false, false),
    passwordRecoveryEnabled: sv(false, false),
    auditRetentionDays: sv(0, 0),
    metricsRetentionDays: sv(90, 90),
    qoeRetentionDays: sv(30, 30),
    playSessionRetentionDays: sv(0, 0),
    transcodeCacheMaxGB: sv(10, 10, "default", { requiresRestart: true }),
    backupIncludeTelemetry: sv(false, false),
    ...overrides,
  };
}

function mount(data: Record<string, unknown>, groups: { fields: ServerSettingField[]; save: string }[]) {
  let current = data;
  eveyFetchMock.mockImplementation((opts?: { method?: string }) => Promise.resolve(opts?.method === "GET" ? current : {}));
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(
    React.createElement(
      QueryClientProvider,
      { client: qc },
      groups.map((g) => React.createElement(ServerSettingsGroup, { key: g.save, title: g.save, fields: g.fields, saveTestId: g.save })),
    ),
  );
  return { setData: (d: Record<string, unknown>) => (current = d) };
}

const puts = () =>
  eveyFetchMock.mock.calls.map((c) => c[0] as { method: string; data: unknown }).filter((c) => c.method === "PUT").map((c) => c.data);

beforeEach(() => eveyFetchMock.mockReset());

// The server-settings resource is split across sections: each group sends a
// partial object with only the fields the admin changed in it (the server
// leaves absent fields alone), numbers as numbers, and an emptied field never
// becomes a 0. Reset sends an explicit null for one field.
describe("ServerSettingsGroup", () => {
  it("shows defaults, saves only the touched fields of its group, numbers parsed", async () => {
    mount(settings({ loginRateLimit: sv(5, 10, "override") }), [
      { fields: ["serverName", "logLevel"], save: "save-general" },
      { fields: ["auditRetentionDays", "metricsRetentionDays", "qoeRetentionDays", "playSessionRetentionDays"], save: "save-retention" },
      { fields: ["passwordRecoveryEnabled", "loginRateLimit", "globalRateLimit"], save: "save-sign-in" },
    ]);
    const name = (await screen.findByTestId("server-name-input")) as HTMLInputElement;
    expect(name.value).toBe("");
    expect(screen.getByTestId("audit-retention-input")).toHaveAttribute("placeholder", "0");
    expect((screen.getByTestId("login-rate-limit-input") as HTMLInputElement).value).toBe("5");
    for (const id of ["save-general", "save-retention", "save-sign-in"]) expect(screen.getByTestId(id)).toBeDisabled();

    fireEvent.change(screen.getByTestId("metrics-retention-input"), { target: { value: "120" } });
    fireEvent.click(screen.getByTestId("save-retention"));
    await waitFor(() => expect(puts()).toEqual([{ metricsRetentionDays: 120 }]));
    await waitFor(() => expect(screen.getByTestId("save-retention")).toBeDisabled());
  });

  it("an unsaved edit in one group survives another group's save and refetch", async () => {
    const { setData } = mount(settings(), [
      { fields: ["serverName"], save: "save-general" },
      { fields: ["backupIncludeTelemetry"], save: "save-backups" },
    ]);
    fireEvent.change(await screen.findByTestId("server-name-input"), { target: { value: "Salon" } });
    setData(settings({ backupIncludeTelemetry: sv(true, false, "override") }));
    fireEvent.click(screen.getByTestId("backup-telemetry-toggle"));
    fireEvent.click(screen.getByTestId("save-backups"));
    await waitFor(() => expect(puts()).toEqual([{ backupIncludeTelemetry: true }]));
    // The refetch landed (the toggle reads the new override)...
    await waitFor(() => expect(screen.getByTestId("setting-row-backupIncludeTelemetry")).toHaveTextContent("admin.serverSettings.reset"));
    // ...and the name typed in the other group is still there, still unsaved.
    expect((screen.getByTestId("server-name-input") as HTMLInputElement).value).toBe("Salon");
    expect(screen.getByTestId("save-general")).toBeEnabled();
  });

  it("a toggle is saved like any field, and an emptied field is not sent as 0", async () => {
    mount(settings({ globalRateLimit: sv(400, 300, "override") }), [
      { fields: ["passwordRecoveryEnabled", "loginRateLimit", "globalRateLimit"], save: "save-sign-in" },
    ]);
    fireEvent.click(await screen.findByTestId("password-recovery-toggle"));
    fireEvent.change(screen.getByTestId("global-rate-limit-input"), { target: { value: "" } });
    fireEvent.click(screen.getByTestId("save-sign-in"));
    await waitFor(() => expect(puts()).toEqual([{ passwordRecoveryEnabled: true }]));
  });

  it("an override offers a reset that nulls that one field", async () => {
    mount(settings({ serverName: sv("Salon", "Evey", "override") }), [{ fields: ["serverName"], save: "save-general" }]);
    expect(((await screen.findByTestId("server-name-input")) as HTMLInputElement).value).toBe("Salon");
    fireEvent.click(screen.getByRole("button", { name: "admin.serverSettings.reset" }));
    await waitFor(() =>
      expect(eveyFetchMock).toHaveBeenCalledWith({ url: "/api/v1/admin/server-settings", method: "PUT", data: { serverName: null } }),
    );
  });

  it("badges the settings read at boot and greys a locked one", async () => {
    mount(settings({ loginRateLimit: sv(10, 10, "default", { locked: true }) }), [
      { fields: ["libraryWatcherEnabled", "periodicScanInterval"], save: "save-scanning" },
      { fields: ["loginRateLimit"], save: "save-sign-in" },
    ]);
    const watcher = await screen.findByTestId("setting-row-libraryWatcherEnabled");
    expect(within(watcher).getByTestId("setting-restart-badge")).toBeInTheDocument();
    expect(within(screen.getByTestId("setting-row-loginRateLimit")).queryByTestId("setting-restart-badge")).toBeNull();
    expect(screen.getByTestId("login-rate-limit-input")).toBeDisabled();
  });

  it("explains every field behind an 'i', except the auto-skip intro switch", async () => {
    mount(settings(), [{ fields: ["skipPreviewEnabled", "introAutoSkipEnabled"], save: "save-skipping" }]);
    const preview = await screen.findByTestId("setting-row-skipPreviewEnabled");
    fireEvent.click(within(preview).getByRole("button", { name: "admin.settingsLayout.moreInfo" }));
    expect(within(preview).getByRole("dialog")).toHaveTextContent("admin.serverSettings.skipPreviewInfo");
    const autoSkip = screen.getByTestId("setting-row-introAutoSkipEnabled");
    expect(within(autoSkip).queryByRole("button", { name: "admin.settingsLayout.moreInfo" })).toBeNull();
    expect(within(autoSkip).getByRole("switch", { name: "admin.serverSettings.introAutoSkip" })).toBeInTheDocument();
  });
});
