import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import en from "@/test/en-translations.json";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockUseQuery = vi.fn();
const mockUseMutation = vi.fn();
const mockInvalidateQueries = vi.fn();
// The /admin/server-settings payload the field groups read; undefined = loading.
let mockServerSettings: unknown;

vi.mock("@tanstack/react-query", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-query")>();
  return {
    ...actual,
    // Every section is mounted (hidden ones too), so each query gets a
    // settled answer of its own shape: the server settings, the SSO card,
    // and nothing for the rest (update status, locale defaults).
    useQuery: (...args: unknown[]) => {
      const opts = args[0] as { queryKey?: string[] } | undefined;
      const key = JSON.stringify(opts?.queryKey ?? []);
      if (key.includes("server-settings")) {
        return mockServerSettings ? { data: mockServerSettings, isLoading: false } : { data: undefined, isLoading: true };
      }
      if (key.includes("sso")) return { data: ssoSettings, isLoading: false };
      return { data: undefined, isLoading: false };
    },
    useMutation: (...args: unknown[]) => mockUseMutation(...args),
    useQueryClient: () => ({ invalidateQueries: mockInvalidateQueries }),
  };
});

const ssoSettings = {
  enabled: false,
  issuer: "",
  clientId: "",
  clientSecret: "",
  allowedEmailDomains: [],
  buttonLabel: "",
  linkByEmail: true,
  allowSignup: false,
  allowSignupDefault: false,
  defaultLibraryIds: [],
  redirectUri: "https://evey.example/api/v1/auth/oidc/callback",
  postLogoutRedirectUri: "https://evey.example/login",
  source: "none",
  status: { state: "disabled" },
};

// Stable identity on purpose: a fresh object per render would re-run every
// effect keyed on `data` and spin the page in a synchronous loop.
const stableEmptyQuery = { data: { plugins: [], libraries: [], configs: [], items: [] }, isLoading: false, isError: false };
vi.mock("@evey/api", () => ({
  useAdminPluginServiceListPlugins: () => stableEmptyQuery,
  useLibraryServiceListLibraries: () => stableEmptyQuery,
  useFeedServiceGetAdminFeedConfigs: () => stableEmptyQuery,
  eveyFetch: vi.fn(),
  useServerConfigServiceListServerConfigEntries: (...args: unknown[]) => mockUseQuery(...args),
  serverConfigServiceSetServerConfigEntry: vi.fn(),
  serverConfigServiceDeleteServerConfigEntry: vi.fn(),
  getServerConfigServiceListServerConfigEntriesQueryKey: () => ["server-config-entries"],
  apiErrorMessage: (e: Error) => e.message,
  configDefaultsKey: ["config-defaults"],
  getConfigDefaults: vi.fn(),
  defaultRegionForLocale: () => "US",
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
  useLocaleStore: (fn: (s: { setLocale: () => void }) => unknown) => fn({ setLocale: vi.fn() }),
  useDemoStore: (fn: (s: { demoLocked: string[] }) => unknown) => fn({ demoLocked: [] }),
}));

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (opts: { component: React.ComponentType }) => opts,
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

const mockEntries = [
  { key: "smtp.host", value: "smtp.example.com", updatedAt: "2024-01-15T10:00:00Z" },
  { key: "custom.key", value: "custom-value", updatedAt: "2024-01-15T10:00:00Z" },
];

const tr = (key: string) => (en as Record<string, string>)[key] ?? key;
const sv = <T,>(value: T, def: T, extra: Record<string, unknown> = {}) => ({ value, default: def, source: "default", ...extra });

