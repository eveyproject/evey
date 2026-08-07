import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { useQuery } from "@tanstack/react-query";
import { SkipIntroButton } from "./SkipIntroButton";
import type { MediaSegment } from "@/hooks/useMediaSegments";

vi.mock("@evey/api", () => ({
  mKVServiceListChapters: vi.fn(),
}));

vi.mock("@tanstack/react-query", () => ({
  useQuery: vi.fn(),
}));

vi.mock("@/hooks/useMediaSegments", () => ({
  useMediaSegments: vi.fn(),
}));

const toastInfoMock = vi.fn();
vi.mock("@evey/store", () => ({
  toast: { info: (...args: unknown[]) => toastInfoMock(...args) },
}));

import { useMediaSegments } from "@/hooks/useMediaSegments";

// segData mirrors the hook's response envelope; autoSkip defaults to the
// server default (off).
function segData(segments: MediaSegment[], introAutoSkip = false) {
  return { segments, introAutoSkip };
}

const mockUseQuery = useQuery as ReturnType<typeof vi.fn>;
const mockUseMediaSegments = useMediaSegments as ReturnType<typeof vi.fn>;

const introChapter = {
  title: "Intro",
  startMs: "5000",
  endMs: "90000",
};

const introSegment: MediaSegment = {
  type: "intro",
  startMs: 10000,
  endMs: 80000,
  confidence: 0.95,
  source: "audio",
};

