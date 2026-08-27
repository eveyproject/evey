import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import React from "react";
import { PlayActivityChart, StorageDonut, PeakHours } from "./DashboardCharts";

// recharts is mocked to simple markup: it renders real SVG that trips jsdom
// (getBBox/getComputedTextLength are unimplemented). The point of this test is
// that the components integrate the chart lib without throwing and still render
// their chrome (titles, toggles, legend) - not to pixel-check recharts itself.
vi.mock("recharts", () => {
  const Passthrough = ({ children }: { children?: React.ReactNode }) => React.createElement("div", null, children);
  return {
    BarChart: Passthrough,
    Bar: () => null,
    XAxis: () => null,
    YAxis: () => null,
    PieChart: Passthrough,
    Pie: ({ children }: { children?: React.ReactNode }) => React.createElement("div", null, children),
    Cell: () => null,
  };
});

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock("@/hooks/useCssVar", () => ({ useCssVar: (_v: string, fallback?: string) => fallback ?? "#000" }));

const analytics = {
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
  });

  it("renders PeakHours with its title", () => {
    render(<PeakHours />);
    expect(screen.getByText("admin.dashboardStats.peakHours")).toBeTruthy();
  });

  it("renders StorageDonut with the used percentage and legend", () => {
    render(<StorageDonut storage={[{ path: "/data", totalBytes: "100", usedBytes: "40" }]} />);
    expect(screen.getByText("40%")).toBeTruthy();
    expect(screen.getByText("admin.dashboardStats.used")).toBeTruthy();
    expect(screen.getByText("admin.dashboardStats.free")).toBeTruthy();
  });
});
