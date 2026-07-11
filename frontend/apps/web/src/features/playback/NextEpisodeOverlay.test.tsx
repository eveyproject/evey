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
});
