import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, ...p }: any) =>
    React.createElement("button", { onClick: onPress, ...p }, children),
  StyleSheet: { create: (s: any) => s },
}));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { border: "#333", overlay: "rgba(0,0,0,0.5)", text: "#fff", accent: "#f00" },
  }),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

const useMediaSegmentsMock = vi.fn();
vi.mock("../hooks/useMediaSegments", () => ({
  useMediaSegments: (...args: unknown[]) => useMediaSegmentsMock(...args),
}));

import { SkipIntroButton } from "./SkipIntroButton";

beforeEach(() => {
  useMediaSegmentsMock.mockReset();
});

describe("SkipIntroButton", () => {
  it("renders nothing when there is no active segment", () => {
    useMediaSegmentsMock.mockReturnValue({ data: { segments: [], introAutoSkip: false } });
    const { container } = render(<SkipIntroButton mediaId="mi-1" currentTime={0} onSkip={vi.fn()} />);
    expect(container.innerHTML).toBe("");
  });

  it("renders nothing when currentTime is outside every segment", () => {
    useMediaSegmentsMock.mockReturnValue({
      data: { introAutoSkip: false, segments: [{ type: "intro", startMs: 0, endMs: 30000, confidence: 0.9, source: "audio" }] },
    });
    const { container } = render(<SkipIntroButton mediaId="mi-1" currentTime={40} onSkip={vi.fn()} />);
    expect(container.innerHTML).toBe("");
  });

  it("ignores segments shorter than the minimum skippable duration", () => {
    useMediaSegmentsMock.mockReturnValue({
      data: { introAutoSkip: false, segments: [{ type: "intro", startMs: 0, endMs: 2000, confidence: 0.9, source: "audio" }] },
    });
    const { container } = render(<SkipIntroButton mediaId="mi-1" currentTime={1} onSkip={vi.fn()} />);
    expect(container.innerHTML).toBe("");
  });

  it("shows the intro skip button and skips to the segment end on press", () => {
    useMediaSegmentsMock.mockReturnValue({
      data: { introAutoSkip: false, segments: [{ type: "intro", startMs: 0, endMs: 30000, confidence: 0.9, source: "audio" }] },
    });
    const onSkip = vi.fn();
    const { getByText } = render(<SkipIntroButton mediaId="mi-1" currentTime={10} onSkip={onSkip} />);

    fireEvent.click(getByText("player.skipIntro"));
    expect(onSkip).toHaveBeenCalledWith(30);
  });

  it("labels a recap segment distinctly from an intro", () => {
    useMediaSegmentsMock.mockReturnValue({
      data: { introAutoSkip: false, segments: [{ type: "recap", startMs: 0, endMs: 20000, confidence: 0.8, source: "audio" }] },
    });
    const { getByText } = render(<SkipIntroButton mediaId="mi-1" currentTime={5} onSkip={vi.fn()} />);
    expect(getByText("player.skipRecap")).toBeTruthy();
  });

  it("picks the segment matching the current playhead among several", () => {
    useMediaSegmentsMock.mockReturnValue({
      data: { introAutoSkip: false, segments: [
        { type: "intro", startMs: 0, endMs: 30000, confidence: 0.9, source: "audio" },
        { type: "end_credits", startMs: 60000, endMs: 90000, confidence: 0.9, source: "audio" },
      ] },
    });
    const onSkip = vi.fn();
    const { getByText } = render(<SkipIntroButton mediaId="mi-1" currentTime={70} onSkip={onSkip} />);

    fireEvent.click(getByText("player.skipCredits"));
    expect(onSkip).toHaveBeenCalledWith(90);
  });
  // The admin "auto-skip the intro" toggle. Web and mobile have honoured it
  // since it shipped; the TV player never read it, so turning it on did
  // nothing on this one player even though the setting claimed to cover them
  // all. Found 2026-08-08 by comparing what each client parses.
  describe("auto-skip (admin toggle)", () => {
    const intro = { type: "intro", startMs: 0, endMs: 30000, confidence: 0.9, source: "audio" };

    it("jumps the intro without waiting for OK when the toggle is on", () => {
      useMediaSegmentsMock.mockReturnValue({
        data: { introAutoSkip: true, segments: [intro] },
      });
      const onSkip = vi.fn();
      const { container } = render(<SkipIntroButton mediaId="mi-1" currentTime={10} onSkip={onSkip} />);

      expect(onSkip).toHaveBeenCalledWith(30);
      // The button must not also be on screen: it holds TV focus, and OK on a
      // control that is about to disappear is the double-fire this screen has
      // already been bitten by.
      expect(container.innerHTML).toBe("");
    });

    it("leaves the manual button alone when the toggle is off", () => {
      useMediaSegmentsMock.mockReturnValue({
        data: { introAutoSkip: false, segments: [intro] },
      });
      const onSkip = vi.fn();
      const { getByText } = render(<SkipIntroButton mediaId="mi-1" currentTime={10} onSkip={onSkip} />);

      expect(onSkip).not.toHaveBeenCalled();
      expect(getByText("player.skipIntro")).toBeTruthy();
    });

    it("never auto-skips a recap or the credits, only the intro", () => {
      useMediaSegmentsMock.mockReturnValue({
        data: {
          introAutoSkip: true,
          segments: [{ type: "recap", startMs: 0, endMs: 20000, confidence: 0.8, source: "audio" }],
        },
      });
      const onSkip = vi.fn();
      const { getByText } = render(<SkipIntroButton mediaId="mi-1" currentTime={5} onSkip={onSkip} />);

      expect(onSkip).not.toHaveBeenCalled();
      expect(getByText("player.skipRecap")).toBeTruthy();
    });

    it("skips a given intro once - seeking back into it offers the button instead", () => {
      useMediaSegmentsMock.mockReturnValue({
        data: { introAutoSkip: true, segments: [intro] },
      });
      const onSkip = vi.fn();
      const { rerender, getByText } = render(
        <SkipIntroButton mediaId="mi-1" currentTime={10} onSkip={onSkip} />,
      );
      expect(onSkip).toHaveBeenCalledTimes(1);

      // The viewer seeks back into the same intro: that is a choice to watch
      // it, so it must not be jumped again.
      rerender(<SkipIntroButton mediaId="mi-1" currentTime={5} onSkip={onSkip} />);
      expect(onSkip).toHaveBeenCalledTimes(1);
      expect(getByText("player.skipIntro")).toBeTruthy();
    });
  });
});
