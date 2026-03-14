import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { ThemeSettings } from "./ThemeSettings";

vi.mock("@/components/ThemeSelector", () => ({
  ThemeSelector: () => <div data-testid="theme-selector">ThemeSelector</div>,
}));

describe("ThemeSettings", () => {
  it("renders Appearance heading", () => {
    render(<ThemeSettings />);
    expect(screen.getByText("Appearance")).toBeInTheDocument();
  });

  it("renders theme selector", () => {
    render(<ThemeSettings />);
    expect(screen.getByTestId("theme-selector")).toBeInTheDocument();
  });

  it("renders description", () => {
    render(<ThemeSettings />);
    expect(screen.getByText(/color theme/i)).toBeInTheDocument();
  });
});
