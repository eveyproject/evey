import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockGetSystemInfo = vi.fn();
const mockSetConfigEntry = vi.fn();
const mockListConfigEntries = vi.fn();
const mockEveyFetch = vi.fn(() => Promise.resolve({}));

vi.mock("@evey/api", () => ({
  eveyFetch: (...args: unknown[]) => mockEveyFetch(...args),
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
// LoadTestCard's status poll (GET /admin/load-test) is intercepted the same
// way - mockLoadTestStatus lets each test drive idle/running/report directly
// instead of fighting the polling timer.
let mockLoadTestStatus: unknown;

vi.mock("@tanstack/react-query", async () => {
  const actual = await vi.importActual<typeof import("@tanstack/react-query")>("@tanstack/react-query");
  return {
    ...actual,
    useQuery: (opts: { queryKey?: unknown[] }) => {
      const key = JSON.stringify(opts?.queryKey ?? []);
      if (key.includes("server-settings")) {
        return { data: { maxConcurrentTranscodes: { value: 2, default: 2, source: "default" } }, isLoading: false };
      }
      if (key.includes("load-test")) {
        return { data: mockLoadTestStatus, isLoading: false };
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
  mockEveyFetch.mockReset().mockResolvedValue({});
  mockLoadTestStatus = undefined;
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

  it("shows a pool per resource (cpu + gpu) with their own budgets and rung fits when the server reports pools", async () => {
    mockGetSystemInfo.mockReturnValue({
      data: {
        videoSessions: {
          ...measured.videoSessions,
          pools: [
            {
              pool: "cpu",
              budgetUsed: 0.431,
              budgetTotal: 1,
              classes: [
                {
                  ladderClass: "software",
                  rungFits: [{ height: 1080, cost: 0.431, maxStreams: 2 }],
                },
              ],
            },
            {
              pool: "gpu",
              budgetUsed: 0.98,
              budgetTotal: 1,
              classes: [
                {
                  ladderClass: "hw",
                  rungFits: [{ height: 1080, cost: 0.245, maxStreams: 4 }],
                },
              ],
            },
          ],
        },
      },
    });
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });

    expect(screen.getByTestId("video-pool-cpu")).toBeTruthy();
    expect(screen.getByTestId("video-pool-gpu")).toBeTruthy();
    // No stale single-pool view once pools are present.
    expect(screen.queryByTestId("video-sessions-measured")).toBeNull();
    expect(screen.getByText(/processor \(cpu\)/i)).toBeTruthy();
    expect(screen.getByText(/graphics card \(gpu\)/i)).toBeTruthy();
    expect(screen.getByText(/~4 streams? at 1080p/i)).toBeTruthy();
    expect(screen.getByText(/~2 streams? at 1080p/i)).toBeTruthy();
  });

  it("falls back to the aggregate view when the server reports no pools (older server)", async () => {
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });

    expect(screen.queryByTestId("video-pools")).toBeNull();
    expect(screen.getByTestId("video-sessions-measured")).toBeTruthy();
  });

  it("keeps the admin reduction control present but visually secondary to the measured facts", async () => {
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });

    // The auto-adjusts statement and the input both render - the control
    // itself must never be removed, just de-emphasised.
    expect(screen.getByText(/adjusts automatically/i)).toBeTruthy();
    expect(screen.getByLabelText(/limit simultaneous video streams/i)).toBeTruthy();
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

describe("LoadTestCard", () => {
  it("rests with a start button and launches the default (balanced mix) scenario", async () => {
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });

    expect(screen.getByTestId("load-test-idle")).toBeTruthy();
    expect(screen.queryByTestId("load-test-running")).toBeNull();
    expect((screen.getByTestId("load-test-scenario-select") as HTMLSelectElement).value).toBe("balanced");

    fireEvent.click(screen.getByTestId("load-test-start-btn"));

    await waitFor(() =>
      expect(mockEveyFetch).toHaveBeenCalledWith({
        url: "/api/v1/admin/load-test",
        method: "POST",
        data: { scenario: "balanced" },
      }),
    );
  });

  it("offers all 6 scenarios grouped into usage profiles and pure scenarios", async () => {
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });

    const select = screen.getByTestId("load-test-scenario-select");
    const values = Array.from(select.querySelectorAll("option")).map((o) => (o as HTMLOptionElement).value);
    expect(values).toEqual(["light", "balanced", "heavy", "transcode", "mkvgo", "natural"]);
    const groups = Array.from(select.querySelectorAll("optgroup")).map((g) => g.getAttribute("label"));
    expect(groups).toEqual(["Usage profiles", "Pure scenarios"]);
  });

  it("sends the picked usage-profile scenario in the POST payload", async () => {
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });

    fireEvent.change(screen.getByTestId("load-test-scenario-select"), { target: { value: "light" } });
    fireEvent.click(screen.getByTestId("load-test-start-btn"));

    await waitFor(() =>
      expect(mockEveyFetch).toHaveBeenCalledWith({
        url: "/api/v1/admin/load-test",
        method: "POST",
        data: { scenario: "light" },
      }),
    );
  });

  it("sends the picked pure (mkvgo-only) scenario in the POST payload", async () => {
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });

    fireEvent.change(screen.getByTestId("load-test-scenario-select"), { target: { value: "mkvgo" } });
    fireEvent.click(screen.getByTestId("load-test-start-btn"));

    await waitFor(() =>
      expect(mockEveyFetch).toHaveBeenCalledWith({
        url: "/api/v1/admin/load-test",
        method: "POST",
        data: { scenario: "mkvgo" },
      }),
    );
  });

  it("shows the running state (viewers, phase, elapsed) while polling, with a cancel button", async () => {
    mockLoadTestStatus = { running: true, phase: "ramping", viewers: 3, elapsedMs: 15_000 };
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });

    const running = screen.getByTestId("load-test-running");
    expect(running.textContent).toContain("3");
    expect(running.textContent).toContain("ramping");
    expect(running.textContent).toContain("0:15");
    expect(screen.queryByTestId("load-test-idle")).toBeNull();

    fireEvent.click(screen.getByTestId("load-test-cancel-btn"));
    await waitFor(() =>
      expect(mockEveyFetch).toHaveBeenCalledWith({ url: "/api/v1/admin/load-test", method: "DELETE" }),
    );
  });

  it("renders the report with the headline number up front and the stop reason in plain language", async () => {
    mockLoadTestStatus = {
      running: false,
      lastReport: {
        scenario: "transcode",
        sustainedViewers: 5,
        stopReason: "admission_full",
        ttffMsP50: 120,
        ttffMsP95: 300,
        segmentMsP50: 400,
        segmentMsP95: 900,
        segmentSloMs: 1000,
        samples: 42,
        sharedCacheViewers: 2,
      },
    };
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });

    const report = screen.getByTestId("load-test-report");
    expect(report.textContent).toContain("5");
    expect(report.textContent).toMatch(/sustains/i);
    const stopReason = screen.getByTestId("load-test-stop-reason");
    expect(stopReason.textContent).toMatch(/refused the next viewer, as designed/i);
    expect(report.textContent).toMatch(/2 of them served/i);
    // The idle controls stay reachable so the operator can relaunch a test.
    expect(screen.getByTestId("load-test-idle")).toBeTruthy();
  });

  // A viewer count on its own says nothing about WHY the server stopped there.
  // When a resource ran out, that IS the answer: the panel must name it, show
  // the consumption behind it, and say what one more viewer would have cost.
  it("names the bottleneck and reports what the viewers cost", async () => {
    mockLoadTestStatus = {
      running: false,
      lastReport: {
        scenario: "balanced",
        sustainedViewers: 7,
        stopReason: "cpu_saturated",
        stopDetail: "CPU saturated at 96% with 8 viewers",
        resources: {
          cpuPercentAvg: 78,
          cpuPercentPeak: 91,
          memUsedBytesAvg: 2 * 1024 * 1024 * 1024,
          memLimitBytes: 8 * 1024 * 1024 * 1024,
          diskReadBytesPerSec: 12 * 1024 * 1024,
          servedBytesPerSec: 5 * 1024 * 1024,
          samples: 40,
        },
        perViewerCost: {
          cpuPercentAvg: 11,
          memUsedBytesAvg: 64 * 1024 * 1024,
          diskReadBytesPerSec: 2 * 1024 * 1024,
          servedBytesPerSec: 700 * 1024,
          samples: 20,
        },
      },
    };
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });

    const stopReason = screen.getByTestId("load-test-stop-reason");
    expect(stopReason.textContent).toMatch(/CPU/i);

    // What it consumed while holding those viewers.
    const resources = screen.getByTestId("load-test-resources");
    expect(resources.textContent).toContain("78");
    expect(resources.textContent).toContain("91"); // peak
    expect(resources.textContent).toContain("2.0 Go");
    expect(resources.textContent).toContain("8.0 Go"); // the limit it is measured against

    // And what one more viewer would have cost.
    const perViewer = screen.getByTestId("load-test-per-viewer");
    expect(perViewer.textContent).toContain("11");
    expect(perViewer.textContent).toContain("64 Mo");

    // A resource DID give out, so no "raise the ceiling" hint.
    expect(screen.queryByTestId("load-test-no-bottleneck")).toBeNull();
  });

  // The opposite case: the run stopped without any resource giving out (an
  // explicit ceiling passed through the API or CLI - the panel itself never
  // sends one). That is not a capacity result, and the panel must not let it
  // pass for one.
  it("says so when nothing saturated and the run simply stopped", async () => {
    mockLoadTestStatus = {
      running: false,
      lastReport: {
        scenario: "mkvgo",
        sustainedViewers: 12,
        stopReason: "max_viewers",
        resources: { cpuPercentAvg: 20, cpuPercentPeak: 25, samples: 30 },
      },
    };
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });

    expect(screen.getByTestId("load-test-no-bottleneck")).toBeTruthy();
  });

  // A run whose disk barely moved measured the OS page cache, not the server:
  // the library fitted in RAM. The viewer count it produces can be several times
  // the truth (measured: 61 on a dev box whose disk read 11 Mb/s while serving
  // 2032), and an operator planning capacity on it is planning on a mirage.
  it("warns when the result was served from cache rather than disk", async () => {
    mockLoadTestStatus = {
      running: false,
      lastReport: {
        scenario: "mkvgo",
        sustainedViewers: 61,
        stopReason: "throughput_saturated",
        diskReadStarved: true,
        viewerMix: { mkvgo: 61 },
      },
    };
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });
    expect(screen.getByTestId("load-test-disk-starved")).toBeTruthy();
  });

  // A server that reads four bytes to serve one is throwing away three quarters
  // of its storage bandwidth - and on a direct-play box, read bandwidth IS the
  // ceiling, so it is throwing away three quarters of its audience.
  it("warns when the server reads far more than it serves", async () => {
    mockLoadTestStatus = {
      running: false,
      lastReport: {
        scenario: "mkvgo",
        sustainedViewers: 21,
        stopReason: "throughput_saturated",
        readAmplification: 3.9,
        viewerMix: { mkvgo: 21 },
      },
    };
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });
    const warn = screen.getByTestId("load-test-read-amplification");
    expect(warn.textContent).toContain("3.9");
  });

  it("renders the viewer mix composition when the server reports it", async () => {
    mockLoadTestStatus = {
      running: false,
      lastReport: {
        scenario: "balanced",
        sustainedViewers: 5,
        stopReason: "slo_breach",
        viewerMix: { mkvgo: 4, transcode: 1 },
      },
    };
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });

    const mix = screen.getByTestId("load-test-viewer-mix");
    expect(mix.textContent).toContain("4");
    expect(mix.textContent).toMatch(/direct play \(mkvgo\)/i);
    expect(mix.textContent).toContain("1");
    expect(mix.textContent).toMatch(/transcoding/i);
  });

  it("renders skippedCandidates as a discreet line only when greater than zero", async () => {
    mockLoadTestStatus = {
      running: false,
      lastReport: { scenario: "mkvgo", sustainedViewers: 6, stopReason: "max_viewers", skippedCandidates: 3 },
    };
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });

    const skipped = screen.getByTestId("load-test-skipped");
    expect(skipped.textContent).toContain("3");
    expect(skipped.textContent).toMatch(/not eligible for mkvgo/i);
  });

  it("does not render a skippedCandidates line when it is zero or absent", async () => {
    mockLoadTestStatus = {
      running: false,
      lastReport: { scenario: "heavy", sustainedViewers: 2, stopReason: "error", skippedCandidates: 0 },
    };
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });

    expect(screen.queryByTestId("load-test-skipped")).toBeNull();
  });

  it("shows a clear message when the POST is refused because real streams are active", async () => {
    mockEveyFetch.mockRejectedValueOnce(Object.assign(new Error("locked"), { httpStatus: 423 }));
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });

    fireEvent.click(screen.getByTestId("load-test-start-btn"));

    const err = await screen.findByTestId("load-test-error");
    expect(err.textContent).toMatch(/test refuses to run over real viewers/i);
  });

  it("shows the calibration's predicted stream count next to the measured report", async () => {
    mockGetSystemInfo.mockReturnValue({
      data: {
        videoSessions: {
          ...measured.videoSessions,
          pools: [
            {
              pool: "cpu",
              budgetUsed: 0.431,
              budgetTotal: 1,
              classes: [{ ladderClass: "software", rungFits: [{ height: 1080, cost: 0.431, maxStreams: 2 }] }],
            },
            {
              pool: "gpu",
              budgetUsed: 0.98,
              budgetTotal: 1,
              classes: [{ ladderClass: "hw", rungFits: [{ height: 1080, cost: 0.245, maxStreams: 4 }] }],
            },
          ],
        },
      },
    });
    mockLoadTestStatus = {
      running: false,
      lastReport: {
        scenario: "transcode",
        sustainedViewers: 3,
        stopReason: "slo_breach",
        viewerMix: { transcode: 3 },
      },
    };
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });

    const predicted = screen.getByTestId("load-test-predicted");
    expect(predicted.textContent).toContain("4"); // the calibration's prediction
    expect(predicted.textContent).toContain("3"); // the transcodes actually held
  });

  // The calibration predicts ENCODER capacity. Putting "~2 predicted" beside a
  // direct-play result compares an encoder budget to a file server: the numbers
  // measure different things, and showing them together reads as a wild miss.
  it("hides the calibration prediction when the run held no transcodes", async () => {
    mockGetSystemInfo.mockReturnValue({
      data: {
        videoSessions: {
          ...measured.videoSessions,
          pools: [
            {
              pool: "cpu",
              budgetUsed: 0.431,
              budgetTotal: 1,
              classes: [{ ladderClass: "software", rungFits: [{ height: 1080, cost: 0.431, maxStreams: 2 }] }],
            },
          ],
        },
      },
    });
    mockLoadTestStatus = {
      running: false,
      lastReport: {
        scenario: "mkvgo",
        sustainedViewers: 61,
        stopReason: "slo_breach",
        viewerMix: { mkvgo: 61 },
      },
    };
    const Page = await getPage();
    render(React.createElement(Page), { wrapper: createWrapper() });

    expect(screen.queryByTestId("load-test-predicted")).toBeNull();
  });
});
