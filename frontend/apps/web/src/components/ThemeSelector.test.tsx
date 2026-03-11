import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { ThemeSelector } from "./ThemeSelector";

const mockSetTheme = vi.fn();
let mockCurrentTheme = "default";

vi.mock("@evey/store", () => ({
  useThemeStore: () => ({
    theme: mockCurrentTheme,
    setTheme: mockSetTheme,
  }),
}));

vi.mock("@evey/theme", () => ({
  themes: {
    default: {},
    amoled: {},
    "light-default": {},
    "light-blue": {},
  },
}));

describe("ThemeSelector", () => {
  beforeEach(() => {
    mockSetTheme.mockReset();
    mockCurrentTheme = "default";
  });

  it("renders theme options", () => {
    render(<ThemeSelector />);
    expect(screen.getByText("Dark")).toBeInTheDocument();
    expect(screen.getByText("AMOLED")).toBeInTheDocument();
    expect(screen.getByText("Light")).toBeInTheDocument();
    expect(screen.getByText("Light Blue")).toBeInTheDocument();
  });

  it("changes theme on select", async () => {
    const user = userEvent.setup();
    render(<ThemeSelector />);
    await user.click(screen.getByText("AMOLED"));
    expect(mockSetTheme).toHaveBeenCalledWith("amoled");
  });

  it("highlights active theme", () => {
    mockCurrentTheme = "amoled";
    render(<ThemeSelector />);
    const amoledBtn = screen.getByText("AMOLED");
    expect(amoledBtn.className).toMatch(/bg-\[var\(--accent\)\]/);
  });
});
