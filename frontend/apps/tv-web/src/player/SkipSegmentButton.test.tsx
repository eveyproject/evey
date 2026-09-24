import type React from "react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";

const segments = vi.fn(() => ({ data: { segments: [] as unknown[], introAutoSkip: false } }));
const chapterSkippables = vi.fn((_path?: string, _needed?: boolean) => [] as { type: string; startSec: number; endSec: number }[]);
vi.mock("@evey/client", async (orig) => ({
  ...(await orig<typeof import("@evey/client")>()),
  useMediaSegments: () => segments(),
  useChapterSkippables: (path?: string, needed?: boolean) => chapterSkippables(path, needed),
}));
vi.mock("../components/ui", () => ({
  Pressable: ({ children, onPress }: { children?: React.ReactNode; onPress?: () => void }) => <button onClick={onPress}>{children}</button>,
}));
vi.mock("../tv/focus", () => ({
  FocusRoot: ({ children }: { children?: React.ReactNode }) => <div>{children}</div>,
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock("@evey/store", () => ({ toast: { info: vi.fn() } }));

import { SkipSegmentButton } from "./SkipSegmentButton";

beforeEach(() => { segments.mockClear(); chapterSkippables.mockClear(); });

describe("SkipSegmentButton (tv-web)", () => {
  // The browser television had no chapter fallback: a file whose own chapters
  // name its opening got no button while the server had not detected segments.
  it("offers to skip the opening a file's chapters name while the server has no segment", () => {
    chapterSkippables.mockReturnValue([{ type: "intro", startSec: 0, endSec: 90 }]);
    const onSkip = vi.fn();
    render(<SkipSegmentButton mediaId="mi-1" filePath="/media/ep1.mkv" currentTime={12} onSkip={onSkip} />);
    expect(chapterSkippables).toHaveBeenCalledWith("/media/ep1.mkv", true);
    fireEvent.click(screen.getByText("player.skipIntro"));
    expect(onSkip).toHaveBeenCalledWith(90);
  });

  it("does not ask the chapters when the server has segments", () => {
    segments.mockReturnValue({ data: { segments: [{ type: "intro", startMs: 0, endMs: 30000 }], introAutoSkip: false } });
    render(<SkipSegmentButton mediaId="mi-1" filePath="/media/ep1.mkv" currentTime={12} onSkip={vi.fn()} />);
    expect(chapterSkippables).toHaveBeenCalledWith("/media/ep1.mkv", false);
    expect(screen.getByText("player.skipIntro")).toBeTruthy();
  });
});
