import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, within, act } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// The server configuration screen: the web's eight sections behind a tab
// row, every setting in exactly one of them, an "i" on every setting row but
// the auto-skip intro switch, and saves that send only what a group changed.
// The pull-to-refresh handler the screen hands its ScrollView.
const refreshProps = vi.hoisted(() => ({ onRefresh: async () => {} }));
vi.mock("react-native", async () => {
  const rn = (await import("../../../src/test/rnDom")).rnDom;
  const ScrollView = React.forwardRef((p: any, ref) => {
    if (p.refreshControl) refreshProps.onRefresh = p.refreshControl.props.onRefresh;
    return React.createElement(rn.ScrollView, { ...p, ref });
  });
  return { ...rn, ScrollView };
});
const push = vi.hoisted(() => vi.fn());
vi.mock("expo-router", () => ({ Stack: { Screen: () => null }, useRouter: () => ({ push }) }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string) => k, i18n: { language: "en" } }),
}));
const eveyFetch = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  eveyFetch: (...a: unknown[]) => eveyFetch(...a),
  apiErrorMessage: () => "error",
  getConfigDefaults: () => Promise.resolve({}),
  useLibraryServiceListLibraries: () => ({ data: { libraries: [] }, isLoading: false }),
}));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", async (importOriginal) => ({ ...(await importOriginal<typeof import("@evey/store")>()), toast }));
vi.mock("@evey/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/client")>()),
  useCheckUpdateNow: () => ({ mutate: vi.fn(), isPending: false }),
  useUpdateStatus: () => ({ data: undefined }),
  useServerClock: () => null,
}));
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: new Proxy({}, { get: () => "#000" }), radii: { sm: 4, md: 8, lg: 12 } }),
}));

import Screen from "./server-config";
import { scrollLog } from "../../../src/test/rnDom";

const sv = <T,>(value: T, def: T, source: "override" | "default" = "default", extra: Record<string, unknown> = {}) => ({ value, default: def, source, ...extra });

function settings(over: Record<string, unknown> = {}) {
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
    backupIncludeTelemetry: sv(false, false),
    transcodeCacheMaxGB: sv(10, 10),
    maxConcurrentTranscodes: sv(2, 2),
    hwAccelEnabled: sv(true, true),
    ...over,
  };
}

const sso = {
  enabled: false, issuer: "", clientId: "", clientSecret: "", allowedEmailDomains: [], buttonLabel: "",
  linkByEmail: true, allowSignup: true, allowSignupDefault: false, defaultLibraryIds: [],
  redirectUri: "https://evey.example/cb", postLogoutRedirectUri: "https://evey.example/login", source: "none", status: { state: "disabled" },
};

// Every entry the cards read, so the conditional rows show too (the TIDB
// submit switch needs the providers on and a TIDB key).
const entries = [
  { key: "intro_providers.enabled", value: "true" },
  { key: "api.tidb_key", value: "••••••••" },
];

let current = settings();

function mount() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(<QueryClientProvider client={qc}><Screen /></QueryClientProvider>);
}

const section = (id: string) => screen.getByTestId(`section-${id}`);
const isHidden = (el: HTMLElement) => JSON.parse(el.parentElement?.getAttribute("data-style") ?? "{}").display === "none";
const puts = () => eveyFetch.mock.calls.map((c) => c[0]).filter((r) => r.method === "PUT" && r.url === "/api/v1/admin/server-settings").map((r) => r.data);

beforeEach(() => {
  vi.clearAllMocks();
  current = settings();
  eveyFetch.mockImplementation(async (req: { url: string; method: string }) => {
    if (req.url === "/api/v1/admin/server-config") return { entries };
    if (req.url === "/api/v1/admin/sso") return sso;
    if (req.url === "/api/v1/admin/server-settings") return req.method === "GET" ? current : {};
    return {};
  });
});

