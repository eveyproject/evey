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

  // A pointer scrub aims with the hover tooltip, which displays whole seconds:
  // the committed seek must land exactly on that displayed second, not on the
  // raw pixel value (one pixel spans several seconds on a long media).
  it("floors a pointer-driven seek to the second the tooltip displays", () => {
    const onSeek = vi.fn();
    const { container } = render(<PlayerProgressBar {...defaultProps} onSeek={onSeek} />);
    const bar = container.querySelector("[data-controls]") as HTMLElement;
    const slider = screen.getByRole("slider", { name: "Seek" }) as HTMLInputElement;

    fireEvent.mouseMove(bar, { clientX: 40 });
    fireEvent.change(slider, { target: { value: "75.5" } });
    act(() => { vi.advanceTimersByTime(200); });
    expect(onSeek).toHaveBeenCalledTimes(1);
    expect(onSeek.mock.calls[0][0].target.value).toBe("75");
  });

  // Keyboard seeks have no hover tooltip and step by 0.5s: they must keep the
  // fractional value (flooring would swallow every other arrow press).
  it("keeps the fractional value for a keyboard seek (no hover)", () => {
    const onSeek = vi.fn();
    render(<PlayerProgressBar {...defaultProps} onSeek={onSeek} />);
    const slider = screen.getByRole("slider", { name: "Seek" }) as HTMLInputElement;

    fireEvent.change(slider, { target: { value: "75.5" } });
    act(() => { vi.advanceTimersByTime(200); });
    expect(onSeek.mock.calls[0][0].target.value).toBe("75.5");
  });

  // Anti-regression: only the last change in a burst commits - protects the
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

  // Anti-regression for the "I scrub to 10min and it jumps back to 30min" bug:
  // a transcode seek restarts the stream, so absoluteTime keeps reporting the
  // OLD position for a couple of seconds. The thumb must hold the requested
  // position until the real playhead catches up, then track it again.
  it("holds the requested position after commit, then releases when it converges", () => {
    const onSeek = vi.fn();
    const { rerender } = render(
      <PlayerProgressBar {...defaultProps} absoluteTime={1800} effectiveDuration={3000} onSeek={onSeek} />,
    );
    const slider = screen.getByRole("slider", { name: "Seek" }) as HTMLInputElement;

    fireEvent.change(slider, { target: { value: "600" } });
    act(() => { vi.advanceTimersByTime(200); });
    expect(onSeek).toHaveBeenCalledTimes(1);

    // absoluteTime still old (restart in flight) - thumb must NOT snap back.
    rerender(<PlayerProgressBar {...defaultProps} absoluteTime={1800} effectiveDuration={3000} onSeek={onSeek} />);
    expect(slider.value).toBe("600");

    // playhead reaches the target - held value released, tracks real position.
    rerender(<PlayerProgressBar {...defaultProps} absoluteTime={600} effectiveDuration={3000} onSeek={onSeek} />);
    expect(slider.value).toBe("600");
    rerender(<PlayerProgressBar {...defaultProps} absoluteTime={605} effectiveDuration={3000} onSeek={onSeek} />);
    expect(slider.value).toBe("605");
  });

  // The hold must not get stuck if the position never converges (failed seek):
  // a safety timeout releases it back to the live position.
  it("releases the held value after the safety timeout if it never converges", () => {
    const onSeek = vi.fn();
    const { rerender } = render(
      <PlayerProgressBar {...defaultProps} absoluteTime={1800} effectiveDuration={3000} onSeek={onSeek} />,
    );
    const slider = screen.getByRole("slider", { name: "Seek" }) as HTMLInputElement;

    fireEvent.change(slider, { target: { value: "600" } });
    act(() => { vi.advanceTimersByTime(200); });
    expect(slider.value).toBe("600");

    act(() => { vi.advanceTimersByTime(12000); });
    rerender(<PlayerProgressBar {...defaultProps} absoluteTime={1800} effectiveDuration={3000} onSeek={onSeek} />);
    expect(slider.value).toBe("1800");
  });
});
