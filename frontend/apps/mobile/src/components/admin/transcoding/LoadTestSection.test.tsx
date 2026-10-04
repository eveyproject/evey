import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Pressable: ({ children, onPress, disabled }: any) => React.createElement("button", { onClick: onPress, disabled }, children),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
  Switch: ({ value, onValueChange }: any) =>
    React.createElement("input", { type: "checkbox", "aria-label": "cold", checked: value, onChange: (e: React.ChangeEvent<HTMLInputElement>) => onValueChange(e.target.checked) }),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (k: string, f?: unknown) => {
      if (f && typeof f === "object") return `${k}(${Object.values(f as object).join(",")})`;
      return typeof f === "string" ? f : k;
    },
  }),
}));
vi.mock("../../../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { text: "#fff", textMuted: "#888", accent: "#f00", onAccent: "#000", border: "#333", surface: "#111", surfaceRaised: "#222", error: "#f33", warning: "#fa0", success: "#0a0" }, radii: { sm: 4, md: 8 } }),
}));
const eveyFetchMock = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", () => ({ eveyFetch: (...a: unknown[]) => eveyFetchMock(...a), formatBytes: (n: number) => `${n}B` }));

import { LoadTestSection } from "./LoadTestSection";

function mount() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(React.createElement(QueryClientProvider, { client: qc }, React.createElement(LoadTestSection, { pools: [] })));
}

beforeEach(() => {
  eveyFetchMock.mockReset();
});

// Mobile twin of the web card: same endpoint, same scenarios, no viewer
// ceiling; the live state shows the counters with a cancel; a refusal caused
// by real playback is named; the last report shows at rest.
describe("LoadTestSection (mobile)", () => {
  it("launches with the picked scenario and cache mode, shows the live state, cancels, then the report", async () => {
    let running = false;
    let lastReport: unknown;
    eveyFetchMock.mockImplementation((opts?: { method?: string; data?: unknown }) => {
      const { method, data } = opts ?? {};
      if (method === "GET") return Promise.resolve(running ? { running: true, viewers: 4, phase: "climb", elapsedMs: 9000 } : { running: false, lastReport });
      if (method === "POST") {
        running = true;
        expect(data).toEqual({ scenario: "mkvgo", coldCache: true });
        return Promise.resolve({ running: true, viewers: 0, phase: "warmup", elapsedMs: 0 });
      }
      running = false;
      lastReport = { scenario: "mkvgo", sustainedViewers: 11, stopReason: "max_viewers" };
      return Promise.resolve({ running: false, lastReport });
    });
    mount();
    const start = await screen.findByText("Lancer le test");
    fireEvent.click(screen.getByText("admin.loadtest.scenarioMkvgo"));
    fireEvent.click(screen.getByLabelText("cold"));
    fireEvent.click(start);
    await waitFor(() => expect(screen.getByText(/runningViewers\(4\)/)).toBeTruthy(), { timeout: 4000 });
    expect(screen.getByText(/runningElapsed\(0:09\)/)).toBeTruthy();
    fireEvent.click(screen.getByText("Annuler"));
    await screen.findByText("admin.loadtest.reportHeadline(11,admin.loadtest.scenarioMkvgoNoun)");
    expect(screen.getByText("admin.loadtest.stopReason.maxViewers")).toBeTruthy();
    expect(screen.getByText("Aucune ressource n'a cédé : le plafond est ailleurs.")).toBeTruthy();
  });

  it("a pooled run shows where the audience sat and what each disk did", async () => {
    eveyFetchMock.mockResolvedValue({
      running: false,
      lastReport: {
        scenario: "mkvgo",
        sustainedViewers: 46,
        stopReason: "slo_breach",
        poolBranches: [{ branch: "/mnt/disk_2", viewers: 41 }, { branch: "/mnt/disk_1", viewers: 9 }],
        disks: [{ device: "sdc", utilPercentAvg: 88.6, utilPercentPeak: 97, readBytesPerSecAvg: 104 * 1048576, awaitMsAvg: 132.4 }],
      },
    });
    mount();
    await screen.findByText("admin.loadtest.poolTitle");
    expect(screen.getByText(/poolBranchLine\(\/mnt\/disk_2,41,82\)/)).toBeTruthy();
    expect(screen.getByText(/poolBranchLine\(\/mnt\/disk_1,9,18\)/)).toBeTruthy();
    expect(screen.getByText(/diskLine\(sdc,89,97,/)).toBeTruthy();
  });

  it("shows the server's Go runtime line in the details when it was measured", async () => {
    eveyFetchMock.mockResolvedValue({
      running: false,
      lastReport: {
        scenario: "mkvgo",
        sustainedViewers: 29,
        stopReason: "slo_breach",
        resources: { goHeapLiveBytesAvg: 6000, goHeapLiveBytesMax: 8000, goMemLimitBytes: 9000, goGcCpuPercentAvg: 31.6, goGcCpuPercentPeak: 58.2 },
      },
    });
    mount();
    fireEvent.click(await screen.findByText(/admin\.loadtest\.detailsSummary|Détails de métrologie/));
    expect(await screen.findByText(/goRuntimeLine\(6000B,8000B,9000B,32,58\)/)).toBeTruthy();
  });

  it("names a start refused by live playback", async () => {
    eveyFetchMock.mockImplementation((opts?: { method?: string }) => {
      if (opts?.method === "GET") return Promise.resolve({ running: false });
      return Promise.reject(Object.assign(new Error("locked"), { httpStatus: 423 }));
    });
    mount();
    fireEvent.click(await screen.findByText("Lancer le test"));
    await screen.findByText("Impossible de lancer : une lecture est déjà en cours.");
  });
});