// Each setting the old flat screen offered, by its label key, with the
// section it now lives in. hwAccelEnabled, maxConcurrentTranscodes and
// transcodeCacheMaxGB moved to the Transcoding screen, like on the web.
const PLACEMENT: Record<string, string> = {
  "admin.serverSettings.serverName": "general",
  "admin.serverSettings.logLevel": "general",
  "admin.serverConfig.serverLanguage": "general",
  "admin.serverConfig.serverRegion": "general",
  "admin.serverConfig.metadataLanguage": "general",
  "admin.serverConfig.timezone": "general",
  "admin.update.enabled": "general",
  "admin.serverSettings.libraryWatcher": "libraries",
  "admin.serverSettings.periodicScanInterval": "libraries",
  "admin.serverSettings.skipPreview": "libraries",
  "admin.serverSettings.introAutoSkip": "libraries",
  "admin.serverConfig.introProvidersEnabled": "libraries",
  "admin.serverConfig.introProvidersTidbSubmit": "libraries",
  "admin.serverConfig.artworkAutoDownload": "libraries",
  "admin.serverConfig.artworkRetryNow": "libraries",
  "admin.serverConfig.trickplayEnabled": "libraries",
  "admin.serverConfig.parserTokensQualities": "libraries",
  "admin.serverConfig.parserTokensEditions": "libraries",
  "admin.serverConfig.apiKeys.tmdb": "services",
  "admin.serverConfig.apiKeys.omdb": "services",
  "admin.serverConfig.apiKeys.opensubtitles": "services",
  "admin.serverConfig.apiKeys.tidb": "services",
  "admin.serverSettings.passwordRecovery": "accounts",
  "admin.serverSettings.loginRateLimit": "accounts",
  "admin.serverSettings.globalRateLimit": "accounts",
  "admin.sso.enabled": "accounts",
  "admin.serverConfig.smtpHost": "accounts",
  "admin.serverConfig.memoryProfileLabel": "performance",
  "admin.serverConfig.cpuBudget.globalLabel": "performance",
  "admin.serverConfig.ingestServingPlanLabel": "performance",
  "admin.serverConfig.playabilityShadowLabel": "performance",
  "admin.serverSettings.auditRetentionDays": "data",
  "admin.serverSettings.metricsRetentionDays": "data",
  "admin.serverSettings.qoeRetentionDays": "data",
  "admin.serverSettings.playSessionRetentionDays": "data",
  "admin.serverSettings.backupTelemetry": "data",
  "admin.serverConfig.feedConfig.title": "home",
};

const SECTIONS = ["general", "libraries", "services", "accounts", "performance", "data", "home", "advanced"];

