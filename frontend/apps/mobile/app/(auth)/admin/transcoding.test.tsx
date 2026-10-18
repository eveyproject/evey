import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// Recalibrating answers with the bare capability, while the calibration GET
// wraps it with hw_mode, is_wsl and hw_fallbacks. The screen re-reads the GET
// after a recalibration so the hardware fallbacks stay on screen.
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
  RefreshControl: () => null,
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
  TextInput: ({ value, onChangeText, accessibilityLabel }: any) =>
    React.createElement("input", { value, "aria-label": accessibilityLabel, onChange: (e: any) => onChangeText?.(e.target.value) }),
  Switch: ({ value, onValueChange }: any) =>
    React.createElement("input", { type: "checkbox", role: "switch", checked: !!value, onChange: () => onValueChange?.(!value) }),
  Pressable: ({ children, onPress, disabled }: any) => React.createElement("button", { onClick: onPress, disabled }, children),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));
vi.mock("expo-router", () => ({ Stack: { Screen: () => null } }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (k: string, o?: string | Record<string, unknown>) => (typeof o === "string" ? o : typeof o?.defaultValue === "string" ? o.defaultValue : k),
    i18n: { language: "fr" },
  }),
}));
const eveyFetch = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", () => ({
  eveyFetch: (...a: unknown[]) => eveyFetch(...a),
  useAdminSystemServiceGetSystemInfo: () => ({ data: undefined, refetch: vi.fn() }),
  useServerConfigServiceListServerConfigEntries: () => ({ data: [] }),
  getServerConfigServiceListServerConfigEntriesQueryKey: () => ["server-config"],
  serverConfigServiceSetServerConfigEntry: vi.fn(),
  apiErrorMessage: (e: Error) => e.message,
}));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: new Proxy({}, { get: () => "#888" }),
    radii: { sm: 4, md: 8, lg: 12 },
  }),
}));
vi.mock("../../../src/components/admin/transcoding/LoadTestSection", () => ({ LoadTestSection: () => null }));

import AdminTranscodingScreen from "./transcoding";

const capability = { software_max_height: 1080, hw_max_height: 0, benchmarked_at: "2026-10-18T04:00:00Z", benchmark_duration_ms: 1200 };
const calibration = { ...capability, hw_mode: "auto", is_wsl: false, hw_fallbacks: [{ class: "hw_init", codec: "hevc", at: "2026-10-18T03:00:00Z" }] };
const settings = { maxConcurrentTranscodes: { value: 2, default: 2, source: "default" } };

function mount() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={qc}><AdminTranscodingScreen /></QueryClientProvider>);
}

beforeEach(() => {
  vi.clearAllMocks();
  eveyFetch.mockImplementation(async (req: { url: string; method: string }) => {
    if (req.url === "/api/v1/admin/transcode/calibration" && req.method === "GET") return calibration;
    if (req.url === "/api/v1/admin/transcode/calibration/recalibrate" && req.method === "POST") return capability;
    if (req.url === "/api/v1/admin/server-settings" && req.method === "GET") return settings;
    return undefined;
  });
});

describe("admin transcoding screen (mobile)", () => {
  it("keeps the hardware fallbacks after a recalibration by re-reading the calibration", async () => {
    mount();
    expect(await screen.findByText("Bascules matérielles")).toBeTruthy();
    fireEvent.click(screen.getByText("Recalibrer"));
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("Recalibration terminée"));
    await waitFor(() =>
      expect(eveyFetch.mock.calls.filter(([r]) => r.url === "/api/v1/admin/transcode/calibration")).toHaveLength(2),
    );
    expect(screen.getByText("Bascules matérielles")).toBeTruthy();
  });
});
