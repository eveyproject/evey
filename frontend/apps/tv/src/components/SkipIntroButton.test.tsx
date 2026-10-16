import { describe, it, expect, vi, beforeEach } from "vitest";
import { act, render, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Pressable: ({ children, onPress, ...p }: any) =>
    React.createElement("button", { onClick: onPress, ...p }, children),
  StyleSheet: { create: (s: any) => s },
}));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { border: "#333", overlay: "rgba(0,0,0,0.5)", text: "#fff", accent: "#f00" },
  }),
}));

// The real catalogue interpolates {{seconds}} into English text; this mock
// only needs to make the countdown value observable, so it appends it
// rather than reproducing i18next's templating.
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, opts?: { seconds?: number }) =>
      opts && typeof opts.seconds === "number" ? `${key}:${opts.seconds}` : key,
  }),
}));

const useMediaSegmentsMock = vi.fn();
vi.mock("../hooks/useMediaSegments", () => ({
  useMediaSegments: (...args: unknown[]) => useMediaSegmentsMock(...args),
}));

// The MKV-chapter fallback (shared hook): what the file's chapters name, set
// per test; nothing by default.
const chapterSkippablesMock = vi.fn((_filePath?: string, _needed?: boolean) => [] as { type: string; startSec: number; endSec: number }[]);
vi.mock("@evey/client", async (orig) => ({
  ...(await orig<typeof import("@evey/client")>()),
  useChapterSkippables: (filePath?: string, needed?: boolean) => chapterSkippablesMock(filePath, needed),
}));

const toastInfoMock = vi.fn();
vi.mock("@evey/store", () => ({
  toast: { info: (...args: unknown[]) => toastInfoMock(...args) },
}));

import { SkipIntroButton } from "./SkipIntroButton";

