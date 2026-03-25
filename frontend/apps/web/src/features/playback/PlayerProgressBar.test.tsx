import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { PlayerProgressBar } from "./PlayerProgressBar";

vi.mock("./TrickplayPreview", () => ({
  TrickplayPreview: () => null,
}));

const defaultProps = {
  absoluteTime: 30,
  effectiveDuration: 120,
  bufferedEnd: 60,
  transcodeOffset: 0,
  chapters: [],
  onSeek: vi.fn(),
  onChapterSeek: vi.fn(),
};

describe("PlayerProgressBar", () => {
  it("renders seek slider", () => {
    render(<PlayerProgressBar {...defaultProps} />);
    expect(screen.getByRole("slider", { name: "Seek" })).toBeInTheDocument();
  });

  it("sets slider max to effective duration", () => {
    render(<PlayerProgressBar {...defaultProps} />);
    const slider = screen.getByRole("slider", { name: "Seek" }) as HTMLInputElement;
    expect(slider.max).toBe("120");
  });

  it("sets slider value to absolute time", () => {
    render(<PlayerProgressBar {...defaultProps} absoluteTime={45} />);
    const slider = screen.getByRole("slider", { name: "Seek" }) as HTMLInputElement;
    expect(slider.value).toBe("45");
  });
});
