import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { ChapterMarkers } from "./ChapterMarkers";
import type { Chapter } from "@/hooks/useChapters";

const chapters: Chapter[] = [
  { title: "Intro", startTimeMs: 0, endTimeMs: 60000 },
  { title: "Act 1", startTimeMs: 60000, endTimeMs: 180000 },
  { title: "Act 2", startTimeMs: 180000, endTimeMs: 300000 },
];

// Chapters DIVIDE the progress bar. They do not own the click, they do not paint
// it, and they say nothing about the playhead. Two real bugs came from forgetting
// that, and these tests pin the contract that came out of them.
describe("ChapterMarkers", () => {
  it("renders nothing when durationSec is 0", () => {
    const { container } = render(<ChapterMarkers chapters={chapters} durationSec={0} />);
    expect(container.firstChild).toBeNull();
  });

  it("renders nothing when chapters array is empty", () => {
    const { container } = render(<ChapterMarkers chapters={[]} durationSec={300} />);
    expect(container.firstChild).toBeNull();
  });

  // The chapter markers used to sit ABOVE the seek input and swallow its clicks:
  // clicking mid-bar jumped to the START of the chapter instead of seeking where
  // you actually clicked. Nothing here may be clickable.
  it("never takes the click: the seek input owns the whole bar", () => {
    const { container } = render(<ChapterMarkers chapters={chapters} durationSec={300} />);

    expect(screen.queryAllByRole("button")).toHaveLength(0);
    expect(container.querySelector(".pointer-events-none")).toBeInTheDocument();
  });

  // One notch per BOUNDARY - the first chapter starts at 0, and a notch on the
  // bar's own edge is noise.
  it("draws one notch per chapter boundary, none at the edges", () => {
    const { container } = render(<ChapterMarkers chapters={chapters} durationSec={300} />);

    const notches = container.querySelectorAll("span");
    expect(notches).toHaveLength(2); // Act 1 @ 20%, Act 2 @ 60% - Intro @ 0% dropped

    expect(notches[0]).toHaveStyle({ left: "20%" });
    expect(notches[1]).toHaveStyle({ left: "60%" });
  });

  // The purple line a chapter once drew under the bar reads as "this segment is
  // preloaded" in the player's own visual language - it must never mean "chapter".
  it("paints nothing under or across the bar", () => {
    const { container } = render(<ChapterMarkers chapters={chapters} durationSec={300} />);

    for (const span of container.querySelectorAll("span")) {
      // A notch is a hairline, never a block spanning the chapter's width.
      const style = span.getAttribute("style") ?? "";
      expect(style).not.toContain("width");
      expect(span.className).not.toContain("bottom-");
    }
  });
});
