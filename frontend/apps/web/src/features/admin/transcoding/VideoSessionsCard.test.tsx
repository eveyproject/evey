import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const api = vi.hoisted(() => ({ sys: { data: undefined as unknown }, entries: [] as unknown[], setEntry: vi.fn() }));
vi.mock("@evey/api", () => ({
  useAdminSystemServiceGetSystemInfo: () => api.sys,
  useServerConfigServiceListServerConfigEntries: (opts: any) => ({ data: opts.query.select({ entries: api.entries }) }),
  getServerConfigServiceListServerConfigEntriesQueryKey: () => ["config"],
  serverConfigServiceSetServerConfigEntry: (...a: unknown[]) => api.setEntry(...a),
}));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, o?: Record<string, unknown>) => (o && typeof o === "object" ? `${k}(${Object.entries(o).map(([a, b]) => `${a}=${String(b)}`).join(",")})` : k) }),
}));
vi.mock("@/components/ui/Input", () => ({
  Input: ({ label, value, onChange, id }: any) => React.createElement("input", { id, "aria-label": label, value, onChange }),
}));
vi.mock("@/components/ui/Button", () => ({
  Button: ({ children, onClick, disabled, loading, ...p }: any) => React.createElement("button", { onClick, disabled: disabled || loading, "data-testid": p["data-testid"] }, children),
}));

import { MaxVideoSessionsSetting, VideoSessionsCard } from "./VideoSessionsCard";

function mount(component: React.ComponentType = VideoSessionsCard) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(React.createElement(QueryClientProvider, { client: qc }, React.createElement(component)));
}

beforeEach(() => {
  vi.clearAllMocks();
  api.sys = { data: undefined };
  api.entries = [];
  api.setEntry.mockResolvedValue({});
});

// The card is a statement of fact: each pool shows its budget, the ladder
// class that describes it (software for the CPU pool) and how many streams
// of each SOURCE the cheapest rung holds; without pools the measured capacity
// and its evidence show instead. The one control writes the cap.
describe("VideoSessionsCard", () => {
  it("describes each pool by its primary ladder class and the cheapest rung's source fits", () => {
    api.sys = {
      data: {
        videoSessions: {
          autoCapacity: 6, active: 1,
          pools: [
            {
              pool: "cpu", budgetUsed: 1.5, budgetTotal: 4,
              classes: [
                { ladderClass: "hw", rungFits: [{ height: 1080, maxStreams: 9, cost: 1 }] },
                { ladderClass: "software", rungFits: [
                  { height: 1080, maxStreams: 2, cost: 2, bySource: [{ source: { codec: "hevc", height: 2160 }, maxStreams: 1 }] },
                  { height: 480, maxStreams: 8, cost: 0.5, bySource: [{ source: { codec: "h264", height: 1080 }, maxStreams: 6 }] },
                ] },
              ],
            },
            { pool: "gpu", budgetUsed: 0, budgetTotal: 2, classes: [{ ladderClass: "hw", rungFits: [{ height: 1080, maxStreams: 5, cost: 1 }] }] },
          ],
        },
      },
    };
    mount();
    const cpu = screen.getByTestId("video-pool-cpu");
    expect(within(cpu).getByText("admin.transcoding.videoPoolBudget(used=1.5,total=4)")).toBeInTheDocument();
    expect(within(cpu).getByRole("progressbar").getAttribute("aria-valuenow")).toBe("38");
    expect(within(cpu).getByText("admin.transcoding.videoPoolRungFit(count=2,height=1080)")).toBeInTheDocument();
    expect(within(cpu).queryByText("admin.transcoding.videoPoolRungFit(count=9,height=1080)")).toBeNull();
    const sources = within(cpu).getByTestId("video-pool-cpu-sources");
    expect(sources).toHaveTextContent("H264 1080p");
    expect(sources).toHaveTextContent("videoSourceStreams(count=6)");
    expect(sources).not.toHaveTextContent("HEVC");
    const gpu = screen.getByTestId("video-pool-gpu");
    expect(within(gpu).getByText("admin.transcoding.videoPoolRungFit(count=5,height=1080)")).toBeInTheDocument();
    expect(within(gpu).queryByTestId("video-pool-gpu-sources")).toBeNull();
    expect(screen.queryByTestId("video-sessions-measured")).toBeNull();
  });

  it("without pools shows the measured capacity and says when it was not measured", () => {
    api.sys = { data: { videoSessions: { autoCapacity: 3, active: 2, pools: [], evidence: { derived: false } } } };
    mount();
    expect(screen.getByTestId("video-sessions-measured")).toHaveTextContent("videoSessionsMeasured(auto=3,active=2)");
    expect(screen.getByText("admin.transcoding.videoSessionsNotMeasured")).toBeInTheDocument();
  });

  it("the cap control seeds from the stored row and saves the trimmed value", async () => {
    api.entries = [{ key: "transcoding.max_video_sessions", value: "4" }];
    // The cap is a setting: it sits on the settings side of the page, apart
    // from the measured readout.
    mount(MaxVideoSessionsSetting);
    const input = screen.getByLabelText("admin.transcoding.maxVideoSessionsLabel") as HTMLInputElement;
    expect(input.value).toBe("4");
    const save = screen.getByTestId("max-video-sessions-save-btn") as HTMLButtonElement;
    expect(save.disabled).toBe(true);
    fireEvent.change(input, { target: { value: " 2 " } });
    expect(save.disabled).toBe(false);
    fireEvent.click(save);
    await waitFor(() => expect(api.setEntry).toHaveBeenCalledWith("transcoding.max_video_sessions", { value: "2" }));
    await waitFor(() => expect(toast.success).toHaveBeenCalled());
    expect(save.disabled).toBe(true);
  });
});