describe("admin server config (mobile)", () => {
  it("offers the eight sections as tabs, General open, the others mounted but hidden", async () => {
    mount();
    await screen.findByTestId("server-name-input");
    const tabs = within(screen.getByRole("tablist")).getAllByRole("tab");
    expect(tabs.map((tab) => tab.textContent)).toEqual(SECTIONS.map((id) => `admin.settingsLayout.sections.${id}.title`));
    expect(tabs[0].getAttribute("aria-selected")).toBe("true");
    expect(isHidden(section("general"))).toBe(false);
    expect(isHidden(section("accounts"))).toBe(true);
    fireEvent.click(screen.getByTestId("section-tab-accounts"));
    expect(screen.getByTestId("section-tab-accounts").getAttribute("aria-selected")).toBe("true");
    expect(isHidden(section("accounts"))).toBe(false);
    expect(isHidden(section("general"))).toBe(true);
    // The new section opens at its top, not at the depth the last was read to.
    expect(scrollLog).toContainEqual({ y: 0, animated: false });
  });

  it("places every setting in exactly one section, and the transcoding ones nowhere", async () => {
    mount();
    await screen.findByTestId("intro-tidb-submit-toggle");
    await screen.findByTestId("sso-settings");
    for (const [label, id] of Object.entries(PLACEMENT)) {
      for (const other of SECTIONS) {
        const n = within(section(other)).queryAllByText(label, { exact: true }).length;
        if (other === id) expect(n, `${label} in ${id}`).toBeGreaterThan(0);
        else expect(n, `${label} also in ${other}`).toBe(0);
      }
    }
    expect(screen.queryByTestId("max-concurrent-input")).toBeNull();
    expect(screen.queryByTestId("cache-budget-input")).toBeNull();
    expect(screen.queryByText("admin.serverSettings.hwAccelEnabled")).toBeNull();
  });

  it("gives every setting row an 'i', except the auto-skip intro switch", async () => {
    mount();
    await screen.findByTestId("intro-tidb-submit-toggle");
    await screen.findByTestId("sso-settings");
    await screen.findByTestId("setting-row-backupIncludeTelemetry");
    const headings = screen.getAllByTestId("setting-heading");
    expect(headings.length).toBeGreaterThanOrEqual(50);
    let bare = 0;
    for (const h of headings) {
      const label = h.firstElementChild?.textContent ?? "";
      const infos = within(h).queryAllByTestId("setting-info-button").length;
      if (label === "admin.serverSettings.introAutoSkip") {
        bare++;
        expect(infos).toBe(0);
      } else {
        expect(infos, `${label} has no "i"`).toBe(1);
      }
    }
    expect(bare).toBe(1);
  });

  it("explains a setting in plain words behind its 'i' and shows its restart badge", async () => {
    mount();
    const row = await screen.findByTestId("setting-row-libraryWatcherEnabled");
    fireEvent.click(within(row).getByTestId("setting-info-button"));
    expect(within(row).getByRole("dialog").textContent).toContain("admin.settingsHelp.libraryWatcher");
    expect(within(row).getByTestId("setting-restart-badge").textContent).toBe("admin.serverConfig.appliedOnRestart");
  });

  it("saves a group's own changed fields only, with its own Save", async () => {
    mount();
    fireEvent.change(await screen.findByTestId("metrics-retention-input"), { target: { value: "120" } });
    fireEvent.change(screen.getByTestId("server-name-input"), { target: { value: "Salon" } });
    fireEvent.click(screen.getByTestId("server-settings-save-retention"));
    await waitFor(() => expect(puts()).toEqual([{ metricsRetentionDays: 120 }]));
    // The general group's edit is still pending, untouched by the refetch.
    expect((screen.getByTestId("server-name-input") as HTMLInputElement).value).toBe("Salon");
    fireEvent.click(screen.getByTestId("server-settings-save-general"));
    await waitFor(() => expect(puts()).toEqual([{ metricsRetentionDays: 120 }, { serverName: "Salon" }]));
  });

  it("refuses an invalid value inline and sends nothing", async () => {
    mount();
    fireEvent.change(await screen.findByTestId("login-rate-limit-input"), { target: { value: "-1" } });
    fireEvent.click(screen.getByTestId("server-settings-save-sign-in"));
    const row = screen.getByTestId("setting-row-loginRateLimit");
    expect((await within(row).findByTestId("setting-error")).textContent).toBe("admin.settingsLayout.errorNumber");
    expect(puts()).toEqual([]);
  });

  it("resets the log level to the default with an explicit null", async () => {
    current = settings({ logLevel: sv("debug", "info", "override") });
    mount();
    const row = await screen.findByTestId("setting-row-logLevel");
    await waitFor(() => expect(within(row).getByRole("radio", { checked: true }).textContent).toBe("debug"));
    fireEvent.click(within(row).getByTestId("setting-reset"));
    await waitFor(() => expect(puts()).toEqual([{ logLevel: null }]));
  });

  // The old screen's "env default chip replaces an override" case: picking the
  // Default option and saving clears the stored log level.
  it("sends a reset when the Default option replaces a stored log level", async () => {
    current = settings({ logLevel: sv("debug", "info", "override") });
    mount();
    const row = await screen.findByTestId("setting-row-logLevel");
    await waitFor(() => expect(within(row).getByRole("radio", { checked: true }).textContent).toBe("debug"));
    fireEvent.click(within(row).getByText("admin.settingsLayout.defaultOption"));
    fireEvent.click(screen.getByTestId("server-settings-save-general"));
    await waitFor(() => expect(puts()).toEqual([{ logLevel: null }]));
  });

  it("greys a setting the demo locks", async () => {
    current = settings({ serverName: sv("Evey", "Evey", "default", { locked: true }) });
    mount();
    expect(((await screen.findByTestId("server-name-input")) as HTMLInputElement).disabled).toBe(true);
  });

  it("refreshes every section's data on pull", async () => {
    mount();
    await screen.findByTestId("sso-settings");
    const gets = () => eveyFetch.mock.calls.map((c) => c[0]).filter((r) => r.method === "GET").map((r) => r.url);
    const before = gets();
    expect(before.filter((u) => u === "/api/v1/admin/sso")).toHaveLength(1);
    await act(async () => {
      await refreshProps.onRefresh();
    });
    const after = gets().slice(before.length);
    for (const url of ["/api/v1/admin/server-settings", "/api/v1/admin/server-config", "/api/v1/admin/sso"]) expect(after, url).toContain(url);
  });

  it("puts the raw entries in Advanced with a warning, and opens the home rows screen", async () => {
    mount();
    await screen.findByTestId("server-name-input");
    expect(within(section("advanced")).getByTestId("raw-entries-warning").textContent).toBe("admin.settingsLayout.rawEntriesWarning");
    fireEvent.click(within(section("home")).getByTestId("home-feeds-open"));
    expect(push).toHaveBeenCalledWith("/admin/feeds");
  });
});
