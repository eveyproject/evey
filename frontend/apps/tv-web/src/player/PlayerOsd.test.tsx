import type React from "react";
import { describe, it, expect, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (k: string, o?: unknown) => (typeof o === "string" ? o : k),
  }),
}));
vi.mock("./TrickplayTile", () => ({ TrickplayTile: () => null }));
vi.mock("../components/icons", () => ({ Icon: ({ name }: { name: string }) => <i data-icon={name} /> }));
vi.mock("../components/ui", () => ({
  Pressable: ({ children, onPress }: { children?: React.ReactNode; onPress?: () => void }) => <button onClick={onPress}>{children}</button>,
}));
vi.mock("../tv/focus", () => ({
  FocusRoot: ({ children }: { children?: React.ReactNode }) => <div>{children}</div>,
}));

import { PlayerOsd } from "./PlayerOsd";
import { NextEpisodeCard } from "./NextEpisodeCard";

const base = {
  mediaId: "m-1",
  title: "A film",
  syncConnected: false,
  quality: 0,
  shown: 60,
  duration: 600,
  preview: null,
  pendingDelta: 0,
  endsAt: null,
  paused: false,
  speed: 1,
  skipBack: 10,
  skipFwd: 30,
  chapters: [],
  barFocus: null,
  onPress: vi.fn(),
};

describe("PlayerOsd (tv-web)", () => {
  it("lights the control the remote is on and hands a click back as its key", () => {
    const onPress = vi.fn();
    const { container } = render(<PlayerOsd {...base} barFocus="forward" onPress={onPress} />);
    const forward = screen.getByText("+30s");
    expect(forward.className).toContain("focused");
    fireEvent.click(screen.getByText("-10s"));
    expect(onPress).toHaveBeenCalledWith("rewind");
    expect(container.querySelector(".osd-knob")).toBeNull();
  });

  it("fills the timeline to the shown position and says how far a pending jump goes", () => {
    const { container } = render(<PlayerOsd {...base} shown={150} pendingDelta={-30} barFocus="timeline" />);
    expect((container.querySelector(".osd-fill") as HTMLElement).style.width).toBe("25%");
    expect(container.querySelector(".osd-knob")).not.toBeNull();
    expect(screen.getByText("-0:30")).toBeTruthy();
  });

  it("marks the chapters inside the film, not the ones at its edges", () => {
    const chapters = [
      { title: "a", startTimeMs: 0, endTimeMs: 1 },
      { title: "b", startTimeMs: 300_000, endTimeMs: 1 },
    ];
    const { container } = render(<PlayerOsd {...base} chapters={chapters as never} />);
    expect(container.querySelectorAll(".osd-notch")).toHaveLength(1);
  });
});

describe("NextEpisodeCard (tv-web)", () => {
  it("names the next episode and plays it on Play now", () => {
    const playNow = vi.fn();
    const nextEp = { nextEpisode: { indexNumber: 4, seasonIndexNumber: 2, title: "The box" }, ended: false, countdown: 10, playNow, cancel: vi.fn() };
    render(<NextEpisodeCard nextEp={nextEp as never} />);
    expect(screen.getByText("S02E04 · The box")).toBeTruthy();
    fireEvent.click(screen.getByText("Play now"));
    expect(playNow).toHaveBeenCalled();
  });
});
