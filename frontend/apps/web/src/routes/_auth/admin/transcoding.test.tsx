import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockGetSystemInfo = vi.fn();
const mockSetConfigEntry = vi.fn();
const mockListConfigEntries = vi.fn();

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(() => Promise.resolve({})),
  useAdminSystemServiceGetSystemInfo: (...args: unknown[]) => mockGetSystemInfo(...args),
  useServerConfigServiceListServerConfigEntries: (...args: unknown[]) => mockListConfigEntries(...args),
  getServerConfigServiceListServerConfigEntriesQueryKey: () => ["cfg"],
  serverConfigServiceSetServerConfigEntry: (...args: unknown[]) => mockSetConfigEntry(...args),
}));

vi.mock("@evey/store", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (opts: { component: React.ComponentType }) => opts,
}));

// ConcurrencyCard (the background-job knob) fetches /admin/server-settings via
// useQuery; give it a settled shape so the page renders both sections.
vi.mock("@tanstack/react-query", async () => {
  const actual = await vi.importActual<typeof import("@tanstack/react-query")>("@tanstack/react-query");
  return {
    ...actual,
    useQuery: (opts: { queryKey?: unknown[] }) => {
      const key = JSON.stringify(opts?.queryKey ?? []);
      if (key.includes("server-settings")) {
        return { data: { maxConcurrentTranscodes: { value: 2, default: 2, source: "default" } }, isLoading: false };
      }
      return { data: undefined, isLoading: false };
    },
  };
});

vi.mock("@/features/admin/EncodeCalibrationCard", () => ({
  EncodeCalibrationCard: () => React.createElement("div", null, "calibration"),
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

async function getPage() {
  const mod = await import("./transcoding.lazy");
  return (mod.Route as unknown as { component: React.ComponentType }).component;
}

const measured = {
  videoSessions: {
    autoCapacity: 3,
    effectiveLimit: 3,
    active: 1,
    evidence: { height: 1440, fps: 54.4, requiredFps: 28.8, ratio: 1.89, cpuBudget: 4, derived: true },
  },
};

beforeEach(() => {
  vi.clearAllMocks();
  mockGetSystemInfo.mockReturnValue({ data: measured });
  mockListConfigEntries.mockReturnValue({ data: [] });
  mockSetConfigEntry.mockResolvedValue({});
});

describe("TranscodingConfigPage capacity", () => {
  it("shows the measured live-playback capacity and its evidence", async () => {
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });

    expect(screen.getByTestId("video-sessions-card")).toBeTruthy();
    const measuredLine = screen.getByTestId("video-sessions-measured").textContent ?? "";
    expect(measuredLine).toContain("3");
    expect(measuredLine).toContain("1");
    expect(screen.getByText(/1440/)).toBeTruthy();
  });

  it("keeps the live-playback capacity and the background job queue as two distinct sections", async () => {
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });

    // The whole point of moving this card here: an operator must not mistake
    // the playback cap for the job-queue cap.
    expect(screen.getByText(/live playback capacity/i)).toBeTruthy();
    expect(screen.getByText(/background transcode jobs/i)).toBeTruthy();
  });

  it("warns when the benchmark could not measure a capacity", async () => {
    mockGetSystemInfo.mockReturnValue({
      data: { videoSessions: { ...measured.videoSessions, evidence: { derived: false } } },
    });
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });
    expect(screen.getByText(/not measured/i)).toBeTruthy();
  });

  it("renders nothing capacity-related when the server does not report it", async () => {
    mockGetSystemInfo.mockReturnValue({ data: {} });
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });
    expect(screen.queryByTestId("video-sessions-measured")).toBeNull();
  });

  it("reflects a stored reduction and saves it under the right key", async () => {
    mockListConfigEntries.mockReturnValue({ data: [{ key: "transcoding.max_video_sessions", value: "2" }] });
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });

    const input = (await screen.findByLabelText(/limit simultaneous video streams/i)) as HTMLInputElement;
    expect(input.value).toBe("2");

    fireEvent.change(input, { target: { value: "1" } });
    fireEvent.click(screen.getByTestId("max-video-sessions-save-btn"));
    await waitFor(() =>
      expect(mockSetConfigEntry).toHaveBeenCalledWith("transcoding.max_video_sessions", { value: "1" }),
    );
  });
});
