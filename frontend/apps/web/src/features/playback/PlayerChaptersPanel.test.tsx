import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { ChaptersPanel } from "./PlayerChaptersPanel";

const chapters = [
  { startTimeMs: 0, title: "Intro" },
  { startTimeMs: 60000, title: "Act One" },
  { startTimeMs: 180000, title: "Finale" },
];

const defaultProps = {
  chapters,
  currentTimeSec: 90,
  onSeek: vi.fn(),
  open: false,
  onToggle: vi.fn(),
};

describe("ChaptersPanel", () => {
  it("renders chapters button", () => {
    render(<ChaptersPanel {...defaultProps} />);
    expect(screen.getByRole("button", { name: "Chapters" })).toBeInTheDocument();
  });

  it("shows chapter list when open", () => {
    render(<ChaptersPanel {...defaultProps} open={true} />);
    expect(screen.getByText("Intro")).toBeInTheDocument();
    expect(screen.getByText("Act One")).toBeInTheDocument();
    expect(screen.getByText("Finale")).toBeInTheDocument();
  });

  it("calls onSeek and onToggle when chapter clicked", async () => {
    const user = userEvent.setup();
    const onSeek = vi.fn();
    const onToggle = vi.fn();
    render(<ChaptersPanel {...defaultProps} open={true} onSeek={onSeek} onToggle={onToggle} />);
    await user.click(screen.getByText("Act One"));
    expect(onSeek).toHaveBeenCalledWith(60);
    expect(onToggle).toHaveBeenCalled();
  });

  it("highlights active chapter based on currentTimeSec", () => {
    render(<ChaptersPanel {...defaultProps} open={true} currentTimeSec={90} />);
    const actOneBtn = screen.getByText("Act One").closest("button");
    expect(actOneBtn?.style.color).toBe("var(--accent)");
  });
});
