import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { StatCard, formatBps, getUsageColor } from "./DashboardStatCard";

// A rate with no measurement yet (-1 on the first poll) is a dash, never a
// misleading 0; each magnitude picks its unit and precision; usage colour
// turns at 70 % and 85 %.
describe("dashboard stat helpers", () => {
  it("formats bits per second by magnitude and dashes what is not measured", () => {
    expect(formatBps(-1)).toBe("-");
    expect(formatBps("-1")).toBe("-");
    expect(formatBps(undefined)).toBe("0 bps");
    expect(formatBps("abc")).toBe("-");
    expect(formatBps(950)).toBe("950 bps");
    expect(formatBps(1500)).toBe("2 kbps");
    expect(formatBps("2500000")).toBe("2.5 Mbps");
    expect(formatBps(1.234e9)).toBe("1.23 Gbps");
  });

  it("colours usage green, warning above 70 and danger above 85", () => {
    expect(getUsageColor(70)).toBe("var(--color-success)");
    expect(getUsageColor(71)).toBe("var(--color-warning)");
    expect(getUsageColor(85)).toBe("var(--color-warning)");
    expect(getUsageColor(86)).toBe("var(--color-danger)");
  });
});

// The tile shows value over label, takes an explicit colour for the value,
// and carries the optional second fact only when given.
describe("StatCard", () => {
  it("renders value, label, colour and the optional sub line", () => {
    const { rerender } = render(<StatCard icon={<i data-testid="icon" />} label="Streams" value={3} valueColor="red" sub="2 transcoded" />);
    expect(screen.getByTestId("icon")).toBeInTheDocument();
    expect(screen.getByText("3").style.color).toBe("red");
    expect(screen.getByText("Streams")).toBeInTheDocument();
    expect(screen.getByText("2 transcoded")).toBeInTheDocument();

    rerender(<StatCard icon={null} label="Users" value="12" />);
    expect(screen.getByText("12").style.color).toBe("var(--text)");
    expect(screen.queryByText("2 transcoded")).toBeNull();
  });
});
