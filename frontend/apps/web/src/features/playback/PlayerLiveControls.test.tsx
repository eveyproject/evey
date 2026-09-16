import { describe, it, expect, vi } from "vitest";
import { render, renderHook, screen, act } from "@testing-library/react";
import React from "react";

const ctx = vi.hoisted(() => ({ time: { absoluteTime: 12, bufferedEnd: 30 } }));
vi.mock("./PlayerContext", () => ({ usePlayerTime: () => ctx.time }));
vi.mock("./PlayerProgressBar", () => ({
  PlayerProgressBar: ({ absoluteTime, bufferedEnd }: any) => React.createElement("div", { "data-testid": "bar", "data-t": absoluteTime, "data-b": bufferedEnd }),
}));
vi.mock("./PlayerChaptersPanel", () => ({ ChaptersPanel: ({ currentTimeSec }: any) => React.createElement("div", { "data-testid": "chapters", "data-t": currentTimeSec }) }));
vi.mock("@evey/api", () => ({ formatTime: (s: number) => `${s}s` }));

import { LiveProgressBar, LiveTimeLabel, LiveChaptersPanel, useActiveChapterIndex } from "./PlayerLiveControls";

const chapters = [
  { startTimeMs: 0, endTimeMs: 60_000, title: "A" },
  { startTimeMs: 60_000, endTimeMs: 120_000, title: "B" },
] as any[];

// The leaf controls read the ~4x/s position from the player context, except
// while casting where the device's position (a prop) wins; the active
// chapter index follows the <video> timeupdate and only changes on a
// boundary crossing, from the cast position when casting.
describe("PlayerLiveControls", () => {
  it("reads the context position, overridden by the cast position", () => {
    const noop = () => {};
    const { rerender } = render(
      <>
        <LiveProgressBar effectiveDuration={100} transcodeOffset={0} chapters={[]} onSeek={noop} onChapterSeek={noop} />
        <LiveTimeLabel effectiveDuration={100} />
        <LiveChaptersPanel chapters={[]} onSeek={noop} open onToggle={noop} />
      </>,
    );
    expect(screen.getByTestId("bar").getAttribute("data-t")).toBe("12");
    expect(screen.getByTestId("bar").getAttribute("data-b")).toBe("30");
    expect(screen.getByText("12s / 100s")).toBeInTheDocument();
    expect(screen.getByTestId("chapters").getAttribute("data-t")).toBe("12");
    rerender(
      <>
        <LiveProgressBar castTimeSec={55} effectiveDuration={100} transcodeOffset={0} chapters={[]} onSeek={noop} onChapterSeek={noop} />
        <LiveTimeLabel castTimeSec={55} effectiveDuration={100} />
        <LiveChaptersPanel castTimeSec={55} chapters={[]} onSeek={noop} open onToggle={noop} />
      </>,
    );
    expect(screen.getByTestId("bar").getAttribute("data-t")).toBe("55");
    expect(screen.getByText("55s / 100s")).toBeInTheDocument();
    expect(screen.getByTestId("chapters").getAttribute("data-t")).toBe("55");
  });

  it("tracks the chapter under the playhead from the video, offset by the transcode start, or from the cast position", () => {
    const video = document.createElement("video");
    Object.defineProperty(video, "currentTime", { configurable: true, writable: true, value: 10 });
    const ref = { current: video };
    const { result, rerender } = renderHook(({ offset, cast }: { offset: number; cast?: number }) => useActiveChapterIndex(chapters, ref, offset, cast), {
      initialProps: { offset: 0, cast: undefined as number | undefined },
    });
    expect(result.current).toBe(0);
    (video as { currentTime: number }).currentTime = 70;
    act(() => video.dispatchEvent(new Event("timeupdate")));
    expect(result.current).toBe(1);
    (video as { currentTime: number }).currentTime = 500;
    act(() => video.dispatchEvent(new Event("timeupdate")));
    expect(result.current).toBe(-1);

    rerender({ offset: 60, cast: undefined });
    (video as { currentTime: number }).currentTime = 5;
    act(() => video.dispatchEvent(new Event("timeupdate")));
    expect(result.current).toBe(1);

    rerender({ offset: 0, cast: 30 });
    expect(result.current).toBe(0);
    (video as { currentTime: number }).currentTime = 90;
    act(() => video.dispatchEvent(new Event("timeupdate")));
    expect(result.current).toBe(0);

    const none = renderHook(() => useActiveChapterIndex(chapters, { current: null }, 0));
    expect(none.result.current).toBe(-1);
  });
});
