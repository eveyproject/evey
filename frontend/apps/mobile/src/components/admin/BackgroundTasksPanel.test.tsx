import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Pressable: ({ children, onPress, disabled }: any) => React.createElement("button", { onClick: onPress, disabled }, children),
  ActivityIndicator: () => React.createElement("span", { "data-testid": "spinner" }),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, o?: any) => (o && typeof o === "object" && "count" in o ? `${k}:${o.count}` : k) }),
}));
const eveyFetchMock = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  eveyFetch: (...args: unknown[]) => eveyFetchMock(...args),
}));
const toastMock = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast: toastMock }));

import { BackgroundTasksPanel, type BackgroundTaskView } from "./BackgroundTasksPanel";

const theme = {
  colors: { surface: "#111", border: "#333", accent: "#f00", text: "#fff", textMuted: "#888", warning: "#fa0", success: "#0f0", danger: "#f33", bg: "#000" },
  radii: { md: 8 },
} as any;

function task(over: Partial<BackgroundTaskView>): BackgroundTaskView {
  return { name: "x", state: "idle", processed: 0, controllable: true, paused: false, processes: 0, rssBytes: 0, cpuPercent: 0, cpuSecondsTotal: 0, forks: 0, ...over };
}

function renderPanel(showCoverage = false) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={qc}><BackgroundTasksPanel theme={theme} showCoverage={showCoverage} /></QueryClientProvider>);
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.useRealTimers();
});

describe("BackgroundTasksPanel", () => {
  it("lists running tasks first, then admin-stopped ones, then the idle rest", async () => {
    eveyFetchMock.mockResolvedValueOnce([
      task({ name: "zeta-idle" }),
      task({ name: "alpha-idle" }),
      task({ name: "stopped", paused: true }),
      task({ name: "sweep", state: "running", detail: "42 files left", processes: 2, rssBytes: 1048576, cpuPercent: 37 }),
    ]);
    renderPanel();
    await waitFor(() => expect(screen.getByText("42 files left")).toBeTruthy());
    const labels = screen.getAllByText(/^admin\.activity\.bgTask\./).map((el) => el.textContent);
    expect(labels).toEqual(["admin.activity.bgTask.sweep", "admin.activity.bgTask.stopped", "admin.activity.bgTask.alpha-idle", "admin.activity.bgTask.zeta-idle"]);
    expect(screen.getByText("admin.backgroundTasks.impact")).toBeTruthy();
  });

  it("stops only on a confirming second tap, and the arm expires on its own", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    eveyFetchMock.mockResolvedValueOnce([task({ name: "sweep", state: "running" })]);
    eveyFetchMock.mockResolvedValue(undefined);
    renderPanel();
    await waitFor(() => expect(screen.getByText("admin.backgroundTasks.stop")).toBeTruthy());

    fireEvent.click(screen.getByText("admin.backgroundTasks.stop"));
    expect(screen.getByText("admin.backgroundTasks.confirmStop")).toBeTruthy();
    expect(eveyFetchMock).toHaveBeenCalledTimes(1);
    await act(async () => { await vi.advanceTimersByTimeAsync(4500); });
    expect(screen.getByText("admin.backgroundTasks.stop")).toBeTruthy();

    fireEvent.click(screen.getByText("admin.backgroundTasks.stop"));
    fireEvent.click(screen.getByText("admin.backgroundTasks.confirmStop"));
    await waitFor(() => expect(eveyFetchMock).toHaveBeenCalledWith(expect.objectContaining({ url: "/api/v1/admin/background-tasks/sweep/stop", method: "POST" })));
    await waitFor(() => expect(toastMock.success).toHaveBeenCalledWith("admin.backgroundTasks.stopped"));
  });

  it("resumes a stopped task in one tap and reports a refusal", async () => {
    eveyFetchMock.mockResolvedValueOnce([task({ name: "backfill", paused: true })]);
    eveyFetchMock.mockRejectedValueOnce(new Error("still busy"));
    renderPanel();
    await waitFor(() => expect(screen.getByText("admin.backgroundTasks.resume")).toBeTruthy());
    fireEvent.click(screen.getByText("admin.backgroundTasks.resume"));
    await waitFor(() => expect(eveyFetchMock).toHaveBeenCalledWith(expect.objectContaining({ url: "/api/v1/admin/background-tasks/backfill/resume", method: "POST" })));
    await waitFor(() => expect(toastMock.error).toHaveBeenCalled());
  });
  // The health screen, as the web's health page: each task's library-wide
  // progress and ETA, and the whole on top. The dashboard keeps its rows.
  it("shows library-wide progress and ETA only when asked (the health screen)", async () => {
    const list = [
      task({ name: "intro", state: "running", coverage: { doneItems: 10, totalItems: 40, doneSeconds: 3600, totalSeconds: 7200, etaSeconds: 7200 } }),
      task({ name: "kf", coverage: { doneItems: 1, totalItems: 5, doneSeconds: 0, totalSeconds: 3600 } }),
    ];
    eveyFetchMock.mockResolvedValue(list);
    const { unmount } = renderPanel(true);
    await waitFor(() => expect(screen.getByText("admin.backgroundTasks.overallTitle")).toBeTruthy());
    expect(screen.getByText("admin.backgroundTasks.libraryProgress · admin.backgroundTasks.remaining:30 · admin.backgroundTasks.etaHours:2")).toBeTruthy();
    // No pace yet for kf, so no ETA for the whole either: said, not guessed.
    expect(screen.getAllByText("admin.backgroundTasks.libraryProgress · admin.backgroundTasks.remaining:4 · admin.backgroundTasks.etaUnknown")).toHaveLength(1);
    expect(screen.getByText("admin.backgroundTasks.libraryProgress · admin.backgroundTasks.remaining:34 · admin.backgroundTasks.etaUnknown")).toBeTruthy();
    unmount();

    renderPanel(false);
    await waitFor(() => expect(screen.getByText("admin.activity.bgTask.intro")).toBeTruthy());
    expect(screen.queryByText("admin.backgroundTasks.overallTitle")).toBeNull();
    expect(screen.queryByText(/admin\.backgroundTasks\.libraryProgress/)).toBeNull();
  });
});
