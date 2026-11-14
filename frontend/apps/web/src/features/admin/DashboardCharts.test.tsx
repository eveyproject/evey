import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import React from "react";
import { PlayActivityChart, PeakHours } from "./DashboardCharts";

// recharts is mocked to simple markup: it renders real SVG that trips jsdom
// (getBBox/getComputedTextLength are unimplemented). The point of this test is
// that the components integrate the chart lib without throwing and still render
// their chrome (titles, toggles, legend) - not to pixel-check recharts itself.
const chartWidths = vi.hoisted(() => [] as number[]);
vi.mock("recharts", () => {
  const Passthrough = ({ children }: { children?: React.ReactNode }) => React.createElement("div", null, children);
  return {
    BarChart: ({ children, width }: { children?: React.ReactNode; width: number }) => {
      chartWidths.push(width);
      return React.createElement("div", null, children);
    },
    Bar: () => null,
    XAxis: () => null,
    YAxis: () => null,
    CartesianGrid: () => null,
    Tooltip: () => null,
    PieChart: Passthrough,
    Pie: ({ children }: { children?: React.ReactNode }) => React.createElement("div", null, children),
    Cell: () => null,
  };
});

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

let analytics: { dailyPlays: unknown[]; hourlyPlays: unknown[] } | undefined = {
  dailyPlays: [{ date: "2026-08-01", playCount: 5, watchTimeSeconds: "3600" }],
  hourlyPlays: [{ hour: 20, count: 3 }],
};
vi.mock("@evey/api", () => ({
  useActivityServiceGetAnalytics: () => ({ data: analytics }),
  formatBytes: (b?: string) => `${b ?? 0} B`,
}));

describe("DashboardCharts (recharts)", () => {
  it("renders PlayActivityChart with its title and mode toggles", () => {
    render(<PlayActivityChart />);
    expect(screen.getByText("admin.dashboardStats.playActivity")).toBeTruthy();
    expect(screen.getByText("admin.dashboardStats.plays")).toBeTruthy();
    expect(screen.getByText("admin.dashboardStats.watchTime")).toBeTruthy();
    // The chart is exposed to screen readers as a labelled image.
    expect(screen.getByRole("img", { name: "admin.dashboardStats.playActivity" })).toBeTruthy();
  });

  // The chart wrapper only mounts once the analytics arrive: the width must be
  // measured then, not on the first render (the chart stayed at its minimum
  // width, half the card on a wide screen).
  it("sizes the chart to its card once the data arrives after the first render", () => {
    const saved = analytics;
    analytics = undefined;
    const widthSpy = vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(1200);
    try {
      const { rerender } = render(<PlayActivityChart />);
      expect(screen.queryByRole("img")).toBeNull();
      analytics = saved;
      chartWidths.length = 0;
      rerender(<PlayActivityChart />);
      expect(chartWidths.at(-1)).toBe(1200);
    } finally {
      widthSpy.mockRestore();
      analytics = saved;
    }
  });

  it("renders PeakHours with its title", () => {
    render(<PeakHours />);
    expect(screen.getByText("admin.dashboardStats.peakHours")).toBeTruthy();
  });

});
