import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// The settings Save sends what the admin changed. It used to send hwMode on
// every save, which turned a backend pinned on the web (nvenc, vaapi, qsv)
// back to "auto" and started a new HW bench each time.
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
  RefreshControl: () => null,
  TextInput: ({ value, onChangeText, accessibilityLabel }: any) =>
    React.createElement("input", { value, "aria-label": accessibilityLabel, onChange: (e: any) => onChangeText(e.target.value) }),
  Switch: ({ value, onValueChange, accessibilityLabel }: any) =>
    React.createElement("input", { type: "checkbox", role: "switch", "aria-label": accessibilityLabel, checked: value, onChange: (e: any) => onValueChange(e.target.checked) }),
  Pressable: ({ children, onPress, disabled }: any) => React.createElement("button", { onClick: onPress, disabled }, children),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));
vi.mock("expo-router", () => ({ Stack: { Screen: () => null } }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string, fallback?: string) => (typeof fallback === "string" ? fallback : k) }) }));
const eveyFetch = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", () => ({ eveyFetch: (...a: unknown[]) => eveyFetch(...a), apiErrorMessage: () => "error" }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));
vi.mock("@evey/client", () => ({
  latestLabel: () => "",
  useCheckUpdateNow: () => ({ mutate: vi.fn(), isPending: false }),
  useUpdateStatus: () => ({ data: undefined }),
}));
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: new Proxy({}, { get: () => "#000" }), radii: { md: 8 } }),
}));
for (const card of ["LocaleSettingsCard", "MemoryProfileCard", "MkvgoCard", "SmtpSettingsCard", "SsoSettingsCard", "BackgroundJobsCard", "CpuBudgetCard", "ParserTokensCard", "ConfigEntriesCard"]) {
  vi.doMock(`../../../src/components/admin/server-config/${card}`, () => ({ [card]: () => null }));
}

const setting = <T,>(value: T, def: T, source: "override" | "default" = "default") => ({ value, default: def, source });

function settings(over: Record<string, unknown> = {}) {
  return {
    serverName: setting("Evey", "Evey"),
    maxConcurrentTranscodes: setting(2, 2),
    hwAccelEnabled: setting(true, true, "override"),
    loginRateLimit: setting(10, 10),
    globalRateLimit: setting(300, 300),
    logLevel: setting("info", "info"),
    libraryWatcherEnabled: setting(true, true),
    periodicScanInterval: setting("", ""),
    skipPreviewEnabled: setting(false, false),
    introAutoSkipEnabled: setting(false, false),
    passwordRecoveryEnabled: setting(false, false),
    auditRetentionDays: setting(90, 90),
    metricsRetentionDays: setting(30, 30),
    qoeRetentionDays: setting(30, 30),
    playSessionRetentionDays: setting(90, 90),
    transcodeCacheMaxGB: setting(50, 50),
    ...over,
  };
}

let current = settings();

async function mount() {
  const { default: Screen } = await import("./server-config");
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={qc}><Screen /></QueryClientProvider>);
  await screen.findByText("debug");
}

function savedBody(): Record<string, unknown> {
  const put = eveyFetch.mock.calls.map((c) => c[0]).find((r) => r.method === "PUT" && r.url === "/api/v1/admin/server-settings");
  expect(put).toBeDefined();
  return put.data;
}

async function saveSettings() {
  const saves = screen.getAllByText("common.save");
  fireEvent.click(saves[saves.length - 1]);
  await waitFor(() => expect(toast.success).toHaveBeenCalled());
}

beforeEach(() => {
  vi.clearAllMocks();
  current = settings();
  eveyFetch.mockImplementation(async (req: { url: string; method: string }) => {
    if (req.url === "/api/v1/admin/server-config") return { entries: [] };
    return current;
  });
});

describe("admin server config (mobile): settings save", () => {
  it("leaves the HW mode alone when the admin only changed the log level", async () => {
    await mount();
    fireEvent.click(screen.getByText("debug"));
    await saveSettings();
    expect(savedBody()).toMatchObject({ logLevel: "debug" });
    expect(savedBody()).not.toHaveProperty("hwMode");
  });

  it("sends the HW mode once the switch is flipped", async () => {
    await mount();
    fireEvent.click(screen.getByLabelText("admin.serverSettings.hwAccelEnabled"));
    await saveSettings();
    expect(savedBody()).toMatchObject({ hwMode: "off" });
  });

  it("sends a reset when the env default chip replaces an override", async () => {
    current = settings({ logLevel: setting("debug", "info", "override") });
    await mount();
    fireEvent.click(screen.getByText(/Défaut env/));
    await saveSettings();
    expect(savedBody()).toMatchObject({ logLevel: null });
  });
});
