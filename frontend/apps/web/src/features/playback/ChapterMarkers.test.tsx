import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { ChapterMarkers } from "./ChapterMarkers";
import type { Chapter } from "@/hooks/useChapters";

const chapters: Chapter[] = [
  { title: "Intro", startTimeMs: 0, endTimeMs: 60000 },
  { title: "Act 1", startTimeMs: 60000, endTimeMs: 180000 },
  { title: "Act 2", startTimeMs: 180000, endTimeMs: 300000 },
];

describe("ChapterMarkers", () => {
  it("renders nothing when durationSec is 0", () => {
    const { container } = render(
      <ChapterMarkers chapters={chapters} durationSec={0} onSeek={vi.fn()} />,
    );
    expect(container.firstChild).toBeNull();
  });

  it("renders nothing when chapters array is empty", () => {
    const { container } = render(
      <ChapterMarkers chapters={[]} durationSec={300} onSeek={vi.fn()} />,
    );
    expect(container.firstChild).toBeNull();
  });

  it("skips chapter at startTimeMs=0 (no marker at position 0)", () => {
    render(<ChapterMarkers chapters={chapters} durationSec={300} onSeek={vi.fn()} />);
    // Intro starts at 0ms → pct=0 → skipped
    // Act 1 starts at 60s → button rendered
    // Act 2 starts at 180s → button rendered
    const buttons = screen.getAllByRole("button");
    expect(buttons).toHaveLength(2);
  });

  it("renders marker buttons with correct aria-label", () => {
    render(<ChapterMarkers chapters={chapters} durationSec={300} onSeek={vi.fn()} />);
    expect(screen.getByRole("button", { name: "Jump to chapter: Act 1" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Jump to chapter: Act 2" })).toBeInTheDocument();
  });

  it("uses 'Chapter N' fallback when title is empty", () => {
    const noTitle: Chapter[] = [
      { title: "", startTimeMs: 30000, endTimeMs: 60000 },
    ];
    render(<ChapterMarkers chapters={noTitle} durationSec={300} onSeek={vi.fn()} />);
    expect(screen.getByRole("button", { name: "Jump to chapter: Chapter 1" })).toBeInTheDocument();
  });

  it("positions marker at correct percentage", () => {
    render(<ChapterMarkers chapters={chapters} durationSec={300} onSeek={vi.fn()} />);
    const act1Button = screen.getByRole("button", { name: "Jump to chapter: Act 1" });
    // Act 1 startTimeMs=60000 → 60s / 300s * 100 = 20%
    expect(act1Button).toHaveStyle({ left: "20%" });
  });

  it("calls onSeek with correct time in seconds on click", async () => {
    const user = userEvent.setup();
    const onSeek = vi.fn();
    render(<ChapterMarkers chapters={chapters} durationSec={300} onSeek={onSeek} />);
    await user.click(screen.getByRole("button", { name: "Jump to chapter: Act 1" }));
    expect(onSeek).toHaveBeenCalledWith(60);
  });

  it("calls onSeek with correct time for second marker", async () => {
    const user = userEvent.setup();
    const onSeek = vi.fn();
    render(<ChapterMarkers chapters={chapters} durationSec={300} onSeek={onSeek} />);
    await user.click(screen.getByRole("button", { name: "Jump to chapter: Act 2" }));
    expect(onSeek).toHaveBeenCalledWith(180);
  });
});