function fullServerSettings() {
  return {
    serverName: sv("Evey", "Evey"),
    logLevel: sv("info", "info"),
    loginRateLimit: sv(10, 10),
    globalRateLimit: sv(300, 300),
    libraryWatcherEnabled: sv(true, true, { requiresRestart: true }),
    periodicScanInterval: sv("2m", "2m", { requiresRestart: true }),
    skipPreviewEnabled: sv(false, false),
    introAutoSkipEnabled: sv(false, false),
    passwordRecoveryEnabled: sv(false, false),
    auditRetentionDays: sv(0, 0),
    metricsRetentionDays: sv(90, 90),
    qoeRetentionDays: sv(30, 30),
    playSessionRetentionDays: sv(0, 0),
    transcodeCacheMaxGB: sv(10, 10, { requiresRestart: true }),
    backupIncludeTelemetry: sv(false, false),
  };
}

async function renderPage() {
  const mod = await import("./server-config.lazy");
  const Page = (mod.Route as unknown as { component: React.ComponentType }).component;
  render(React.createElement(Page), { wrapper: createWrapper() });
}

function openSection(titleKey: string) {
  fireEvent.click(screen.getByRole("tab", { name: tr(`admin.settingsLayout.sections.${titleKey}.title`) }));
}

const section = (id: string) => screen.getByTestId(`section-${id}`);

// Every setting the old single page offered, by the section it now lives in
// (as the label the admin reads). transcodeCacheMaxGB moved to the
// transcoding page and is checked there.
const SETTINGS_BY_SECTION: Record<string, string[]> = {
  general: [
    "admin.serverSettings.serverName",
    "admin.serverSettings.logLevel",
    "admin.serverConfig.serverLanguage",
    "admin.serverConfig.serverRegion",
    "admin.serverConfig.metadataLanguage",
    "admin.serverConfig.timezone",
    "admin.update.enabled",
  ],
  libraries: [
    "admin.serverSettings.libraryWatcher",
    "admin.serverSettings.periodicScanInterval",
    "admin.serverSettings.skipPreview",
    "admin.serverSettings.introAutoSkip",
    "admin.serverConfig.introProvidersEnabled",
    "admin.serverConfig.artworkAutoDownload",
    "admin.serverConfig.artworkRetryNow",
    "admin.serverConfig.trickplayEnabled",
    "admin.serverConfig.parserTokensQualities",
    "admin.serverConfig.parserTokensEditions",
  ],
  services: [
    "admin.serverConfig.apiKeys.tmdb",
    "admin.serverConfig.apiKeys.omdb",
    "admin.serverConfig.apiKeys.opensubtitles",
    "admin.serverConfig.apiKeys.tidb",
  ],
  accounts: [
    "admin.serverSettings.passwordRecovery",
    "admin.serverSettings.loginRateLimit",
    "admin.serverSettings.globalRateLimit",
    "admin.sso.enabled",
    "admin.sso.issuer",
    "admin.sso.clientId",
    "admin.sso.clientSecret",
    "admin.sso.allowedDomains",
    "admin.sso.buttonLabel",
    "admin.sso.linkByEmail",
    "admin.sso.allowSignup",
    "admin.serverConfig.smtpHost",
    "admin.serverConfig.port",
    "admin.serverConfig.tlsMode",
    "admin.serverConfig.smtpUsername",
    "admin.serverConfig.smtpPassword",
    "admin.serverConfig.fromAddress",
  ],
  performance: [
    "admin.serverConfig.memoryProfileTitle",
    "admin.serverConfig.cpuBudget.globalLabel",
    "admin.serverConfig.cpuBudget.tasks.library_scan.label",
    "admin.serverConfig.cpuBudget.tasks.trickplay_generation.label",
    "admin.serverConfig.cpuBudget.tasks.intro_detection.label",
    "admin.serverConfig.cpuBudget.tasks.complexity_classifier.label",
    "admin.serverConfig.backgroundJobs.pauseBgOnPlaybackLabel",
    "admin.serverConfig.backgroundJobs.levelBackfillLabel",
    "admin.serverConfig.backgroundJobs.keyframeBackfillLabel",
    "admin.serverConfig.backgroundJobs.snapshotFastpathLabel",
    "admin.serverConfig.backgroundJobs.levelBackfillBatchLabel",
    "admin.serverConfig.backgroundJobs.levelBackfillPauseMsLabel",
    "admin.serverConfig.backgroundJobs.levelBackfillIdleSecLabel",
    "admin.serverConfig.backgroundJobs.keyframeBackfillBatchLabel",
    "admin.serverConfig.backgroundJobs.keyframeBackfillPauseMsLabel",
    "admin.serverConfig.backgroundJobs.keyframeBackfillIdleSecLabel",
    "admin.serverConfig.ingestServingPlanLabel",
    "admin.serverConfig.playabilityShadowLabel",
  ],
  data: [
    "admin.serverSettings.auditRetentionDays",
    "admin.serverSettings.metricsRetentionDays",
    "admin.serverSettings.qoeRetentionDays",
    "admin.serverSettings.playSessionRetentionDays",
    "admin.serverSettings.backupTelemetry",
  ],
  home: ["admin.serverConfig.feedConfig.title"],
  advanced: ["admin.serverConfig.entriesTitle"],
};