describe("SkipIntroButton", () => {
  beforeEach(() => {
    mockUseQuery.mockReset();
    mockUseMediaSegments.mockReset();
    toastInfoMock.mockReset();
    // Default: no segments from API
    mockUseMediaSegments.mockReturnValue({ data: segData([]) });
  });

  // --- Original MKV chapter tests (adapted for new mediaId prop) ---

  it("renders Skip Intro button when in intro chapter (MKV fallback)", () => {
    mockUseMediaSegments.mockReturnValue({ data: segData([]) });
    mockUseQuery.mockReturnValue({ data: { chapters: [introChapter] } });
    render(
      <SkipIntroButton
        filePath="/media/ep1.mkv"
        currentTime={30}
        onSkip={vi.fn()}
      />,
    );
    expect(screen.getByText("Skip Intro")).toBeInTheDocument();
  });

  it("returns null when no intro chapter (MKV fallback)", () => {
    mockUseMediaSegments.mockReturnValue({ data: segData([]) });
    mockUseQuery.mockReturnValue({
      data: { chapters: [{ title: "Main", startMs: "0", endMs: "90000" }] },
    });
    const { container } = render(
      <SkipIntroButton filePath="/media/ep1.mkv" currentTime={30} onSkip={vi.fn()} />,
    );
    expect(container.firstChild).toBeNull();
  });

  it("returns null when currentTime is before intro start (MKV fallback)", () => {
    mockUseMediaSegments.mockReturnValue({ data: segData([]) });
    mockUseQuery.mockReturnValue({ data: { chapters: [introChapter] } });
    const { container } = render(
      <SkipIntroButton
        filePath="/media/ep1.mkv"
        currentTime={2}
        onSkip={vi.fn()}
      />,
    );
    expect(container.firstChild).toBeNull();
  });

  it("returns null when currentTime is at or past intro end (MKV fallback)", () => {
    mockUseMediaSegments.mockReturnValue({ data: segData([]) });
    mockUseQuery.mockReturnValue({ data: { chapters: [introChapter] } });
    const { container } = render(
      <SkipIntroButton
        filePath="/media/ep1.mkv"
        currentTime={90}
        onSkip={vi.fn()}
      />,
    );
    expect(container.firstChild).toBeNull();
  });

  it("calls onSkip with endSec on click (MKV fallback)", async () => {
    const user = userEvent.setup();
    const onSkip = vi.fn();
    mockUseMediaSegments.mockReturnValue({ data: segData([]) });
    mockUseQuery.mockReturnValue({ data: { chapters: [introChapter] } });
    render(
      <SkipIntroButton
        filePath="/media/ep1.mkv"
        currentTime={30}
        onSkip={onSkip}
      />,
    );
    await user.click(screen.getByText("Skip Intro"));
    expect(onSkip).toHaveBeenCalledWith(90);
  });

  // --- API segments as priority ---

  it("uses API segment when available, ignoring MKV chapters", () => {
    mockUseMediaSegments.mockReturnValue({ data: segData([introSegment]) });
    // MKV chapters should not be consulted, but mock just in case
    mockUseQuery.mockReturnValue({ data: { chapters: [introChapter] } });
    render(
      <SkipIntroButton
        mediaId="media-123"
        filePath="/media/ep1.mkv"
        currentTime={30}
        onSkip={vi.fn()}
      />,
    );
    expect(screen.getByText("Skip Intro")).toBeInTheDocument();
  });

  it("calls onSkip with API segment endSec when API segment active", async () => {
    const user = userEvent.setup();
    const onSkip = vi.fn();
    mockUseMediaSegments.mockReturnValue({ data: segData([introSegment]) });
    mockUseQuery.mockReturnValue({ data: { chapters: [] } });
    render(
      <SkipIntroButton
        mediaId="media-123"
        currentTime={30}
        onSkip={onSkip}
      />,
    );
    await user.click(screen.getByText("Skip Intro"));
    // endMs=80000ms → 80s
    expect(onSkip).toHaveBeenCalledWith(80);
  });

  it("returns null when currentTime is outside API segment window", () => {
    mockUseMediaSegments.mockReturnValue({ data: segData([introSegment]) });
    mockUseQuery.mockReturnValue({ data: { chapters: [] } });
    const { container } = render(
      <SkipIntroButton
        mediaId="media-123"
        currentTime={5}
        onSkip={vi.fn()}
      />,
    );
    // introSegment starts at 10s, so currentTime=5 is before it
    expect(container.firstChild).toBeNull();
  });

  it("falls back to MKV chapters when API returns empty segments", () => {
    mockUseMediaSegments.mockReturnValue({ data: segData([]) });
    mockUseQuery.mockReturnValue({ data: { chapters: [introChapter] } });
    render(
      <SkipIntroButton
        mediaId="media-123"
        filePath="/media/ep1.mkv"
        currentTime={30}
        onSkip={vi.fn()}
      />,
    );
    expect(screen.getByText("Skip Intro")).toBeInTheDocument();
  });

  it("falls back to MKV chapters when API returns undefined segments", () => {
    mockUseMediaSegments.mockReturnValue({ data: undefined });
    mockUseQuery.mockReturnValue({ data: { chapters: [introChapter] } });
    render(
      <SkipIntroButton
        mediaId="media-123"
        filePath="/media/ep1.mkv"
        currentTime={30}
        onSkip={vi.fn()}
      />,
    );
    expect(screen.getByText("Skip Intro")).toBeInTheDocument();
  });

  it("returns null when API segment is shorter than 5 seconds", () => {
    mockUseMediaSegments.mockReturnValue({
      data: segData([{ ...introSegment, startMs: 1000, endMs: 4000 }]),
    });
    mockUseQuery.mockReturnValue({ data: { chapters: [] } });
    const { container } = render(
      <SkipIntroButton mediaId="media-123" currentTime={2} onSkip={vi.fn()} />,
    );
    expect(container.firstChild).toBeNull();
  });

  it("returns null when MKV chapter intro is shorter than 5 seconds", () => {
    mockUseMediaSegments.mockReturnValue({ data: segData([]) });
    mockUseQuery.mockReturnValue({
      data: { chapters: [{ title: "Intro", startMs: "1000", endMs: "4000" }] },
    });
    const { container } = render(
      <SkipIntroButton filePath="/media/ep1.mkv" currentTime={2} onSkip={vi.fn()} />,
    );
    expect(container.firstChild).toBeNull();
  });

  // --- recap + end_credits (generalized skip) ---

  const recapSegment: MediaSegment = { type: "recap", startMs: 0, endMs: 30000, confidence: 0.9, source: "audio" };
  const creditsSegment: MediaSegment = { type: "end_credits", startMs: 1800000, endMs: 1860000, confidence: 0.9, source: "audio" };

  it("renders Skip Recap when the playhead is inside a recap segment", () => {
    mockUseMediaSegments.mockReturnValue({ data: segData([recapSegment]) });
    mockUseQuery.mockReturnValue({ data: { chapters: [] } });
    render(<SkipIntroButton mediaId="m1" currentTime={10} onSkip={vi.fn()} />);
    expect(screen.getByText("Skip Recap")).toBeInTheDocument();
  });

  it("renders Skip Credits when the playhead is inside an end_credits segment", () => {
    mockUseMediaSegments.mockReturnValue({ data: segData([creditsSegment]) });
    mockUseQuery.mockReturnValue({ data: { chapters: [] } });
    render(<SkipIntroButton mediaId="m1" currentTime={1810} onSkip={vi.fn()} />);
    expect(screen.getByText("Skip Credits")).toBeInTheDocument();
  });

  it("picks the segment the playhead is inside, not just the first (intro shadow bug)", () => {
    // intro at the start + end_credits at the end. Playhead is in credits.
    // The old find(first skippable) resolved intro, then the currentTime
    // guard hid the button entirely - credits were never skippable.
    mockUseMediaSegments.mockReturnValue({ data: segData([introSegment, creditsSegment]) });
    mockUseQuery.mockReturnValue({ data: { chapters: [] } });
    render(<SkipIntroButton mediaId="m1" currentTime={1810} onSkip={vi.fn()} />);
    expect(screen.getByText("Skip Credits")).toBeInTheDocument();
    expect(screen.queryByText("Skip Intro")).not.toBeInTheDocument();
  });

  it("skips to the active segment end on click (credits)", async () => {
    const user = userEvent.setup();
    const onSkip = vi.fn();
    mockUseMediaSegments.mockReturnValue({ data: segData([creditsSegment]) });
    mockUseQuery.mockReturnValue({ data: { chapters: [] } });
    render(<SkipIntroButton mediaId="m1" currentTime={1810} onSkip={onSkip} />);
    await user.click(screen.getByText("Skip Credits"));
    expect(onSkip).toHaveBeenCalledWith(1860); // endMs 1860000 → 1860s
  });

  it("recognizes a recap chapter via the MKV fallback", () => {
    mockUseMediaSegments.mockReturnValue({ data: segData([]) });
    mockUseQuery.mockReturnValue({
      data: { chapters: [{ title: "Previously on...", startMs: "0", endMs: "30000" }] },
    });
    render(<SkipIntroButton filePath="/media/ep1.mkv" currentTime={10} onSkip={vi.fn()} />);
    expect(screen.getByText("Skip Recap")).toBeInTheDocument();
  });

  it("recognizes an end-credits chapter via the MKV fallback", () => {
    mockUseMediaSegments.mockReturnValue({ data: segData([]) });
    mockUseQuery.mockReturnValue({
      data: { chapters: [{ title: "End Credits", startMs: "1800000", endMs: "1860000" }] },
    });
    render(<SkipIntroButton filePath="/media/ep1.mkv" currentTime={1810} onSkip={vi.fn()} />);
    expect(screen.getByText("Skip Credits")).toBeInTheDocument();
  });

  // --- Auto-skip (server admin toggle) ---

  it("auto-skips an intro without a click when the server flag is on", () => {
    const onSkip = vi.fn();
    mockUseMediaSegments.mockReturnValue({ data: segData([introSegment], true) });
    mockUseQuery.mockReturnValue({ data: undefined });
    render(<SkipIntroButton mediaId="m1" currentTime={30} onSkip={onSkip} />);
    expect(onSkip).toHaveBeenCalledWith(80);
    expect(toastInfoMock).toHaveBeenCalledTimes(1);
  });

  it("never auto-skips when the server flag is off", () => {
    const onSkip = vi.fn();
    mockUseMediaSegments.mockReturnValue({ data: segData([introSegment], false) });
    mockUseQuery.mockReturnValue({ data: undefined });
    render(<SkipIntroButton mediaId="m1" currentTime={30} onSkip={onSkip} />);
    expect(onSkip).not.toHaveBeenCalled();
    expect(screen.getByText("Skip Intro")).toBeInTheDocument();
  });

  it("never auto-skips a recap or the credits, even with the flag on", () => {
    const onSkip = vi.fn();
    mockUseMediaSegments.mockReturnValue({ data: segData([recapSegment, creditsSegment], true) });
    mockUseQuery.mockReturnValue({ data: undefined });
    render(<SkipIntroButton mediaId="m1" currentTime={10} onSkip={onSkip} />);
    expect(onSkip).not.toHaveBeenCalled();
    expect(screen.getByText("Skip Recap")).toBeInTheDocument();
  });

  it("auto-skips once per segment - seeking back into the intro gets the button, not another jump", () => {
    const onSkip = vi.fn();
    mockUseMediaSegments.mockReturnValue({ data: segData([introSegment], true) });
    mockUseQuery.mockReturnValue({ data: undefined });
    const { rerender } = render(<SkipIntroButton mediaId="m1" currentTime={30} onSkip={onSkip} />);
    expect(onSkip).toHaveBeenCalledTimes(1);
    // Viewer seeks back into the intro of the SAME media.
    rerender(<SkipIntroButton mediaId="m1" currentTime={20} onSkip={onSkip} />);
    expect(onSkip).toHaveBeenCalledTimes(1);
    expect(screen.getByText("Skip Intro")).toBeInTheDocument();
  });
});
