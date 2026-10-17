import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

// The section composes range buttons, one chart per unit and the notes; the
// chart library itself is mocked to the props it receives.
vi.mock("react-native", () => ({
  View: ({ children, testID }: any) => React.createElement("div", { "data-testid": testID }, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Pressable: ({ children, onPress, accessibilityState }: any) =>
    React.createElement("button", { onClick: onPress, "aria-pressed": accessibilityState?.selected }, children),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
  useWindowDimensions: () => ({ width: 400, height: 800 }),
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: ({ name }: any) => React.createElement("i", { "data-icon": name }) }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
const charts = vi.hoisted(() => ({ props: [] as any[] }));
vi.mock("react-native-gifted-charts", () => ({
  LineChart: (props: any) => {
    charts.props.push(props);
    return React.createElement("div", { "data-testid": "line-chart", "data-ref1": props.referenceLine1Position, "data-ref2": props.referenceLine2Position });
  },
}));
const q = vi.hoisted(() => ({ data: undefined as unknown, isLoading: false, isFetching: false, isError: false, params: null as any }));
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useActivityServiceListServerMetrics: (params: any) => {
    q.params = params;
    return q;
  },
}));

import { SystemTrends, toPoints, formatTick, formatAxis } from "./SystemTrends";

const theme = { colors: { bg: "#000", surface: "#111", text: "#fff", textMuted: "#888", accent: "#f00", border: "#333", danger: "#d00", warning: "#fa0", success: "#0a0" }, radii: { md: 8 } } as any;

const twoPoints = {
  metrics: [
    { timestamp: "2026-10-15T20:00:00Z", cpuPercent: 10, iowaitPercent: 5, memoryPercent: 40, diskAwaitMs: 7, diskUtilPercent: 10, diskReadBps: "1000", diskWriteBps: "10", networkRxBps: "800", networkTxBps: "1600", bandwidthBps: "1200", storageUsedBytes: "300", storageTotalBytes: "1000", activeStreams: 2, activeTranscodes: 1 },
    { timestamp: "2026-10-15T20:15:00Z", cpuPercent: 30, iowaitPercent: 60, memoryPercent: 45, diskAwaitMs: 40, diskUtilPercent: 70, diskReadBps: "3000", diskWriteBps: "30", networkRxBps: "900", networkTxBps: "1800", bandwidthBps: "1500", storageUsedBytes: "310", storageTotalBytes: "1000", activeStreams: 5, activeTranscodes: 2 },
  ],
  intervalSeconds: 900,
  idleDiskAwaitMs: 7,
  saturationAwaitMs: 20,
};

beforeEach(() => {
  charts.props = [];
  q.data = twoPoints;
  q.isLoading = false;
  q.isFetching = false;
  q.isError = false;
});

describe("SystemTrends (mobile)", () => {
  it("asks for 7 days by default, draws one chart per unit and the disk wait's two reference lines", () => {
    render(<SystemTrends theme={theme} />);
    const hours = (Date.parse(q.params.to) - Date.parse(q.params.from)) / 3600_000;
    expect(hours).toBeCloseTo(7 * 24, 3);
    expect(screen.getAllByTestId("line-chart")).toHaveLength(8);
    const awaitChart = charts.props.find((p) => p.referenceLine1Position !== undefined);
    expect(awaitChart.referenceLine1Position).toBe(7);
    expect(awaitChart.referenceLine2Position).toBe(20);
    // Three series on the network chart, in fixed colour order.
    const network = charts.props.find((p) => p.data3 !== undefined);
    expect(network.data.map((d: any) => d.value)).toEqual([800, 900]);
    expect(network.data3.map((d: any) => d.value)).toEqual([1200, 1500]);
    expect(screen.getByText("admin.systemHealth.trendsBucket")).toBeTruthy();
  });

  it("switches the window on a tap", () => {
    render(<SystemTrends theme={theme} />);
    fireEvent.click(screen.getByText("admin.systemHealth.trendsRange_24h"));
    expect((Date.parse(q.params.to) - Date.parse(q.params.from)) / 3600_000).toBeCloseTo(24, 3);
  });

  it("says the history is short instead of drawing a single point", () => {
    q.data = { metrics: twoPoints.metrics.slice(0, 1), intervalSeconds: 900 };
    render(<SystemTrends theme={theme} />);
    expect(screen.getByText("admin.systemHealth.trendsEmpty")).toBeTruthy();
    expect(screen.queryByTestId("line-chart")).toBeNull();
  });

  it("maps the response and shortens axis labels", () => {
    const pts = toPoints(twoPoints.metrics);
    expect(pts[1].storage).toBeCloseTo(31);
    expect(pts[1].netTx).toBe(1800);
    expect(toPoints([{ timestamp: "garbage" }])).toEqual([]);
    const t = new Date(2026, 9, 15, 20, 5).getTime();
    expect(formatTick("24h", t)).toBe("20:05");
    expect(formatTick("30d", t)).toBe("15/10");
    expect(formatAxis("bps", 8_000_000)).toBe("8M");
    expect(formatAxis("bytesps", 73400320)).toBe("73.4 MB");
    expect(formatAxis("percent", 12.6)).toBe("13");
  });
});
