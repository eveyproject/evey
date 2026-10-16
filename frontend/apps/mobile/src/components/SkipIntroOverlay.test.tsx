import { describe, it, expect, vi, beforeEach } from "vitest";
import { act, render, fireEvent, screen } from "@testing-library/react";
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

  // Playback as the player reports it, a tick every half second: a single
  // rerender far ahead reads as a seek, which cancels (its own test below).
  function mountCredits(at: number, props: { autoAdvance?: boolean; cancelCountdownRef?: React.MutableRefObject<(() => void) | null> } = {}) {
    const onNextEpisode = vi.fn();
    const autoAdvance = props.autoAdvance ?? true;
    const el = (t: number) => (
      <SkipIntroOverlay mediaId="m1" currentTime={t} onSkip={() => {}} onNextEpisode={onNextEpisode} autoAdvance={autoAdvance} cancelCountdownRef={props.cancelCountdownRef} />
    );
    const view = render(el(at));
    const playTo = (to: number, from: number) => {
      for (let t = from + 0.5; t < to; t += 0.5) view.rerender(el(t));
      view.rerender(el(to));
    };
    const seekTo = (to: number) => view.rerender(el(to));
    return { ...view, onNextEpisode, playTo, seekTo };
  }
  const nextText = () => screen.getByLabelText("player.nextEpisodeOverlay.nextEpisode").textContent;

  it("counts down on the playhead from the start of the credits", () => {
    const { playTo } = mountCredits(10);
    expect(screen.getByLabelText("player.watchCredits")).toBeTruthy();
    // remaining = ceil(min(10+15,60) - 10) = 15
    expect(nextText()).toBe("player.nextEpisodeOverlay.nextIn:15");

    playTo(15.2, 10);
    // remaining = ceil(25 - 15.2) = ceil(9.8) = 10
    expect(nextText()).toBe("player.nextEpisodeOverlay.nextIn:10");
  });

  it("fires onNextEpisode exactly once when the countdown reaches zero, and not again on later ticks", () => {
    const { playTo, onNextEpisode } = mountCredits(10);
    playTo(24, 10);
    expect(onNextEpisode).not.toHaveBeenCalled();
    playTo(26, 24);
    expect(onNextEpisode).toHaveBeenCalledTimes(1);
  });

  // Prod 2026-10-15: credits marked four minutes early, a resume inside them
  // inherited six seconds of the clock, then the next episode, every time.
  it("gives a viewer who resumes inside the credits the full countdown", () => {
    const { playTo, onNextEpisode } = mountCredits(24);
    expect(nextText()).toBe("player.nextEpisodeOverlay.nextIn:15");
    playTo(30, 24);
    expect(nextText()).toBe("player.nextEpisodeOverlay.nextIn:9");
    expect(onNextEpisode).not.toHaveBeenCalled();
  });

  it("Keep watching cancels the countdown for these credits and restores Skip Credits, no later auto-fire", () => {
    const { playTo, onNextEpisode } = mountCredits(10);
    fireEvent.click(screen.getByLabelText("player.watchCredits"));
    expect(screen.getByLabelText("player.skipCredits")).toBeTruthy();
    expect(nextText()).toBe("player.nextEpisodeOverlay.nextEpisode");
    playTo(50, 10);
    expect(onNextEpisode).not.toHaveBeenCalled();
  });

  // "Nothing lets me cancel it" (owner, 2026-10-15): a scrub inside the
  // credits is the viewer still watching, it cancels like the button does.
  it("a seek inside the credits cancels the countdown", () => {
    const { playTo, seekTo, onNextEpisode } = mountCredits(10);
    playTo(12, 10);
    seekTo(40);
    expect(screen.getByLabelText("player.skipCredits")).toBeTruthy();
    playTo(59, 40);
    expect(onNextEpisode).not.toHaveBeenCalled();
  });

  it("arms the hardware Back cancel while counting and clears it once the viewer keeps watching", () => {
    const cancelCountdownRef = { current: null as (() => void) | null };
    const { playTo, onNextEpisode } = mountCredits(10, { cancelCountdownRef });
    expect(cancelCountdownRef.current).not.toBeNull();
    act(() => cancelCountdownRef.current?.());
    expect(cancelCountdownRef.current).toBeNull();
    expect(screen.getByLabelText("player.skipCredits")).toBeTruthy();
    playTo(50, 10);
    expect(onNextEpisode).not.toHaveBeenCalled();
  });

  it("never counts down or fires with auto-play off", () => {
    const cancelCountdownRef = { current: null as (() => void) | null };
    const { playTo, onNextEpisode } = mountCredits(10, { autoAdvance: false, cancelCountdownRef });
    playTo(26, 10);
    expect(screen.getByLabelText("player.skipCredits")).toBeTruthy();
    expect(nextText()).toBe("player.nextEpisodeOverlay.nextEpisode");
    expect(cancelCountdownRef.current).toBeNull();
    expect(onNextEpisode).not.toHaveBeenCalled();
  });
});
