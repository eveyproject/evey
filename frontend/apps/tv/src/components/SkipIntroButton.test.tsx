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
    useMediaSegmentsMock.mockReturnValue({ data: [] });
    const { container } = render(<SkipIntroButton mediaId="mi-1" currentTime={0} onSkip={vi.fn()} />);
    expect(container.innerHTML).toBe("");
  });

  it("renders nothing when currentTime is outside every segment", () => {
    useMediaSegmentsMock.mockReturnValue({
      data: [{ type: "intro", startMs: 0, endMs: 30000, confidence: 0.9, source: "audio" }],
    });
    const { container } = render(<SkipIntroButton mediaId="mi-1" currentTime={40} onSkip={vi.fn()} />);
    expect(container.innerHTML).toBe("");
  });

  it("ignores segments shorter than the minimum skippable duration", () => {
    useMediaSegmentsMock.mockReturnValue({
      data: [{ type: "intro", startMs: 0, endMs: 2000, confidence: 0.9, source: "audio" }],
    });
    const { container } = render(<SkipIntroButton mediaId="mi-1" currentTime={1} onSkip={vi.fn()} />);
    expect(container.innerHTML).toBe("");
  });

  it("shows the intro skip button and skips to the segment end on press", () => {
    useMediaSegmentsMock.mockReturnValue({
      data: [{ type: "intro", startMs: 0, endMs: 30000, confidence: 0.9, source: "audio" }],
    });
    const onSkip = vi.fn();
    const { getByText } = render(<SkipIntroButton mediaId="mi-1" currentTime={10} onSkip={onSkip} />);

    fireEvent.click(getByText("player.skipIntro"));
    expect(onSkip).toHaveBeenCalledWith(30);
  });

  it("labels a recap segment distinctly from an intro", () => {
    useMediaSegmentsMock.mockReturnValue({
      data: [{ type: "recap", startMs: 0, endMs: 20000, confidence: 0.8, source: "audio" }],
    });
    const { getByText } = render(<SkipIntroButton mediaId="mi-1" currentTime={5} onSkip={vi.fn()} />);
    expect(getByText("player.skipRecap")).toBeTruthy();
  });

  it("picks the segment matching the current playhead among several", () => {
    useMediaSegmentsMock.mockReturnValue({
      data: [
        { type: "intro", startMs: 0, endMs: 30000, confidence: 0.9, source: "audio" },
        { type: "end_credits", startMs: 60000, endMs: 90000, confidence: 0.9, source: "audio" },
      ],
    });
    const onSkip = vi.fn();
    const { getByText } = render(<SkipIntroButton mediaId="mi-1" currentTime={70} onSkip={onSkip} />);

    fireEvent.click(getByText("player.skipCredits"));
    expect(onSkip).toHaveBeenCalledWith(90);
  });
});
