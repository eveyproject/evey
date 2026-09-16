import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const eveyFetchMock = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", () => ({
  eveyFetch: (...a: unknown[]) => eveyFetchMock(...a),
  useAdminSystemServiceGetSystemInfo: () => ({ data: { videoSessions: { pools: [] } } }),
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, o?: Record<string, unknown>) => (o ? `${k}(${Object.values(o).join(",")})` : k) }),
}));
vi.mock("@/components/ui/Button", () => ({
  Button: ({ children, onClick, disabled, loading, ...p }: any) => React.createElement("button", { onClick, disabled: disabled || loading, "data-testid": p["data-testid"] }, children),
}));
vi.mock("@/components/ui/Checkbox", () => ({
  Checkbox: ({ checked, onChange, ...p }: any) =>
    React.createElement("input", { type: "checkbox", checked, onChange: (e: React.ChangeEvent<HTMLInputElement>) => onChange(e.target.checked), "data-testid": p["data-testid"] }),
}));
vi.mock("@/components/ui/InfoTooltip", () => ({ InfoTooltip: () => null }));
vi.mock("./LoadTestReportPanel", () => ({ LoadTestReportPanel: ({ report }: any) => React.createElement("div", { "data-testid": "report" }, report.stopReason) }));

import { LoadTestCard } from "./LoadTestCard";

function mount() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(React.createElement(QueryClientProvider, { client: qc }, React.createElement(LoadTestCard)));
}

beforeEach(() => {
  eveyFetchMock.mockReset();
});

// The card launches a run with the chosen scenario and cache mode and no
// viewer ceiling (the machine, not a number, ends the climb), shows the live
// counters while running with a cancel, names a refusal caused by real
// playback for what it is, and shows the last report when idle.
describe("LoadTestCard", () => {
  it("starts a run with the chosen scenario, no ceiling, then shows the live state and cancels", async () => {
    let running = false;
    let lastReport: unknown;
    eveyFetchMock.mockImplementation((opts?: { method?: string; data?: unknown }) => {
      const { method, data } = opts ?? {};
      if (method === "GET") return Promise.resolve(running ? { running: true, viewers: 7, phase: "climb", elapsedMs: 65_000 } : { running: false, lastReport });
      if (method === "POST") {
        running = true;
        expect(data).toEqual({ scenario: "heavy", coldCache: true });
        return Promise.resolve({ running: true, viewers: 0, phase: "warmup", elapsedMs: 0 });
      }
      running = false;
      lastReport = { stopReason: "cancelled" };
      return Promise.resolve({ running: false, lastReport });
    });
    mount();
    await screen.findByTestId("load-test-idle");
    expect(screen.queryByTestId("report")).toBeNull();
    fireEvent.change(screen.getByTestId("load-test-scenario-select"), { target: { value: "heavy" } });
    fireEvent.click(screen.getByTestId("load-test-cold-cache-toggle"));
    fireEvent.click(screen.getByTestId("load-test-start-btn"));
    await screen.findByTestId("load-test-running");
    await waitFor(() => expect(screen.getByTestId("load-test-running")).toHaveTextContent("admin.loadtest.runningViewers(7)"), { timeout: 4000 });
    expect(screen.getByTestId("load-test-running")).toHaveTextContent("admin.loadtest.runningElapsed(1:05)");
    fireEvent.click(screen.getByTestId("load-test-cancel-btn"));
    await screen.findByTestId("load-test-idle");
    expect(screen.getByTestId("report")).toHaveTextContent("cancelled");
  });

  it("names a start refused by live playback", async () => {
    eveyFetchMock.mockImplementation((opts?: { method?: string }) => {
      if (opts?.method === "GET") return Promise.resolve({ running: false });
      return Promise.reject(Object.assign(new Error("conflict"), { httpStatus: 409 }));
    });
    mount();
    await screen.findByTestId("load-test-idle");
    fireEvent.click(screen.getByTestId("load-test-start-btn"));
    await waitFor(() => expect(screen.getByTestId("load-test-error")).toHaveTextContent("admin.loadtest.errorActiveStreams"));
  });

  it("any other refusal is a generic failure", async () => {
    eveyFetchMock.mockImplementation((opts?: { method?: string }) => {
      if (opts?.method === "GET") return Promise.resolve({ running: false });
      return Promise.reject(Object.assign(new Error("boom"), { httpStatus: 400 }));
    });
    mount();
    await screen.findByTestId("load-test-idle");
    fireEvent.click(screen.getByTestId("load-test-start-btn"));
    await waitFor(() => expect(screen.getByTestId("load-test-error")).toHaveTextContent("admin.loadtest.errorGeneric"));
  });
});
