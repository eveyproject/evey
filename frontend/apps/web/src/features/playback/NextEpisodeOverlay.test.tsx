import { render, screen, act } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { NextEpisodeOverlay } from "./NextEpisodeOverlay";

const nextEpisode = { id: "ep-2", title: "The Reckoning", indexNumber: 2 };

function creditsProps(overrides: Record<string, unknown> = {}) {
  return {
    nextEpisode,
    currentTime: 3570,
    duration: 3600,
    onPlayNow: vi.fn(),
    onCancel: vi.fn(),
    ...overrides,
  };
}

describe("NextEpisodeOverlay", () => {
  it("shows overlay content when in credits window", () => {
    render(<NextEpisodeOverlay {...creditsProps()} />);

    expect(screen.getByText("Up Next")).toBeInTheDocument();
    expect(screen.getByText(/Episode 2/)).toBeInTheDocument();
    expect(screen.getByText(/The Reckoning/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Play Now" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeInTheDocument();
  });

  it("calls onPlayNow with episode id on Play Now click", () => {
    const onPlayNow = vi.fn();
    render(<NextEpisodeOverlay {...creditsProps({ onPlayNow })} />);

    act(() => {
      screen.getByRole("button", { name: "Play Now" }).click();
    });

    expect(onPlayNow).toHaveBeenCalledWith("ep-2");
  });

  it("calls onCancel and hides overlay on Cancel click", () => {
    const onCancel = vi.fn();
    render(<NextEpisodeOverlay {...creditsProps({ onCancel })} />);

    act(() => {
      screen.getByRole("button", { name: "Cancel" }).click();
    });

    expect(onCancel).toHaveBeenCalledTimes(1);
    expect(screen.queryByText("Up Next")).not.toBeInTheDocument();
  });

  it("does not render when nextEpisode is null", () => {
    render(
      <NextEpisodeOverlay
        nextEpisode={null}
        currentTime={3570}
        duration={3600}
        onPlayNow={vi.fn()}
        onCancel={vi.fn()}
      />,
    );

    expect(screen.queryByText("Up Next")).not.toBeInTheDocument();
  });

  it("renders Up Next label when in credits but not at end", () => {
    render(
      <NextEpisodeOverlay
        nextEpisode={nextEpisode}
        currentTime={3575}
        duration={3600}
        onPlayNow={vi.fn()}
        onCancel={vi.fn()}
      />,
    );

    expect(screen.getByText("Up Next")).toBeInTheDocument();
  });

  // Anti-regression: when the user seeks backward after the video ended,
  // the overlay should disappear from the mid-episode position. Previously
  // `ended` latched to true permanently, so the countdown UI stuck around
  // even after a backward scrub.
  it("clears the overlay when user seeks backward after video ended", () => {
    const props = {
      nextEpisode,
      onPlayNow: vi.fn(),
      onCancel: vi.fn(),
    };
    const { rerender } = render(
      <NextEpisodeOverlay {...props} currentTime={3599.9} duration={3600} />,
    );
    // At end-of-video the overlay is visible (label depends on whether
    // `ended` effect has flushed; either label is fine here).
    expect(
      screen.queryByText("Up Next") || screen.queryByText(/Next in/),
    ).not.toBeNull();

    // User scrubs back to mid-episode - neither label should appear now.
    rerender(<NextEpisodeOverlay {...props} currentTime={1800} duration={3600} />);
    expect(screen.queryByText(/Next in/)).not.toBeInTheDocument();
    expect(screen.queryByText("Up Next")).not.toBeInTheDocument();
  });

  // The profile's choice: with auto-play off the card still offers the next
  // episode, it just waits for the viewer instead of counting down.
  describe("autoAdvance off", () => {
    it("shows the card at the end without a countdown and never advances on its own", () => {
      vi.useFakeTimers();
      try {
        const onPlayNow = vi.fn();
        // Reach the end the way playback does: from inside the credits.
        const { rerender } = render(<NextEpisodeOverlay {...creditsProps({ onPlayNow, autoAdvance: false })} />);
        rerender(<NextEpisodeOverlay {...creditsProps({ onPlayNow, autoAdvance: false, currentTime: 3599.9 })} />);
        act(() => { vi.advanceTimersByTime(7000); });
        expect(screen.getByText("Up Next")).toBeInTheDocument();
        expect(screen.queryByText(/Next in/)).not.toBeInTheDocument();
        expect(onPlayNow).not.toHaveBeenCalled();
        expect(screen.getByRole("button", { name: "Play Now" })).toBeInTheDocument();
      } finally {
        vi.useRealTimers();
      }
    });

    it("still advances on Play Now", () => {
      const onPlayNow = vi.fn();
      render(<NextEpisodeOverlay {...creditsProps({ onPlayNow, autoAdvance: false, currentTime: 3599.9 })} />);
      act(() => { screen.getByRole("button", { name: "Play Now" }).click(); });
      expect(onPlayNow).toHaveBeenCalledWith("ep-2");
    });
  });

  it("advances on its own by default once the countdown runs out", () => {
    vi.useFakeTimers();
    try {
      const onPlayNow = vi.fn();
      const { rerender } = render(<NextEpisodeOverlay {...creditsProps({ onPlayNow })} />);
      rerender(<NextEpisodeOverlay {...creditsProps({ onPlayNow, currentTime: 3599.9 })} />);
      act(() => { vi.advanceTimersByTime(7000); });
      expect(onPlayNow).toHaveBeenCalledWith("ep-2");
    } finally {
      vi.useRealTimers();
    }
  });

  it("reports its visibility so the skip button can step aside", () => {
    const onVisibleChange = vi.fn();
    const { rerender } = render(<NextEpisodeOverlay {...creditsProps({ onVisibleChange })} />);
    expect(onVisibleChange).toHaveBeenLastCalledWith(true);
    rerender(<NextEpisodeOverlay {...creditsProps({ onVisibleChange, currentTime: 1800 })} />);
    expect(onVisibleChange).toHaveBeenLastCalledWith(false);
  });

  // A "stop at end of episode" sleep timer should just stop playback, never
  // offer to continue - suppressAutoAdvance must hide the overlay and never
  // fire onPlayNow, even once the countdown would otherwise reach zero.
  it("never shows or auto-advances when suppressAutoAdvance is set", () => {
    const onPlayNow = vi.fn();
    render(<NextEpisodeOverlay {...creditsProps({ onPlayNow, suppressAutoAdvance: true })} />);

    expect(screen.queryByText("Up Next")).not.toBeInTheDocument();
    expect(screen.queryByText(/Next in/)).not.toBeInTheDocument();
    expect(onPlayNow).not.toHaveBeenCalled();
  });
});
