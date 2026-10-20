import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// The SMTP and background-pacing cards write six keys in one Save. Against the
// screen's REAL mutations: TanStack keeps the callbacks of the last mutate()
// call only, so a per-call counter never reached zero (no "SMTP saved", a
// pacing Save that stayed enabled) while the hook-level onSuccess toasted
// "Config saved" six times.
vi.mock("react-native", async () => (await import("../../../src/test/rnDom")).rnDom);
vi.mock("expo-router", () => ({ Stack: { Screen: () => null }, useRouter: () => ({ push: vi.fn() }) }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string, fallback?: string) => (typeof fallback === "string" ? fallback : k) }) }));
const eveyFetch = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", () => ({ eveyFetch: (...a: unknown[]) => eveyFetch(...a), apiErrorMessage: () => "error" }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));
vi.mock("@evey/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/client")>()),
  latestLabel: () => "",
  useCheckUpdateNow: () => ({ mutate: vi.fn(), isPending: false }),
  useUpdateStatus: () => ({ data: undefined }),
}));
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: new Proxy({}, { get: () => "#000" }), radii: { sm: 4, md: 8, lg: 12 } }),
}));
for (const card of ["LocaleSettingsCard", "MemoryProfileCard", "MkvgoCard", "SsoSettingsCard", "CpuBudgetCard", "ParserTokensCard", "ConfigEntriesCard"]) {
  vi.doMock(`../../../src/components/admin/server-config/${card}`, () => ({ [card]: () => null }));
}

const setting = <T,>(value: T) => ({ value, default: value, source: "default" as const });
const settings = {
  serverName: setting("Evey"),
  maxConcurrentTranscodes: setting(2),
  hwAccelEnabled: setting(true),
  loginRateLimit: setting(10),
  globalRateLimit: setting(300),
  logLevel: setting("info"),
  libraryWatcherEnabled: setting(true),
  periodicScanInterval: setting(""),
  skipPreviewEnabled: setting(false),
  introAutoSkipEnabled: setting(false),
  passwordRecoveryEnabled: setting(false),
  auditRetentionDays: setting(90),
  metricsRetentionDays: setting(30),
  qoeRetentionDays: setting(30),
  playSessionRetentionDays: setting(90),
  transcodeCacheMaxGB: setting(50),
};

let failKey: string | null = null;

async function mount() {
  const { default: Screen } = await import("./server-config");
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(<QueryClientProvider client={qc}><Screen /></QueryClientProvider>);
}

async function mountAndSave() {
  await mount();
  fireEvent.click(await screen.findByText("Save SMTP Settings"));
  await waitFor(() => expect(eveyFetch.mock.calls.filter((c) => c[0].method === "PUT" && String(c[0].url).includes("smtp.")).length).toBe(6));
}

beforeEach(() => {
  vi.clearAllMocks();
  failKey = null;
  eveyFetch.mockImplementation(async (req: { url: string; method: string }) => {
    if (req.url === "/api/v1/admin/server-config") return { entries: [] };
    if (req.method !== "GET" && req.url.startsWith("/api/v1/admin/server-config/")) {
      if (failKey && req.url.endsWith(failKey)) throw new Error("boom");
      return {};
    }
    return settings;
  });
});

describe("admin server config (mobile): SMTP save", () => {
  it("reports one success for the six writes, without a burst of Config saved", async () => {
    await mountAndSave();
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("SMTP settings saved"));
    expect(toast.success).toHaveBeenCalledTimes(1);
    expect(toast.error).not.toHaveBeenCalled();
  });

  it("reports one failure naming the key that did not land, and no success", async () => {
    failKey = "smtp.password";
    await mountAndSave();
    await waitFor(() => expect(toast.error).toHaveBeenCalledTimes(1));
    expect(toast.error).toHaveBeenCalledWith("Failed to save {{key}}");
    expect(toast.success).not.toHaveBeenCalled();
  });
});

describe("admin server config (mobile): background pacing save", () => {
  it("disables Save again once the six writes have landed", async () => {
    await mount();
    const field = await screen.findByTestId("bg-rate-transcoding.level_backfill_batch");
    fireEvent.change(field, { target: { value: "25" } });
    const save = screen.getByTestId("bg-rates-save") as HTMLButtonElement;
    expect(save.disabled).toBe(false);
    fireEvent.click(save);
    await waitFor(() => expect(eveyFetch.mock.calls.filter((c) => c[0].method !== "GET" && String(c[0].url).includes("backfill_")).length).toBe(6));
    await waitFor(() => expect(save.disabled).toBe(true));
    expect(toast.success).toHaveBeenCalledTimes(1);
    expect(toast.success).toHaveBeenCalledWith("Config saved");
    expect(toast.error).not.toHaveBeenCalled();
  });

  it("reports one failure when a write does not land, and keeps Save enabled", async () => {
    failKey = "transcoding.keyframe_backfill_pause_ms";
    await mount();
    fireEvent.change(await screen.findByTestId("bg-rate-transcoding.level_backfill_batch"), { target: { value: "25" } });
    const save = screen.getByTestId("bg-rates-save") as HTMLButtonElement;
    fireEvent.click(save);
    await waitFor(() => expect(toast.error).toHaveBeenCalledTimes(1));
    expect(toast.error).toHaveBeenCalledWith("Failed to update config");
    expect(toast.success).not.toHaveBeenCalled();
    expect(save.disabled).toBe(false);
  });
});
