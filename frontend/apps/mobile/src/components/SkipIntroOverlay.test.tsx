import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, fireEvent, screen } from "@testing-library/react";
import React from "react";

const announce = vi.fn();

vi.mock("react-native", () => ({
  AccessibilityInfo: { announceForAccessibility: (m: string) => announce(m) },
  Pressable: ({ children, onPress, accessibilityLabel, style }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, "data-style": JSON.stringify(style) }, children),
  View: ({ children, style }: any) => React.createElement("div", { "data-style": JSON.stringify(style) }, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  StyleSheet: { create: (s: any) => s },
}));

vi.mock("react-i18next", () => ({
  // The mock returns the raw key so existing assertions can match on it
  // directly; when interpolation params are given (e.g. the countdown's
  // { seconds }), they are appended so a test can still tell 15s from 10s.
  useTranslation: () => ({ t: (key: string, opts?: Record<string, unknown>) => (opts ? `${key}:${opts.seconds}` : key) }),
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

describe("SkipIntroOverlay next-episode pairing", () => {
  beforeEach(() => {
    segments = { segments: [seg("end_credits", 10_000, 60_000)], introAutoSkip: false };
  });

  it("renders both the skip and next-episode buttons during credits, each firing its own callback", () => {
    const onSkip = vi.fn();
    const onNextEpisode = vi.fn();
    render(<SkipIntroOverlay mediaId="m1" currentTime={20} onSkip={onSkip} onNextEpisode={onNextEpisode} />);

    fireEvent.click(screen.getByLabelText("player.skipCredits"));
    expect(onSkip).toHaveBeenCalledWith(60);

    fireEvent.click(screen.getByLabelText("player.nextEpisodeOverlay.nextEpisode"));
    expect(onNextEpisode).toHaveBeenCalledTimes(1);
  });

  it("renders only the skip button when onNextEpisode is not provided", () => {
    render(<SkipIntroOverlay mediaId="m1" currentTime={20} onSkip={() => {}} />);

    expect(screen.getByLabelText("player.skipCredits")).toBeTruthy();
    expect(screen.queryByLabelText("player.nextEpisodeOverlay.nextEpisode")).toBeNull();
  });

  it("renders nothing while hidden", () => {
    const { container } = render(
      <SkipIntroOverlay mediaId="m1" currentTime={20} onSkip={() => {}} onNextEpisode={() => {}} hidden />,
    );
    expect(container.firstChild).toBeNull();
  });

  it("anchors the row at bottomOffset so it clears the caller's control bar", () => {
    segments = { segments: [seg("intro", 10_000, 60_000)], introAutoSkip: false };
    render(<SkipIntroOverlay mediaId="m1" currentTime={20} onSkip={() => {}} bottomOffset={140} />);

    const row = screen.getByLabelText("player.skipIntro").parentElement;
    expect(row?.getAttribute("data-style")).toContain('"bottom":140');
  });
});

describe("SkipIntroOverlay credits countdown with auto-play on", () => {
  beforeEach(() => {
    // startSec=10, endSec=60 (ms -> sec)
    segments = { segments: [seg("end_credits", 10_000, 60_000)], introAutoSkip: false };
  });

  it("counts down on the playhead from the start of the credits", () => {
    const { rerender } = render(
      <SkipIntroOverlay mediaId="m1" currentTime={10} onSkip={() => {}} onNextEpisode={() => {}} autoAdvance />,
    );
    expect(screen.getByLabelText("player.watchCredits")).toBeTruthy();
    // remaining = ceil(min(10+15,60) - 10) = 15
    expect(screen.getByLabelText("player.nextEpisodeOverlay.nextEpisode").textContent).toBe(
      "player.nextEpisodeOverlay.nextIn:15",
    );

    rerender(<SkipIntroOverlay mediaId="m1" currentTime={15.2} onSkip={() => {}} onNextEpisode={() => {}} autoAdvance />);
    // remaining = ceil(25 - 15.2) = ceil(9.8) = 10
    expect(screen.getByLabelText("player.nextEpisodeOverlay.nextEpisode").textContent).toBe(
      "player.nextEpisodeOverlay.nextIn:10",
    );
  });

  it("fires onNextEpisode exactly once when the countdown reaches zero, and not again on later re-renders", () => {
    const onNextEpisode = vi.fn();
    const { rerender } = render(
      <SkipIntroOverlay mediaId="m1" currentTime={24} onSkip={() => {}} onNextEpisode={onNextEpisode} autoAdvance />,
    );
    expect(onNextEpisode).not.toHaveBeenCalled();
    rerender(<SkipIntroOverlay mediaId="m1" currentTime={25} onSkip={() => {}} onNextEpisode={onNextEpisode} autoAdvance />);
    rerender(<SkipIntroOverlay mediaId="m1" currentTime={26} onSkip={() => {}} onNextEpisode={onNextEpisode} autoAdvance />);
    expect(onNextEpisode).toHaveBeenCalledTimes(1);
  });

  it("Watch credits cancels the countdown for these credits and restores Skip Credits, no later auto-fire", () => {
    const onNextEpisode = vi.fn();
    const { rerender } = render(
      <SkipIntroOverlay mediaId="m1" currentTime={10} onSkip={() => {}} onNextEpisode={onNextEpisode} autoAdvance />,
    );
    fireEvent.click(screen.getByLabelText("player.watchCredits"));
    expect(screen.getByLabelText("player.skipCredits")).toBeTruthy();
    expect(screen.getByLabelText("player.nextEpisodeOverlay.nextEpisode").textContent).toBe(
      "player.nextEpisodeOverlay.nextEpisode",
    );
    rerender(<SkipIntroOverlay mediaId="m1" currentTime={50} onSkip={() => {}} onNextEpisode={onNextEpisode} autoAdvance />);
    expect(onNextEpisode).not.toHaveBeenCalled();
  });

  it("never counts down or fires with auto-play off", () => {
    const onNextEpisode = vi.fn();
    const { rerender } = render(
      <SkipIntroOverlay mediaId="m1" currentTime={10} onSkip={() => {}} onNextEpisode={onNextEpisode} />,
    );
    rerender(<SkipIntroOverlay mediaId="m1" currentTime={26} onSkip={() => {}} onNextEpisode={onNextEpisode} />);
    expect(screen.getByLabelText("player.skipCredits")).toBeTruthy();
    expect(screen.getByLabelText("player.nextEpisodeOverlay.nextEpisode").textContent).toBe(
      "player.nextEpisodeOverlay.nextEpisode",
    );
    expect(onNextEpisode).not.toHaveBeenCalled();
  });
});
