import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// The transcoding screen puts what the admin SETS apart from what the server
// MEASURED, like the web page: encoding, capacity (one Save for two limits),
// cache and the expert limits as settings; calibration and live playback as
// diagnostics.
vi.mock("react-native", async () => (await import("../../../src/test/rnDom")).rnDom);
vi.mock("expo-router", () => ({ Stack: { Screen: () => null } }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (k: string, o?: string | Record<string, unknown>) => (typeof o === "string" ? o : typeof o?.defaultValue === "string" ? o.defaultValue : k),
    i18n: { language: "fr" },
  }),
}));
const eveyFetch = vi.hoisted(() => vi.fn());
const setEntry = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  eveyFetch: (...a: unknown[]) => eveyFetch(...a),
  useAdminSystemServiceGetSystemInfo: () => ({ data: undefined, refetch: vi.fn() }),
  useServerConfigServiceListServerConfigEntries: () => ({ data: [] }),
  getServerConfigServiceListServerConfigEntriesQueryKey: () => ["server-config"],
  serverConfigServiceSetServerConfigEntry: (...a: unknown[]) => setEntry(...a),
  apiErrorMessage: (e: Error) => e.message,
}));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", async (importOriginal) => ({ ...(await importOriginal<typeof import("@evey/store")>()), toast }));
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: new Proxy({}, { get: () => "#888" }), radii: { sm: 4, md: 8, lg: 12 } }),
}));
vi.mock("../../../src/components/admin/transcoding/LoadTestSection", () => ({ LoadTestSection: () => null }));

import AdminTranscodingScreen from "./transcoding";

const capability = { software_max_height: 1080, hw_max_height: 0, benchmarked_at: "2026-10-18T04:00:00Z", benchmark_duration_ms: 1200 };
const calibration = {
  ...capability,
  hw_mode: "auto",
  is_wsl: false,
  detected_backends: ["vaapi"],
  hw_features: ["low_power_h264"],
  hw_fallbacks: [{ class: "hw_init", codec: "hevc", at: "2026-10-18T03:00:00Z" }],
};
const sv = (value: unknown, def: unknown, source: "override" | "default" = "default", extra: Record<string, unknown> = {}) => ({ value, default: def, source, ...extra });
let settings: Record<string, unknown>;

function mount() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={qc}><AdminTranscodingScreen /></QueryClientProvider>);
}

const puts = () => eveyFetch.mock.calls.map((c) => c[0]).filter((r) => r.method === "PUT" && r.url === "/api/v1/admin/server-settings").map((r) => r.data);

beforeEach(() => {
  vi.clearAllMocks();
  settings = {
    maxConcurrentTranscodes: sv(2, 2),
    transcodeCacheMaxGB: sv(10, 10, "default", { requiresRestart: true }),
    av1Enabled: sv(true, false, "override"),
    av1MaxHeight: sv(0, 0),
    hwLowPowerEnabled: sv(false, false),
    maxVideoBitrateKbps: sv(0, 0),
    maxVideoBitrateRemoteKbps: sv(0, 0),
    transcodeMaxSessions: sv(0, 0, "default", { requiresRestart: true }),
    softwareMaxHeight: sv(1080, 0, "override", { requiresRestart: true }),
    softwareHDRMaxHeight: sv(0, 0, "default", { requiresRestart: true }),
    softwareAV1MaxHeight: sv(0, 0),
    transcodeSingleVariant: sv(true, true, "default", { requiresRestart: true }),
    transcodeHWPerVariant: sv(false, false, "default", { requiresRestart: true }),
  };
  setEntry.mockResolvedValue({});
  eveyFetch.mockImplementation(async (req: { url: string; method: string }) => {
    if (req.url === "/api/v1/admin/transcode/calibration" && req.method === "GET") return calibration;
    if (req.url === "/api/v1/admin/transcode/calibration/recalibrate" && req.method === "POST") return capability;
    if (req.url === "/api/v1/admin/server-settings") return req.method === "GET" ? settings : {};
    return undefined;
  });
});