describe("ServerConfigPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockServerSettings = fullServerSettings();
    mockUseMutation.mockReturnValue({ mutate: vi.fn(), mutateAsync: vi.fn(async () => ({})), isPending: false });
    mockUseQuery.mockReturnValue({ data: [], isLoading: false });
    window.history.replaceState(null, "", "/admin/server-config");
  });

  it("renders the heading and a section menu, General open by default", async () => {
    await renderPage();
    expect(screen.getByText(/Server Config/i)).toBeInTheDocument();
    const tabs = screen.getAllByRole("tab");
    expect(tabs.map((t) => t.textContent)).toEqual(
      ["general", "libraries", "services", "accounts", "performance", "data", "home", "advanced"].map((id) =>
        tr(`admin.settingsLayout.sections.${id}.title`),
      ),
    );
    expect(screen.getByRole("tab", { name: tr("admin.settingsLayout.sections.general.title") })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tabpanel")).toContainElement(section("general"));
  });

  it("switches sections from the menu, keeps the others mounted but hidden, and names the section in the address", async () => {
    await renderPage();
    openSection("accounts");
    expect(screen.getByRole("tab", { name: tr("admin.settingsLayout.sections.accounts.title") })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tabpanel")).toContainElement(section("accounts"));
    expect(section("general").closest("[role=tabpanel]")).toHaveAttribute("hidden");
    expect(window.location.hash).toBe("#accounts");
  });

  it("moves between sections with the arrow keys", async () => {
    await renderPage();
    const general = screen.getByRole("tab", { name: tr("admin.settingsLayout.sections.general.title") });
    fireEvent.keyDown(general, { key: "ArrowRight" });
    expect(screen.getByRole("tab", { name: tr("admin.settingsLayout.sections.libraries.title") })).toHaveAttribute("aria-selected", "true");
    fireEvent.keyDown(screen.getByRole("tab", { name: tr("admin.settingsLayout.sections.libraries.title") }), { key: "End" });
    expect(screen.getByRole("tab", { name: tr("admin.settingsLayout.sections.advanced.title") })).toHaveAttribute("aria-selected", "true");
  });

  it("opens on the section the address names", async () => {
    window.history.replaceState(null, "", "/admin/server-config#data");
    await renderPage();
    expect(screen.getByRole("tab", { name: tr("admin.settingsLayout.sections.data.title") })).toHaveAttribute("aria-selected", "true");
  });

  it("places every setting of the old page in exactly one section", async () => {
    await renderPage();
    const ids = Object.keys(SETTINGS_BY_SECTION);
    for (const id of ids) {
      for (const key of SETTINGS_BY_SECTION[id]) {
        const label = tr(key);
        expect(within(section(id)).getAllByText(label, { exact: true }).length, `${key} in ${id}`).toBeGreaterThan(0);
        for (const other of ids.filter((o) => o !== id)) {
          expect(within(section(other)).queryAllByText(label, { exact: true }), `${key} also in ${other}`).toHaveLength(0);
        }
      }
    }
    // The cache budget moved to the transcoding page.
    expect(screen.queryByTestId("cache-budget-input")).toBeNull();
  });

  it("gives every setting row an 'i' explanation, except the auto-skip intro switch", async () => {
    await renderPage();
    const autoSkip = screen.getByTestId("setting-row-introAutoSkipEnabled");
    expect(within(autoSkip).queryByRole("button", { name: /more info/i, hidden: true })).toBeNull();
    const rows = Array.from(document.querySelectorAll("[data-setting-row]")).filter((r) => r !== autoSkip);
    expect(rows.length).toBeGreaterThanOrEqual(59);
    for (const row of rows) {
      const label = row.querySelector("label, span")?.textContent;
      expect(within(row as HTMLElement).queryAllByRole("button", { name: /more info/i, hidden: true }).length, `${label} has no info`).toBeGreaterThan(0);
    }
  });

  it("explains a setting in plain words behind its 'i'", async () => {
    await renderPage();
    openSection("libraries");
    const row = screen.getByTestId("setting-row-libraryWatcherEnabled");
    fireEvent.click(within(row).getByRole("button", { name: /more info/i }));
    expect(within(row).getByRole("dialog")).toHaveTextContent(tr("admin.settingsHelp.libraryWatcher"));
    expect(within(row).getByTestId("setting-restart-badge")).toHaveTextContent(tr("admin.serverConfig.appliedOnRestart"));
  });

  it("saves a group's own fields only, with its own Save", async () => {
    const mutateAsync = vi.fn(async () => ({}));
    mockUseMutation.mockReturnValue({ mutate: vi.fn(), mutateAsync, isPending: false });
    await renderPage();
    fireEvent.change(screen.getByTestId("server-name-input"), { target: { value: "Salon" } });
    openSection("data");
    fireEvent.change(screen.getByTestId("audit-retention-input"), { target: { value: "365" } });
    fireEvent.click(screen.getByTestId("server-settings-save-retention"));
    await waitFor(() => expect(mutateAsync).toHaveBeenCalledWith({ body: { auditRetentionDays: 365 }, keys: ["auditRetentionDays"] }));
    openSection("general");
    expect((screen.getByTestId("server-name-input") as HTMLInputElement).value).toBe("Salon");
    fireEvent.click(screen.getByTestId("server-settings-save-general"));
    await waitFor(() => expect(mutateAsync).toHaveBeenLastCalledWith({ body: { serverName: "Salon" }, keys: ["serverName"] }));
  });

  it("renders the config entries in the Advanced section, with a warning", async () => {
    mockUseQuery.mockReturnValue({ data: mockEntries, isLoading: false });
    await renderPage();
    openSection("advanced");
    expect(screen.getByTestId("raw-entries-warning")).toHaveTextContent(tr("admin.settingsLayout.rawEntriesWarning"));
    expect(screen.getByText("smtp.host")).toBeInTheDocument();
    expect(screen.getByText("custom.key")).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: /^Edit$/i }).length).toBeGreaterThanOrEqual(mockEntries.length);
  });

  it("clicking Edit on config row shows inline editor", async () => {
    mockUseQuery.mockReturnValue({ data: mockEntries, isLoading: false });
    await renderPage();
    openSection("advanced");
    const panel = screen.getByRole("tabpanel");
    const saveBefore = within(panel).queryAllByRole("button", { name: /^Save$/i }).length;
    const cancelBefore = within(panel).queryAllByRole("button", { name: /^Cancel$/i }).length;
    fireEvent.click(within(panel).getAllByRole("button", { name: /^Edit$/i })[0]);
    await waitFor(() => {
      expect(within(panel).getAllByRole("button", { name: /^Save$/i }).length).toBe(saveBefore + 1);
      expect(within(panel).getAllByRole("button", { name: /^Cancel$/i }).length).toBe(cancelBefore + 1);
    });
  });

  it("shows Add button and accessible inputs for adding new entries", async () => {
    await renderPage();
    openSection("advanced");
    expect(screen.getByRole("button", { name: /^Add$/i })).toBeInTheDocument();
    // The key input carries a datalist of the known operator flags, which makes
    // its ARIA role combobox rather than textbox - the accessible NAME is what
    // this test is about, and it is unchanged.
    expect(screen.getByRole("combobox", { name: /^Key$/i })).toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: /^Value$/i })).toBeInTheDocument();
  });

  it("suggests the boot-time operator flags and explains a known key", async () => {
    await renderPage();
    openSection("advanced");
    const keyInput = screen.getByRole("combobox", { name: /^Key$/i });
    expect(keyInput).toHaveAttribute("list", "operator-flag-keys");
    expect(screen.queryByTestId("operator-flag-hint")).not.toBeInTheDocument();
    fireEvent.change(keyInput, { target: { value: "transcoding.snapshot_fastpath" } });
    expect(screen.getByTestId("operator-flag-hint").textContent).toContain("EVEY_SNAPSHOT_FASTPATH");
    fireEvent.change(keyInput, { target: { value: "some.custom.key" } });
    expect(screen.queryByTestId("operator-flag-hint")).not.toBeInTheDocument();
  });

  it("puts e-mail with the accounts and lets the SMTP password be revealed", async () => {
    await renderPage();
    openSection("accounts");
    expect(within(section("accounts")).getByText(/Email \/ SMTP/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Save SMTP Settings/i })).toBeInTheDocument();
    const smtpPassword = screen.getByLabelText(/^Password$/) as HTMLInputElement;
    expect(smtpPassword.type).toBe("password");
    fireEvent.click(within(screen.getByTestId("group-smtp")).getByRole("button", { name: /show password/i }));
    expect(smtpPassword.type).toBe("text");
  });

  it("offers the memory profile as three equal options, Auto by default, and saves a pick", async () => {
    const mutate = vi.fn();
    mockUseMutation.mockReturnValue({ mutate, mutateAsync: vi.fn(async () => ({})), isPending: false });
    await renderPage();
    openSection("performance");
    const group = screen.getByRole("radiogroup", { name: /^Memory profile$/i });
    expect(within(group).getAllByRole("radio").map((r) => r.textContent)).toEqual(["Auto (recommended)", "Normal", "Low"]);
    expect(within(group).getByRole("radio", { checked: true })).toHaveTextContent("Auto (recommended)");
    fireEvent.click(within(group).getByRole("radio", { name: "Low" }));
    expect(mutate).toHaveBeenCalledWith(
      { key: "server.memory_profile", value: "low" },
      expect.objectContaining({ onSuccess: expect.any(Function) }),
    );
  });

  it("reflects a stored server.memory_profile value", async () => {
    mockUseQuery.mockReturnValue({
      data: [{ key: "server.memory_profile", value: "low", updatedAt: "2024-01-15T10:00:00Z" }],
      isLoading: false,
    });
    await renderPage();
    openSection("performance");
    const group = screen.getByRole("radiogroup", { name: /^Memory profile$/i });
    expect(within(group).getByRole("radio", { checked: true })).toHaveTextContent("Low");
  });

  it("offers the media analysis options and saves each to its key", async () => {
    const mutate = vi.fn();
    mockUseMutation.mockReturnValue({ mutate, mutateAsync: vi.fn(async () => ({})), isPending: false });
    mockUseQuery.mockReturnValue({
      data: [{ key: "transcoding.playability_shadow", value: "off", updatedAt: "2024-01-15T10:00:00Z" }],
      isLoading: false,
    });
    await renderPage();
    openSection("performance");
    const ingest = screen.getByRole("radiogroup", { name: /^Ingest serving plan$/i });
    expect(within(ingest).getAllByRole("radio").map((r) => r.textContent)).toEqual(["Off", "Plan", "Plan + repair"]);
    expect(within(ingest).getByRole("radio", { checked: true })).toHaveTextContent("Off");
    const shadow = screen.getByRole("radiogroup", { name: /^Playability shadow$/i });
    expect(within(shadow).getByRole("radio", { checked: true })).toHaveTextContent("Off");

    fireEvent.click(within(ingest).getByRole("radio", { name: "Plan + repair" }));
    expect(mutate).toHaveBeenCalledWith({ key: "library.ingest_serving_plan", value: "plan+repair" }, expect.objectContaining({ onSuccess: expect.any(Function) }));
    fireEvent.click(within(shadow).getByRole("radio", { name: "On" }));
    expect(mutate).toHaveBeenCalledWith({ key: "transcoding.playability_shadow", value: "on" }, expect.objectContaining({ onSuccess: expect.any(Function) }));
  });

  it("shows the background job switches, read at boot, on by default", async () => {
    await renderPage();
    openSection("performance");
    expect(within(section("performance")).getByText(/read once at server startup/i)).toBeInTheDocument();
    for (const label of [/Pause background jobs during playback/i, /^H\.264\/HEVC level backfill$/i, /^Keyframe index backfill$/i, /Timeline thumbnail fast path/i]) {
      const sw = screen.getByRole("switch", { name: label });
      expect(sw).toHaveAttribute("aria-checked", "true");
      expect(within(sw.closest("[data-setting-row]") as HTMLElement).getByTestId("setting-restart-badge")).toBeInTheDocument();
    }
  });

  it("unchecking a background job writes 'off', re-checking deletes the key", async () => {
    const mutateAsync = vi.fn(async () => ({}));
    mockUseMutation.mockReturnValue({ mutate: vi.fn(), mutateAsync, isPending: false });
    mockUseQuery.mockReturnValue({
      data: [{ key: "transcoding.level_backfill", value: "off", updatedAt: "2024-01-15T10:00:00Z" }],
      isLoading: false,
    });
    await renderPage();
    openSection("performance");
    fireEvent.click(screen.getByRole("switch", { name: /^Keyframe index backfill$/i }));
    expect(mutateAsync).toHaveBeenCalledWith({ key: "transcoding.keyframe_backfill", value: "off" });
    const level = screen.getByRole("switch", { name: /^H\.264\/HEVC level backfill$/i });
    expect(level).toHaveAttribute("aria-checked", "false");
    fireEvent.click(level);
    expect(mutateAsync).toHaveBeenCalledWith("transcoding.level_backfill");
  });

  it("keeps the pacing numbers behind an Advanced disclosure and saves them, staying dirty on failure", async () => {
    const mutateAsync = vi.fn(async () => { throw new Error("nope"); });
    mockUseMutation.mockReturnValue({ mutate: vi.fn(), mutateAsync, isPending: false });
    await renderPage();
    openSection("performance");
    const pacing = screen.getByTestId("background-pacing");
    expect(pacing.tagName).toBe("DETAILS");
    expect(pacing).not.toHaveAttribute("open");
    fireEvent.change(within(pacing).getByLabelText(/^Keyframe backfill batch size$/i), { target: { value: "50" } });
    const save = within(pacing).getByRole("button", { name: /^Save$/i, hidden: true });
    fireEvent.click(save);
    expect(mutateAsync).toHaveBeenCalledWith({ key: "transcoding.keyframe_backfill_batch", value: "50" });
    await waitFor(() => expect(mutateAsync).toHaveBeenCalledTimes(6));
    await new Promise((r) => setTimeout(r, 0));
    expect(save).not.toBeDisabled();
  });
});
