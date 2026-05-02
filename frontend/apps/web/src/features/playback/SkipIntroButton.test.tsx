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

import { useMediaSegments } from "@/hooks/useMediaSegments";

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
    // Default: no segments from API
    mockUseMediaSegments.mockReturnValue({ data: [] });
  });

  // --- Original MKV chapter tests (adapted for new mediaId prop) ---

  it("renders Skip Intro button when in intro chapter (MKV fallback)", () => {
    mockUseMediaSegments.mockReturnValue({ data: [] });
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
    mockUseMediaSegments.mockReturnValue({ data: [] });
    mockUseQuery.mockReturnValue({
      data: { chapters: [{ title: "Main", startMs: "0", endMs: "90000" }] },
    });
    const { container } = render(
      <SkipIntroButton filePath="/media/ep1.mkv" currentTime={30} onSkip={vi.fn()} />,
    );
    expect(container.firstChild).toBeNull();
  });

  it("returns null when currentTime is before intro start (MKV fallback)", () => {
    mockUseMediaSegments.mockReturnValue({ data: [] });
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
    mockUseMediaSegments.mockReturnValue({ data: [] });
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
    mockUseMediaSegments.mockReturnValue({ data: [] });
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
    mockUseMediaSegments.mockReturnValue({ data: [introSegment] });
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
    mockUseMediaSegments.mockReturnValue({ data: [introSegment] });
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
    mockUseMediaSegments.mockReturnValue({ data: [introSegment] });
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
    mockUseMediaSegments.mockReturnValue({ data: [] });
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
      data: [{ ...introSegment, startMs: 1000, endMs: 4000 }],
    });
    mockUseQuery.mockReturnValue({ data: { chapters: [] } });
    const { container } = render(
      <SkipIntroButton mediaId="media-123" currentTime={2} onSkip={vi.fn()} />,
    );
    expect(container.firstChild).toBeNull();
  });

  it("returns null when MKV chapter intro is shorter than 5 seconds", () => {
    mockUseMediaSegments.mockReturnValue({ data: [] });
    mockUseQuery.mockReturnValue({
      data: { chapters: [{ title: "Intro", startMs: "1000", endMs: "4000" }] },
    });
    const { container } = render(
      <SkipIntroButton filePath="/media/ep1.mkv" currentTime={2} onSkip={vi.fn()} />,
    );
    expect(container.firstChild).toBeNull();
  });
});
