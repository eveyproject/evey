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
      <ChapterMarkers chapters={chapters} durationSec={0} currentTimeSec={0} onSeek={vi.fn()} />,
    );
    expect(container.firstChild).toBeNull();
  });

  it("renders nothing when chapters array is empty", () => {
    const { container } = render(
      <ChapterMarkers chapters={[]} durationSec={300} currentTimeSec={0} onSeek={vi.fn()} />,
    );
    expect(container.firstChild).toBeNull();
  });

  it("renders a button for each chapter segment", () => {
    render(<ChapterMarkers chapters={chapters} durationSec={300} currentTimeSec={0} onSeek={vi.fn()} />);
    const buttons = screen.getAllByRole("button");
    expect(buttons).toHaveLength(3);
  });

  it("renders segment buttons with correct aria-label", () => {
    render(<ChapterMarkers chapters={chapters} durationSec={300} currentTimeSec={0} onSeek={vi.fn()} />);
    expect(screen.getByRole("button", { name: "Jump to chapter: Intro" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Jump to chapter: Act 1" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Jump to chapter: Act 2" })).toBeInTheDocument();
  });

  it("positions segment at correct percentage with correct width", () => {
    render(<ChapterMarkers chapters={chapters} durationSec={300} currentTimeSec={0} onSeek={vi.fn()} />);
    const act1Button = screen.getByRole("button", { name: "Jump to chapter: Act 1" });
    // Act 1: start=60s/300s=20%, end=180s/300s=60%, width=40%
    expect(act1Button).toHaveStyle({ left: "20%", width: "40%" });
  });

  it("highlights active chapter segment", () => {
    // currentTimeSec=90 → inside Act 1 (60s–180s)
    render(<ChapterMarkers chapters={chapters} durationSec={300} currentTimeSec={90} onSeek={vi.fn()} />);
    const act1Button = screen.getByRole("button", { name: "Jump to chapter: Act 1" });
    const span = act1Button.querySelector("span");
    expect(span).toHaveStyle({ backgroundColor: "var(--accent)", opacity: "0.5" });
  });

  it("calls onSeek with chapter start time on click", async () => {
    const user = userEvent.setup();
    const onSeek = vi.fn();
    render(<ChapterMarkers chapters={chapters} durationSec={300} currentTimeSec={0} onSeek={onSeek} />);
    await user.click(screen.getByRole("button", { name: "Jump to chapter: Act 1" }));
    expect(onSeek).toHaveBeenCalledWith(60);
  });

  it("calls onSeek with correct time for Act 2", async () => {
    const user = userEvent.setup();
    const onSeek = vi.fn();
    render(<ChapterMarkers chapters={chapters} durationSec={300} currentTimeSec={0} onSeek={onSeek} />);
    await user.click(screen.getByRole("button", { name: "Jump to chapter: Act 2" }));
    expect(onSeek).toHaveBeenCalledWith(180);
  });
});
