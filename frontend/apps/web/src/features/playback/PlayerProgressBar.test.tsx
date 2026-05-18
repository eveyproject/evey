import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, it, expect, vi } from "vitest";
import { PlayerProgressBar } from "./PlayerProgressBar";

vi.mock("./TrickplayPreview", () => ({
  TrickplayPreview: () => null,
}));

const defaultProps = {
  absoluteTime: 30,
  effectiveDuration: 120,
  bufferedEnd: 60,
  transcodeOffset: 0,
  chapters: [],
  onSeek: vi.fn(),
  onChapterSeek: vi.fn(),
};

describe("PlayerProgressBar", () => {
  it("renders seek slider", () => {
    render(<PlayerProgressBar {...defaultProps} />);
    expect(screen.getByRole("slider", { name: "Seek" })).toBeInTheDocument();
  });

  it("sets slider max to effective duration", () => {
    render(<PlayerProgressBar {...defaultProps} />);
    const slider = screen.getByRole("slider", { name: "Seek" }) as HTMLInputElement;
    expect(slider.max).toBe("120");
  });

  it("sets slider value to absolute time", () => {
    render(<PlayerProgressBar {...defaultProps} absoluteTime={45} />);
    const slider = screen.getByRole("slider", { name: "Seek" }) as HTMLInputElement;
    expect(slider.value).toBe("45");
  });
});

describe("PlayerProgressBar seek commit", () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  // Anti-regression: a quick single change must commit the seek after the
  // 200ms debounce. The previous implementation relied on a stale ref +
  // onMouseUp that never fired for click-without-drag, so onSeek was lost.
  it("commits the seek 200ms after a single change", () => {
    const onSeek = vi.fn();
    render(<PlayerProgressBar {...defaultProps} onSeek={onSeek} />);
    const slider = screen.getByRole("slider", { name: "Seek" }) as HTMLInputElement;

    fireEvent.change(slider, { target: { value: "75" } });
    expect(onSeek).not.toHaveBeenCalled(); // not yet
    act(() => {
      vi.advanceTimersByTime(200);
    });
    expect(onSeek).toHaveBeenCalledTimes(1);
    expect(onSeek.mock.calls[0][0].target.value).toBe("75");
  });

  // Anti-regression: only the last change in a burst commits — protects the
  // transcode pipeline from a cascade of seek restarts during a drag.
  it("debounces a burst of changes to a single commit on the final value", () => {
    const onSeek = vi.fn();
    render(<PlayerProgressBar {...defaultProps} onSeek={onSeek} />);
    const slider = screen.getByRole("slider", { name: "Seek" }) as HTMLInputElement;

    fireEvent.change(slider, { target: { value: "10" } });
    fireEvent.change(slider, { target: { value: "20" } });
    fireEvent.change(slider, { target: { value: "30" } });
    act(() => {
      vi.advanceTimersByTime(200);
    });
    expect(onSeek).toHaveBeenCalledTimes(1);
    expect(onSeek.mock.calls[0][0].target.value).toBe("30");
  });
});
