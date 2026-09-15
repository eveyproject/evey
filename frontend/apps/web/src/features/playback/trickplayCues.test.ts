import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";

const eveyFetchRaw = vi.fn();
vi.mock("@evey/api", () => ({ eveyFetchRaw: (...args: unknown[]) => eveyFetchRaw(...args) }));

import { tileForTime, useTrickplayCues, type TrickplayCue } from "./trickplayCues";

const vtt = `WEBVTT

00:00:00.000 --> 00:00:10.000
/api/v1/media/m1/trickplay/trickplay_001.webp?token=t#xywh=0,0,320,180

00:00:10.000 --> 00:00:20.000
/api/v1/media/m1/trickplay/trickplay_001.webp?token=t#xywh=320,0,320,180

01:02:03.500 --> 01:02:13.500
/api/v1/media/m1/trickplay/trickplay_002.webp?token=t#xywh=0,180,320,180

00:00:30.000 --> 00:00:40.000
not-a-tile-line
`;

describe("trickplay cues", () => {
  beforeEach(() => {
    eveyFetchRaw.mockReset();
    eveyFetchRaw.mockResolvedValue({ text: () => Promise.resolve(vtt) });
  });

  it("parses the sprite-sheet VTT: times, tile rect, and the url kept with its token", async () => {
    const { result } = renderHook(() => useTrickplayCues("m1", "MEDIA_ITEM_TYPE_MOVIE"));
    await waitFor(() => expect(result.current).toHaveLength(3));
    const [first, , late] = result.current;
    expect(first).toEqual({ startTime: 0, endTime: 10, url: "/api/v1/media/m1/trickplay/trickplay_001.webp?token=t", x: 0, y: 0, w: 320, h: 180 });
    expect(late.startTime).toBeCloseTo(3723.5);
    expect(late.y).toBe(180);
    // The same media asked for again is served from the module cache.
    const again = renderHook(() => useTrickplayCues("m1", "MEDIA_ITEM_TYPE_MOVIE"));
    expect(again.result.current).toHaveLength(3);
    expect(eveyFetchRaw).toHaveBeenCalledTimes(1);
  });

  it("fetches nothing for a type that has no trickplay, or without a media id", () => {
    renderHook(() => useTrickplayCues("s1", "MEDIA_ITEM_TYPE_SERIES"));
    renderHook(() => useTrickplayCues(undefined));
    expect(eveyFetchRaw).not.toHaveBeenCalled();
  });

  it("reads as no cues when the manifest cannot be fetched", async () => {
    eveyFetchRaw.mockRejectedValueOnce(new Error("404"));
    const { result } = renderHook(() => useTrickplayCues("m-missing", "MEDIA_ITEM_TYPE_EPISODE"));
    await waitFor(() => expect(eveyFetchRaw).toHaveBeenCalled());
    expect(result.current).toEqual([]);
  });

  it("tileForTime picks the cue covering the position, start inclusive, end exclusive", () => {
    const cues: TrickplayCue[] = [
      { startTime: 0, endTime: 10, url: "a", x: 0, y: 0, w: 1, h: 1 },
      { startTime: 10, endTime: 20, url: "b", x: 1, y: 0, w: 1, h: 1 },
    ];
    expect(tileForTime(cues, 0)?.url).toBe("a");
    expect(tileForTime(cues, 9_999)?.url).toBe("a");
    expect(tileForTime(cues, 10_000)?.url).toBe("b");
    expect(tileForTime(cues, 20_000)).toBeNull();
    expect(tileForTime([], 5)).toBeNull();
  });
});