beforeEach(() => {
  useMediaSegmentsMock.mockReset();
  toastInfoMock.mockReset();
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

  // The television had no chapter fallback: a file whose own chapters name its
  // opening got no button while the server had not detected segments yet.
  it("offers to skip the opening a file's chapters name while the server has no segment", () => {
    useMediaSegmentsMock.mockReturnValue({ data: { introAutoSkip: false, segments: [] } });
    chapterSkippablesMock.mockReturnValueOnce([{ type: "intro", startSec: 0, endSec: 90 }]);
    const onSkip = vi.fn();
    const { getByText } = render(<SkipIntroButton mediaId="mi-1" filePath="/media/ep1.mkv" currentTime={12} onSkip={onSkip} />);
    expect(chapterSkippablesMock).toHaveBeenCalledWith("/media/ep1.mkv", true);
    fireEvent.click(getByText("player.skipIntro"));
    expect(onSkip).toHaveBeenCalledWith(90);
  });

  // B5: the player screen must know whether the button ITSELF holds TV focus
  // (not merely that it is visible) before it can safely swallow "select".
  it("reports its own focus state to the caller, distinctly from visibility", () => {
    useMediaSegmentsMock.mockReturnValue({
      data: { introAutoSkip: false, segments: [{ type: "intro", startMs: 0, endMs: 30000, confidence: 0.9, source: "audio" }] },
    });
    const onFocusChange = vi.fn();
    const { getByText } = render(
      <SkipIntroButton mediaId="mi-1" currentTime={10} onSkip={vi.fn()} onFocusChange={onFocusChange} />,
    );
    // focus/blur do not bubble - the handler lives on the button itself, not
    // the text node getByText resolves to.
    const button = getByText("player.skipIntro").closest("button")!;
    fireEvent.focus(button);
    expect(onFocusChange).toHaveBeenCalledWith(true);
    fireEvent.blur(button);
    expect(onFocusChange).toHaveBeenCalledWith(false);
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

  // M3: a recap and an intro can overlap. The server used to decide the
  // winner by array order (recap listed first shadowed an intro that had
  // actually started sooner); the earliest-starting segment must win instead.
  it("prefers the earlier-starting segment when two overlap, regardless of server order", () => {
    useMediaSegmentsMock.mockReturnValue({
      data: { introAutoSkip: false, segments: [
        { type: "recap", startMs: 20000, endMs: 50000, confidence: 0.8, source: "audio" },
        { type: "intro", startMs: 0, endMs: 30000, confidence: 0.9, source: "audio" },
      ] },
    });
    const { getByText } = render(<SkipIntroButton mediaId="mi-1" currentTime={25} onSkip={vi.fn()} />);
    expect(getByText("player.skipIntro")).toBeTruthy();
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

    // Silent before this fix (M1): the jump happened with no on-screen notice,
    // unlike web and mobile which have toasted it since the toggle shipped.
    it("toasts that the intro was skipped", () => {
      useMediaSegmentsMock.mockReturnValue({
        data: { introAutoSkip: true, segments: [intro] },
      });
      render(<SkipIntroButton mediaId="mi-1" currentTime={10} onSkip={vi.fn()} />);
      expect(toastInfoMock).toHaveBeenCalledWith("player.introSkipped");
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

  // Item 1: "Next episode" appears next to "Skip credits" during the end
  // credits, so the viewer does not have to dismiss the credits button first.
  describe("next episode button (end credits only)", () => {
    const endCredits = { type: "end_credits", startMs: 60000, endMs: 90000, confidence: 0.9, source: "audio" };

    it("renders both buttons during the end credits when onNextEpisode is provided", () => {
      useMediaSegmentsMock.mockReturnValue({ data: { introAutoSkip: false, segments: [endCredits] } });
      const onSkip = vi.fn();
      const onNextEpisode = vi.fn();
      const { getByText } = render(
        <SkipIntroButton mediaId="mi-1" currentTime={70} onSkip={onSkip} onNextEpisode={onNextEpisode} />,
      );

      fireEvent.click(getByText("player.skipCredits"));
      expect(onSkip).toHaveBeenCalledWith(90);

      fireEvent.click(getByText("player.nextEpisodeOverlay.nextEpisode"));
      expect(onNextEpisode).toHaveBeenCalledTimes(1);
    });

    it("renders a single button during the end credits when onNextEpisode is not provided", () => {
      useMediaSegmentsMock.mockReturnValue({ data: { introAutoSkip: false, segments: [endCredits] } });
      const { queryByText } = render(<SkipIntroButton mediaId="mi-1" currentTime={70} onSkip={vi.fn()} />);

      expect(queryByText("player.skipCredits")).toBeTruthy();
      expect(queryByText("player.nextEpisodeOverlay.nextEpisode")).toBeNull();
    });

    it("never shows the next-episode button for an intro segment, even with onNextEpisode provided", () => {
      useMediaSegmentsMock.mockReturnValue({
        data: { introAutoSkip: false, segments: [{ type: "intro", startMs: 0, endMs: 30000, confidence: 0.9, source: "audio" }] },
      });
      const { queryByText } = render(
        <SkipIntroButton mediaId="mi-1" currentTime={10} onSkip={vi.fn()} onNextEpisode={vi.fn()} />,
      );

      expect(queryByText("player.skipIntro")).toBeTruthy();
      expect(queryByText("player.nextEpisodeOverlay.nextEpisode")).toBeNull();
    });
  });

  // With auto-play on, the credits button becomes a countdown that fires on
  // its own - unless the viewer asks to watch the credits.
  describe("countdown with auto-play on", () => {
    const endCredits = { type: "end_credits", startMs: 3_400_000, endMs: 3_600_000, confidence: 0.9, source: "audio" };

    // Playback as the player reports it, a tick every half second: a single
    // rerender far ahead reads as a seek, which cancels (its own test below).
    function mountCredits(at: number, props: { autoAdvance?: boolean; cancelCountdownRef?: React.MutableRefObject<(() => void) | null> } = {}) {
      useMediaSegmentsMock.mockReturnValue({ data: { introAutoSkip: false, segments: [endCredits] } });
      const onNextEpisode = vi.fn();
      const autoAdvance = props.autoAdvance ?? true;
      const el = (t: number) => (
        <SkipIntroButton mediaId="m1" currentTime={t} onSkip={vi.fn()} onNextEpisode={onNextEpisode} autoAdvance={autoAdvance} cancelCountdownRef={props.cancelCountdownRef} />
      );
      const view = render(el(at));
      const playTo = (to: number, from: number) => {
        for (let t = from + 0.5; t < to; t += 0.5) view.rerender(el(t));
        view.rerender(el(to));
      };
      const seekTo = (to: number) => view.rerender(el(to));
      return { ...view, onNextEpisode, playTo, seekTo };
    }

    it("counts down on the playhead from the start of the credits", () => {
      const { getByText, playTo } = mountCredits(3_400);
      expect(getByText("player.watchCredits")).toBeTruthy();
      expect(getByText("player.nextEpisodeOverlay.nextIn:15")).toBeTruthy();

      playTo(3_405.2, 3_400);
      expect(getByText("player.nextEpisodeOverlay.nextIn:10")).toBeTruthy();
    });

    it("fires once when the countdown reaches zero, and not on a later tick", () => {
      const { playTo, onNextEpisode } = mountCredits(3_400);
      playTo(3_414, 3_400);
      expect(onNextEpisode).not.toHaveBeenCalled();

      playTo(3_416, 3_414);
      expect(onNextEpisode).toHaveBeenCalledTimes(1);
    });

    // Prod 2026-10-15: credits marked four minutes early, a resume inside them
    // inherited six seconds of the clock, then the next episode, every time.
    it("gives a viewer who resumes inside the credits the full countdown", () => {
      const { getByText, playTo, onNextEpisode } = mountCredits(3_410);
      expect(getByText("player.nextEpisodeOverlay.nextIn:15")).toBeTruthy();
      playTo(3_416, 3_410);
      expect(getByText("player.nextEpisodeOverlay.nextIn:9")).toBeTruthy();
      expect(onNextEpisode).not.toHaveBeenCalled();
    });

    it("Keep watching cancels the countdown for these credits and restores Skip credits", () => {
      const { getByText, playTo, onNextEpisode } = mountCredits(3_400);
      fireEvent.click(getByText("player.watchCredits"));
      expect(getByText("player.skipCredits")).toBeTruthy();
      expect(getByText("player.nextEpisodeOverlay.nextEpisode")).toBeTruthy();

      playTo(3_500, 3_400);
      expect(onNextEpisode).not.toHaveBeenCalled();
    });

    // "Nothing lets me cancel it" (owner, 2026-10-15): a seek inside the
    // credits is the viewer still watching, it cancels like the button does.
    it("a seek inside the credits cancels the countdown", () => {
      const { getByText, playTo, seekTo, onNextEpisode } = mountCredits(3_400);
      playTo(3_402, 3_400);
      seekTo(3_430);
      expect(getByText("player.skipCredits")).toBeTruthy();
      playTo(3_500, 3_430);
      expect(onNextEpisode).not.toHaveBeenCalled();
    });

    it("arms the BACK cancel while counting and clears it once the viewer keeps watching", () => {
      const cancelCountdownRef = { current: null as (() => void) | null };
      const { getByText, playTo, onNextEpisode } = mountCredits(3_400, { cancelCountdownRef });
      expect(cancelCountdownRef.current).not.toBeNull();
      act(() => cancelCountdownRef.current?.());
      expect(cancelCountdownRef.current).toBeNull();
      expect(getByText("player.skipCredits")).toBeTruthy();
      playTo(3_500, 3_400);
      expect(onNextEpisode).not.toHaveBeenCalled();
    });

    it("never counts down with auto-play off", () => {
      const cancelCountdownRef = { current: null as (() => void) | null };
      const { getByText, playTo, onNextEpisode } = mountCredits(3_400, { autoAdvance: false, cancelCountdownRef });
      expect(getByText("player.skipCredits")).toBeTruthy();
      expect(getByText("player.nextEpisodeOverlay.nextEpisode")).toBeTruthy();

      playTo(3_415, 3_400);
      expect(cancelCountdownRef.current).toBeNull();
      expect(onNextEpisode).not.toHaveBeenCalled();
    });
  });
});
