import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { SegmentTimeline } from "./SegmentTimeline";
import type { MediaSegment } from "./SegmentTimeline";

const introSegment: MediaSegment = {
  type: "intro",
  startMs: 0,
  endMs: 90000,
  confidence: 0.9,
  source: "trickplay",
};

const recapSegment: MediaSegment = {
  type: "recap",
  startMs: 90000,
  endMs: 120000,
  confidence: 0.8,
  source: "manual",
};

const endCreditsSegment: MediaSegment = {
  type: "end_credits",
  startMs: 2400000,
  endMs: 2520000,
  confidence: 0.7,
  source: "manual_propagated",
};

describe("SegmentTimeline", () => {
  it("renders a colored block for each segment", () => {
    render(
      <SegmentTimeline
        segments={[introSegment, recapSegment]}
        durationMs={2700000}
        selectedType={null}
        onSelectSegment={vi.fn()}
        onSegmentChange={vi.fn()}
      />,
    );

    expect(screen.getByTestId("segment-block-intro")).toBeInTheDocument();
    expect(screen.getByTestId("segment-block-recap")).toBeInTheDocument();
  });

  it("clicking a block calls onSelectSegment with the correct type", () => {
    const onSelect = vi.fn();
    render(
      <SegmentTimeline
        segments={[introSegment]}
        durationMs={2700000}
        selectedType={null}
        onSelectSegment={onSelect}
        onSegmentChange={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByTestId("segment-block-intro"));
    expect(onSelect).toHaveBeenCalledWith("intro");
  });

  it("selected block has aria-pressed=true", () => {
    render(
      <SegmentTimeline
        segments={[introSegment, recapSegment]}
        durationMs={2700000}
        selectedType="intro"
        onSelectSegment={vi.fn()}
        onSegmentChange={vi.fn()}
      />,
    );

    const introBlock = screen.getByTestId("segment-block-intro");
    const recapBlock = screen.getByTestId("segment-block-recap");
    expect(introBlock).toHaveAttribute("aria-pressed", "true");
    expect(recapBlock).toHaveAttribute("aria-pressed", "false");
  });

  it("shows selected segment editor with sliders when a segment is selected", () => {
    render(
      <SegmentTimeline
        segments={[introSegment]}
        durationMs={2700000}
        selectedType="intro"
        onSelectSegment={vi.fn()}
        onSegmentChange={vi.fn()}
      />,
    );

    expect(screen.getByTestId("start-slider-intro")).toBeInTheDocument();
    expect(screen.getByTestId("end-slider-intro")).toBeInTheDocument();
  });

  it("dragging the start slider calls onSegmentChange", () => {
    const onChange = vi.fn();
    render(
      <SegmentTimeline
        segments={[introSegment]}
        durationMs={2700000}
        selectedType="intro"
        onSelectSegment={vi.fn()}
        onSegmentChange={onChange}
      />,
    );

    fireEvent.change(screen.getByTestId("start-slider-intro"), {
      target: { value: "5000" },
    });
    expect(onChange).toHaveBeenCalledWith("intro", 5000, introSegment.endMs);
  });

  it("dragging the end slider calls onSegmentChange", () => {
    const onChange = vi.fn();
    render(
      <SegmentTimeline
        segments={[introSegment]}
        durationMs={2700000}
        selectedType="intro"
        onSelectSegment={vi.fn()}
        onSegmentChange={onChange}
      />,
    );

    fireEvent.change(screen.getByTestId("end-slider-intro"), {
      target: { value: "95000" },
    });
    expect(onChange).toHaveBeenCalledWith("intro", introSegment.startMs, 95000);
  });

  it("shows lock icon for source=manual", () => {
    render(
      <SegmentTimeline
        segments={[recapSegment]}
        durationMs={2700000}
        selectedType={null}
        onSelectSegment={vi.fn()}
        onSegmentChange={vi.fn()}
      />,
    );

    expect(screen.getByTestId("lock-icon")).toBeInTheDocument();
  });

  it("shows lock icon for source=manual_propagated", () => {
    render(
      <SegmentTimeline
        segments={[endCreditsSegment]}
        durationMs={2700000}
        selectedType={null}
        onSelectSegment={vi.fn()}
        onSegmentChange={vi.fn()}
      />,
    );

    expect(screen.getByTestId("lock-icon")).toBeInTheDocument();
  });

  it("does not show lock icon for source=trickplay", () => {
    render(
      <SegmentTimeline
        segments={[introSegment]}
        durationMs={2700000}
        selectedType={null}
        onSelectSegment={vi.fn()}
        onSegmentChange={vi.fn()}
      />,
    );

    expect(screen.queryByTestId("lock-icon")).not.toBeInTheDocument();
  });

  it("block has correct proportional width style", () => {
    render(
      <SegmentTimeline
        segments={[introSegment]}
        durationMs={900000}
        selectedType={null}
        onSelectSegment={vi.fn()}
        onSegmentChange={vi.fn()}
      />,
    );

    const block = screen.getByTestId("segment-block-intro");
    // intro is 0-90000 out of 900000 = 10%
    expect(block).toHaveStyle({ width: "10%" });
  });
});