describe("admin transcoding screen (mobile)", () => {
  it("keeps the hardware fallbacks after a recalibration by re-reading the calibration", async () => {
    mount();
    expect(await screen.findByText("Bascules matérielles")).toBeTruthy();
    await screen.findByTestId("hw-enable-toggle");
    const reads = () => eveyFetch.mock.calls.filter(([r]) => r.url === "/api/v1/admin/transcode/calibration").length;
    const before = reads();
    fireEvent.click(screen.getByText("Recalibrer"));
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("Recalibration terminée"));
    await waitFor(() => expect(reads()).toBe(before + 1));
    expect(screen.getByText("Bascules matérielles")).toBeTruthy();
  });

  it("splits settings from diagnostics, the cache budget among the settings", async () => {
    mount();
    const settingsPart = await screen.findByTestId("transcoding-settings");
    for (const id of ["group-encoding", "group-capacity", "group-cache", "group-advanced-limits"]) {
      expect(within(settingsPart).getByTestId(id)).toBeTruthy();
    }
    expect(await within(settingsPart).findByTestId("cache-budget-input")).toBeTruthy();
    const diagnostics = screen.getByTestId("transcoding-diagnostics");
    expect(within(diagnostics).getByText("Recalibrer")).toBeTruthy();
    expect(within(diagnostics).queryByRole("switch")).toBeNull();
  });

  it("gives every setting row an 'i'", async () => {
    mount();
    await screen.findByTestId("software-av1-max-height");
    await screen.findByTestId("low-power-toggle");
    const headings = screen.getAllByTestId("setting-heading");
    // hw, backend, two bitrates, AV1, AV1 max, low power, two capacity, cache, six advanced.
    expect(headings.length).toBe(16);
    for (const h of headings) {
      expect(within(h).queryAllByTestId("setting-info-button").length, h.firstElementChild?.textContent ?? "").toBe(1);
    }
  });

  it("offers the software ceilings, AV1 included, as the ladder the server accepts", async () => {
    mount();
    const sw = await screen.findByTestId("software-max-height");
    expect(within(sw).getAllByRole("radio").map((r) => r.textContent)).toEqual(["admin.settingsLayout.autoMeasured", "480p", "720p", "1080p", "1440p", "2160p"]);
    expect(within(sw).getByRole("radio", { checked: true }).textContent).toBe("1080p");
    const av1 = screen.getByTestId("software-av1-max-height");
    expect(within(av1).getAllByRole("radio").map((r) => r.textContent)).toEqual(["admin.settingsLayout.autoPlain", "480p", "720p", "1080p", "1440p"]);
    expect(within(av1).getByRole("radio", { checked: true }).textContent).toBe("admin.settingsLayout.autoPlain");
  });

  it("saves only the advanced limits that changed", async () => {
    mount();
    const av1 = await screen.findByTestId("software-av1-max-height");
    fireEvent.click(within(av1).getByText("720p"));
    fireEvent.click(screen.getByTestId("advanced-transcoding-save-btn"));
    await waitFor(() => expect(puts()).toEqual([{ softwareAV1MaxHeight: 720 }]));
  });

  it("saves both capacity limits with one Save, and refuses a session cap below 1", async () => {
    mount();
    const sessions = await screen.findByTestId("max-video-sessions-input");
    fireEvent.change(sessions, { target: { value: "0" } });
    fireEvent.click(screen.getByTestId("capacity-save-btn"));
    await waitFor(() => expect(screen.getByTestId("max-video-sessions-row").textContent).toContain("admin.settingsLayout.errorNumber"));
    expect(setEntry).not.toHaveBeenCalled();
    fireEvent.change(sessions, { target: { value: "3" } });
    fireEvent.change(screen.getByTestId("max-concurrent-input"), { target: { value: "4" } });
    fireEvent.click(screen.getByTestId("capacity-save-btn"));
    await waitFor(() => expect(setEntry).toHaveBeenCalledWith("transcoding.max_video_sessions", { value: "3" }));
    await waitFor(() => expect(puts()).toEqual([{ maxConcurrentTranscodes: 4 }]));
  });

  it("applies an encoding pick at once", async () => {
    mount();
    const remote = await screen.findByTestId("max-video-bitrate-remote-select");
    fireEvent.click(within(remote).getByText("12 Mb/s"));
    await waitFor(() => expect(puts()).toEqual([{ maxVideoBitrateRemoteKbps: 12000 }]));
  });
});
