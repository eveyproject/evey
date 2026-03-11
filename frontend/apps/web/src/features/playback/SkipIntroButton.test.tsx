import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { useQuery } from "@tanstack/react-query";
import { SkipIntroButton } from "./SkipIntroButton";

vi.mock("@evey/api", () => ({
  mKVServiceListChapters: vi.fn(),
}));

vi.mock("@tanstack/react-query", () => ({
  useQuery: vi.fn(),
}));

const mockUseQuery = useQuery as ReturnType<typeof vi.fn>;

const introChapter = {
  title: "Intro",
  startMs: "5000",
  endMs: "90000",
};

describe("SkipIntroButton", () => {
  beforeEach(() => {
    mockUseQuery.mockReset();
  });

  it("renders Skip Intro button when in intro chapter", () => {
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

  it("returns null when no intro chapter", () => {
    mockUseQuery.mockReturnValue({
      data: { chapters: [{ title: "Main", startMs: "0", endMs: "90000" }] },
    });
    const { container } = render(
      <SkipIntroButton filePath="/media/ep1.mkv" currentTime={30} onSkip={vi.fn()} />,
    );
    expect(container.firstChild).toBeNull();
  });

  it("returns null when currentTime is before intro start", () => {
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

  it("returns null when currentTime is at or past intro end", () => {
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

  it("calls onSkip with endSec on click", async () => {
    const user = userEvent.setup();
    const onSkip = vi.fn();
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
});
