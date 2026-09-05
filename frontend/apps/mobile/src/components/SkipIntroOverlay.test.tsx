import { describe, it, expect, vi, beforeEach } from "vitest";
import { render } from "@testing-library/react";
import React from "react";

const announce = vi.fn();

vi.mock("react-native", () => ({
  AccessibilityInfo: { announceForAccessibility: (m: string) => announce(m) },
  Pressable: ({ children, onPress, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel }, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  StyleSheet: { create: (s: any) => s },
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { border: "#333", overlay: "#000a", text: "#fff" } }),
}));

vi.mock("@evey/store", () => ({ toast: { info: vi.fn() } }));
vi.mock("@evey/api", () => ({ mKVServiceListChapters: vi.fn() }));
vi.mock("@evey/theme", () => ({ FONT_SIZE: { base: 16 } }));
vi.mock("@tanstack/react-query", () => ({ useQuery: () => ({ data: undefined }) }));

let segments: { segments: unknown[]; introAutoSkip: boolean } | undefined;
vi.mock("../hooks/useMediaSegments", () => ({
  useMediaSegments: () => ({ data: segments }),
}));

import { SkipIntroOverlay } from "./SkipIntroOverlay";

const seg = (type: string, startMs: number, endMs: number) => ({
  type,
  startMs,
  endMs,
  confidence: 1,
  source: "chapter",
});

beforeEach(() => {
  announce.mockClear();
  segments = { segments: [seg("intro", 10_000, 60_000)], introAutoSkip: false };
});

describe("SkipIntroOverlay accessibility", () => {
  // The button shows up on its own a few seconds in and vanishes on its own at
  // the end of the segment. Sighted viewers see it arrive; nobody else did, and
  // an offer that is never announced is an offer that does not exist.
  it("announces the offer when the button appears", () => {
    render(<SkipIntroOverlay mediaId="m1" currentTime={20} onSkip={() => {}} />);
    expect(announce).toHaveBeenCalledWith("player.skipIntro");
  });

  it("says nothing while the playhead is outside the segment", () => {
    render(<SkipIntroOverlay mediaId="m1" currentTime={5} onSkip={() => {}} />);
    expect(announce).not.toHaveBeenCalled();
  });

  // The player ticks about twice a second; re-announcing on each tick would
  // talk over the film for the whole segment.
  it("announces once per segment, not on every tick", () => {
    const { rerender } = render(<SkipIntroOverlay mediaId="m1" currentTime={20} onSkip={() => {}} />);
    rerender(<SkipIntroOverlay mediaId="m1" currentTime={21} onSkip={() => {}} />);
    rerender(<SkipIntroOverlay mediaId="m1" currentTime={22} onSkip={() => {}} />);
    expect(announce).toHaveBeenCalledTimes(1);
  });

  // Same button, different offer: the label follows the segment type, so the
  // announcement has to follow it too.
  it("announces the credits offer with its own wording", () => {
    segments = { segments: [seg("end_credits", 10_000, 60_000)], introAutoSkip: false };
    render(<SkipIntroOverlay mediaId="m1" currentTime={20} onSkip={() => {}} />);
    expect(announce).toHaveBeenCalledWith("player.skipCredits");
  });
});
