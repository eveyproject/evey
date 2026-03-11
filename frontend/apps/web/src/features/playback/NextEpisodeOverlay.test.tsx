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
});
