import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const eveyFetchMock = vi.fn();
vi.mock("@evey/api", () => ({
  eveyFetch: (...args: unknown[]) => eveyFetchMock(...args),
  apiErrorMessage: (e: Error) => e.message,
}));
vi.mock("@evey/store", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string, d?: string) => d ?? k }) }));

import { ServerSettingsCard } from "./ServerSettingsCard";

const sv = <T,>(value: T, def: T, source: "override" | "default" = "default") => ({ value, default: def, source });
function settings(overrides: Record<string, unknown> = {}) {
  return {
    serverName: sv("Evey", "Evey"),
    hwAccelEnabled: sv(false, false),
    loginRateLimit: sv(10, 10),
    globalRateLimit: sv(300, 300),
    logLevel: sv("info", "info"),
    libraryWatcherEnabled: sv(true, true),
    periodicScanInterval: sv("2m", "2m"),
    skipPreviewEnabled: sv(false, false),
    introAutoSkipEnabled: sv(false, false),
    auditRetentionDays: sv(0, 0),
    metricsRetentionDays: sv(90, 90),
    qoeRetentionDays: sv(30, 30),
    playSessionRetentionDays: sv(0, 0),
    transcodeCacheMaxGB: sv(50, 50),
    ...overrides,
  };
}

function mount(data: ReturnType<typeof settings>) {
  eveyFetchMock.mockImplementation((opts?: { method?: string }) => Promise.resolve(opts?.method === "GET" ? data : {}));
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(React.createElement(QueryClientProvider, { client: qc }, React.createElement(ServerSettingsCard)));
}

beforeEach(() => {
  eveyFetchMock.mockReset();
});

// The card only sends what the operator set: a default-sourced setting stays
// out of the PUT (an empty field means "keep the default", never "0"), an
// override is echoed as typed, numbers are numbers, and the toggles always
// travel. Reset sends null for that one field.
describe("ServerSettingsCard", () => {
  it("shows defaults as hints and saves only the touched fields, numbers parsed", async () => {
    mount(settings({ loginRateLimit: sv(5, 10, "override") }));
    const name = await screen.findByTestId("server-name-input");
    expect((name as HTMLInputElement).value).toBe("");
    expect(screen.getByTestId("audit-retention-input")).toHaveAttribute("placeholder", "0");
    expect(screen.getByTestId("server-config-save-btn")).toBeDisabled();

    fireEvent.change(screen.getByTestId("cache-budget-input"), { target: { value: "120" } });
    fireEvent.change(name, { target: { value: "Salon" } });
    expect(screen.getByTestId("server-config-save-btn")).toBeEnabled();
    fireEvent.click(screen.getByTestId("server-config-save-btn"));

    await waitFor(() => expect(eveyFetchMock).toHaveBeenCalledWith(expect.objectContaining({ method: "PUT" })));
    const put = eveyFetchMock.mock.calls.find((c) => (c[0] as { method: string }).method === "PUT")![0] as { data: Record<string, unknown> };
    expect(put.data).toEqual({
      serverName: "Salon",
      loginRateLimit: 5,
      libraryWatcherEnabled: true,
      skipPreviewEnabled: false,
      introAutoSkipEnabled: false,
      transcodeCacheMaxGB: 120,
    });
    expect(put.data).not.toHaveProperty("auditRetentionDays");
    await waitFor(() => expect(screen.getByTestId("server-config-save-btn")).toBeDisabled());
  });

  it("an override offers a reset that nulls that one field", async () => {
    mount(settings({ serverName: sv("Salon", "Evey", "override") }));
    const name = await screen.findByTestId("server-name-input");
    expect((name as HTMLInputElement).value).toBe("Salon");
    fireEvent.click(screen.getAllByRole("button", { name: "admin.serverSettings.reset" })[0]);
    await waitFor(() =>
      expect(eveyFetchMock).toHaveBeenCalledWith({ url: "/api/v1/admin/server-settings", method: "PUT", data: { serverName: null } }),
    );
  });
});
