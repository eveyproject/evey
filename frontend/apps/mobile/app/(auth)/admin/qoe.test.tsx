import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// The dashboard is asked for the chosen window (24h/7d) and polled once a
// minute; the health badge follows the starved ratio and the sample count;
// the starved titles and device rows are listed; an automatic quality limit
// can be reset per source class; a failed fetch names its reason.
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
  RefreshControl: () => null,
  Pressable: ({ children, onPress, accessibilityRole, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, role: accessibilityRole, "aria-label": accessibilityLabel }, children),
  Alert: { alert: vi.fn() },
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
vi.mock("expo-router", () => ({ Stack: { Screen: () => null } }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string, fallback?: string) => fallback ?? k }) }));
const eveyFetch = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", () => ({ eveyFetch: (...a: unknown[]) => eveyFetch(...a), apiErrorMessage: (e: Error) => e.message }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", surface: "#111", surfaceRaised: "#222", text: "#fff", textMuted: "#888", accent: "#f00", onAccent: "#000", border: "#333", success: "#0f0", warning: "#fa0", error: "#f33" }, radii: { md: 8, sm: 4 } }),
}));

import AdminQoEScreen from "./qoe";

const dashboard = (over: Record<string, unknown> = {}) => ({
  summary: { window: "24h", total_events: 1200, starved_events: 84, starved_ratio: 0.07, unique_sessions: 9, unique_media: 5 },
  top_starved_titles: [{ media_item_id: "m1", title: "Some Movie", file_path: "/m/some-movie.mkv", starved_events: 40, total_events: 300, starved_ratio: 0.13, last_starved_at: new Date().toISOString() }],
  by_device: [{ device_class: "mobile", total_events: 700, starved_events: 60, starved_ratio: 0.085, avg_buffer_ms: 5200.4, avg_encoded_kbps: 4100.6 }],
  ...over,
});

function mount() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><AdminQoEScreen /></QueryClientProvider>);
}

beforeEach(() => {
  vi.clearAllMocks();
  eveyFetch.mockImplementation(async (req: { url: string; method: string }) => {
    if (req.url.startsWith("/api/v1/admin/qoe/dashboard")) return dashboard();
    if (req.url.startsWith("/api/v1/admin/transcode/calibration/overrides") && req.method === "GET") {
      return { overrides: [{ source_class: "hevc_2160p", max_height: 1080, reason: "starved", starved_ratio: 0.12, samples_count: 240, applied_at: "", expires_at: "" }] };
    }
    return undefined;
  });
});

describe("admin qoe screen (mobile)", () => {
  it("asks for the chosen window, grades health from the ratio, and lists starved titles, devices and limits", async () => {
    mount();
    await waitFor(() => expect(screen.getByText("Some Movie")).toBeTruthy());
    expect(eveyFetch).toHaveBeenCalledWith({ url: "/api/v1/admin/qoe/dashboard?window=24h", method: "GET" });
    // 7 % starved over 1200 events = marginal.
    expect(screen.getByText("admin.qoe.health.marginal")).toBeTruthy();
    expect(screen.getByText("admin.qoe.health.recoMarginal")).toBeTruthy();
    expect(screen.getByText("mobile")).toBeTruthy();
    expect(screen.getByText(/5 200ms buf|5,200ms buf|5200ms buf/)).toBeTruthy();
    await waitFor(() => expect(screen.getByText(/1080p/)).toBeTruthy());

    fireEvent.click(screen.getByText("admin.qoe.window7d"));
    await waitFor(() => expect(eveyFetch).toHaveBeenCalledWith({ url: "/api/v1/admin/qoe/dashboard?window=7d", method: "GET" }));
  });

  it("resets an automatic limit for its source class and reports a refusal", async () => {
    mount();
    await waitFor(() => expect(screen.getByText("Reset")).toBeTruthy());
    fireEvent.click(screen.getByText("Reset").closest("button")!);
    await waitFor(() => expect(eveyFetch).toHaveBeenCalledWith({ url: "/api/v1/admin/transcode/calibration/overrides?source_class=hevc_2160p", method: "DELETE" }));

    eveyFetch.mockImplementationOnce(async () => { throw new Error("locked"); });
    fireEvent.click(screen.getByText("Reset").closest("button")!);
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("locked"));
  });

  it("says insufficient below 50 events, and names the reason when the dashboard fails", async () => {
    eveyFetch.mockImplementation(async (req: { url: string }) =>
      req.url.startsWith("/api/v1/admin/qoe/dashboard") ? dashboard({ summary: { window: "24h", total_events: 12, starved_events: 0, starved_ratio: 0, unique_sessions: 1, unique_media: 1 }, top_starved_titles: [], by_device: [] }) : { overrides: [] },
    );
    const first = mount();
    await waitFor(() => expect(screen.getByText("admin.qoe.health.insufficient")).toBeTruthy());
    expect(screen.getByText("admin.qoe.topStarved.empty")).toBeTruthy();
    expect(screen.getByText("admin.qoe.byDevice.empty")).toBeTruthy();
    expect(screen.getByText(/No active automatic limits/)).toBeTruthy();
    first.unmount();

    eveyFetch.mockImplementation(async (req: { url: string }) => {
      if (req.url.startsWith("/api/v1/admin/qoe/dashboard")) throw new Error("gateway down");
      return { overrides: [] };
    });
    mount();
    await waitFor(() => expect(screen.getByText(/admin\.qoe\.loadError: gateway down/)).toBeTruthy());
  });
});
