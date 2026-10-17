import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { SystemTrends, toPoints, formatTick, formatValue, formatAxis } from "./SystemTrends";

// recharts renders real SVG that jsdom cannot measure; the test checks that the
// section composes its charts, filters and notes, not recharts' pixels.
vi.mock("recharts", () => {
  const Passthrough = ({ children }: { children?: React.ReactNode }) => React.createElement("div", null, children);
  return {
    LineChart: Passthrough,
    Line: () => null,
    XAxis: () => null,
    YAxis: () => null,
    Tooltip: () => null,
    Legend: () => null,
    ReferenceLine: ({ y }: { y: number }) => React.createElement("span", { "data-testid": "reference-line" }, String(y)),
    CartesianGrid: () => null,
  };
});

vi.mock("@/hooks/useCssVar", () => ({ useCssVar: (_v: string, fallback?: string) => fallback ?? "#000" }));

const mockList = vi.fn();
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useActivityServiceListServerMetrics: (...args: unknown[]) => mockList(...args),
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

const twoPoints = {
  metrics: [
    { timestamp: "2026-10-15T20:00:00Z", cpuPercent: 10, iowaitPercent: 5, memoryPercent: 40, diskAwaitMs: 7, diskUtilPercent: 10, diskBusiest: "disk A", diskReadBps: "1000", diskWriteBps: "10", networkRxBps: "800", networkTxBps: "1600", bandwidthBps: "1200", storageUsedBytes: "300", storageTotalBytes: "1000", activeStreams: 2, activeTranscodes: 1 },
    { timestamp: "2026-10-15T20:15:00Z", cpuPercent: 30, iowaitPercent: 60, memoryPercent: 45, diskAwaitMs: 40, diskUtilPercent: 70, diskBusiest: "disk B", diskReadBps: "3000", diskWriteBps: "30", networkRxBps: "900", networkTxBps: "1800", bandwidthBps: "1500", storageUsedBytes: "310", storageTotalBytes: "1000", activeStreams: 5, activeTranscodes: 2 },
  ],
  intervalSeconds: 900,
  idleDiskAwaitMs: 7,
  saturationAwaitMs: 20,
};

describe("SystemTrends", () => {
  beforeEach(() => {
    mockList.mockReset();
    mockList.mockReturnValue({ data: twoPoints, isLoading: false, isFetching: false, isError: false });
  });

  it("asks for the last 7 days by default and switches the window on a click", () => {
    render(<SystemTrends />, { wrapper: createWrapper() });
    const [params] = mockList.mock.calls[0] as [{ from: string; to: string }];
    const hours = (Date.parse(params.to) - Date.parse(params.from)) / 3600_000;
    expect(hours).toBeCloseTo(7 * 24, 3);
    expect(screen.getByRole("button", { name: "7 days", pressed: true })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "30 days" }));
    const last = mockList.mock.calls.at(-1)?.[0] as { from: string; to: string };
    expect((Date.parse(last.to) - Date.parse(last.from)) / 3600_000).toBeCloseTo(30 * 24, 3);
  });

  it("draws one chart per unit, with the disk wait's rest and saturation lines", () => {
    render(<SystemTrends />, { wrapper: createWrapper() });
    for (const id of ["cpu", "memory", "diskAwait", "diskUtil", "diskRate", "network", "streams", "storage"]) {
      expect(screen.getByTestId(`trend-${id}`)).toBeInTheDocument();
    }
    expect(screen.getByText("CPU and disk wait (%)")).toBeInTheDocument();
    const lines = screen.getAllByTestId("reference-line").map((el) => el.textContent);
    expect(lines).toEqual(["7", "20"]);
    expect(screen.getByText("one point every 15 min")).toBeInTheDocument();
    expect(screen.getByText(/whole machine's/)).toBeInTheDocument();
  });

  it("says the history is short instead of drawing a single point", () => {
    mockList.mockReturnValue({ data: { metrics: twoPoints.metrics.slice(0, 1), intervalSeconds: 900 }, isLoading: false, isFetching: false, isError: false });
    render(<SystemTrends />, { wrapper: createWrapper() });
    expect(screen.getByText(/Not enough history yet/)).toBeInTheDocument();
    expect(screen.queryByTestId("trend-cpu")).not.toBeInTheDocument();
  });

  it("keeps the previous charts dimmed while a new window loads", () => {
    mockList.mockReturnValue({ data: twoPoints, isLoading: false, isFetching: true, isError: false });
    render(<SystemTrends />, { wrapper: createWrapper() });
    expect(screen.getByTestId("trend-cpu").parentElement?.className).toContain("opacity-60");
  });

  it("maps the response into chart points, storage as a share and int64 strings as numbers", () => {
    const pts = toPoints(twoPoints.metrics);
    expect(pts).toHaveLength(2);
    expect(pts[1].diskRead).toBe(3000);
    expect(pts[1].netTx).toBe(1800);
    expect(pts[1].storage).toBeCloseTo(31);
    expect(pts[1].diskBusiest).toBe("disk B");
    expect(toPoints([{ timestamp: "garbage" }])).toEqual([]);
  });

  it("labels ticks by the window and values by their unit", () => {
    const t = new Date(2026, 9, 15, 20, 5).getTime();
    expect(formatTick("24h", t)).toBe("20:05");
    expect(formatTick("7d", t)).toBe("15/10 20:05");
    expect(formatTick("30d", t)).toBe("15/10");
    expect(formatValue("percent", 12.34)).toBe("12.3 %");
    expect(formatValue("ms", 7)).toBe("7.0 ms");
    expect(formatValue("bps", 8_000_000)).toBe("8.0 Mbps");
    expect(formatValue("bytesps", 73400320)).toBe("73.4 MB/s");
    expect(formatValue("count", 2.6)).toBe("3");
    // Axis ticks drop the "/s" so a rate fits the gutter on one line.
    expect(formatAxis("bps", 80_000_000)).toBe("80 Mb");
    expect(formatAxis("bytesps", 3_800_000)).toBe("3.8 MB");
    expect(formatAxis("bytesps", 950_000)).toBe("950 kB");
    expect(formatAxis("percent", 12.6)).toBe("13");
  });
});
