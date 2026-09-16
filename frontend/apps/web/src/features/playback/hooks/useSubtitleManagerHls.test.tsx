import { describe, it, expect, vi } from "vitest";
import { renderHook, act } from "@testing-library/react";
import type Hls from "hls.js";
import { hlsSubtitleTrackFor, useSubtitleManager } from "./useSubtitleManager";
import type { SubtitleStyle } from "@/hooks/usePlaybackPrefs";

// The master's subtitle renditions carry the picker's ordinal (X-EVEY-TRACK):
// the selected track is served segment by segment by the packager instead of
// the /subtitle route, which extracted the whole film first.
function fakeHls(tracks: Array<{ id: number; track: string }>) {
  const handlers: Record<string, Array<() => void>> = {};
  const hls = {
    subtitleTracks: tracks.map((t) => ({ id: t.id, attrs: { "X-EVEY-TRACK": t.track } })),
    subtitleTrack: -1,
    subtitleDisplay: false,
    on: vi.fn((ev: string, h: () => void) => { (handlers[ev] ??= []).push(h); }),
    off: vi.fn(),
    fire: (ev: string) => (handlers[ev] ?? []).forEach((h) => h()),
  };
  return hls as unknown as InstanceType<typeof Hls> & { fire: (ev: string) => void };
}

const style: SubtitleStyle = { position: "bottom", fontSize: 100, color: "#fff", fontFamily: "", bgColor: "#000000", bgOpacity: 0, shadow: true } as SubtitleStyle;

function renderManager(hls: InstanceType<typeof Hls> | null, currentSubtitle: number | null) {
  const video = document.createElement("video");
  Object.defineProperty(video, "textTracks", { value: [] });
  const videoRef = { current: video };
  const hlsRef = { current: hls };
  renderHook(() =>
    useSubtitleManager({
      videoRef, hlsRef, currentSubtitle, externalSubUrl: null, mediaId: "m1", streamToken: "tok",
      selectedBurnInTrack: null, transcodeStartTime: 0, currentTrackIndex: 0, subtitleStyle: style,
    }),
  );
  return { video, hls };
}

describe("hlsSubtitleTrackFor", () => {
  it("finds the rendition tagged with the picker's ordinal, or -1", () => {
    const hls = fakeHls([{ id: 0, track: "0" }, { id: 1, track: "2" }]);
    expect(hlsSubtitleTrackFor(hls, 2)).toBe(1);
    expect(hlsSubtitleTrackFor(hls, 1)).toBe(-1);
    expect(hlsSubtitleTrackFor(hls, null)).toBe(-1);
    expect(hlsSubtitleTrackFor(null, 0)).toBe(-1);
  });
});

describe("useSubtitleManager with packager renditions", () => {
  it("selects the hls.js subtitle track and creates no <track> element", () => {
    const { video, hls } = renderManager(fakeHls([{ id: 0, track: "0" }, { id: 1, track: "2" }]), 2);
    expect(hls!.subtitleTrack).toBe(1);
    expect(hls!.subtitleDisplay).toBe(true);
    expect(video.querySelector("track[data-dynamic]")).toBeNull();
  });

  it("falls back to the /subtitle route only once the manifest is known to lack the rendition", () => {
    const hls = fakeHls([{ id: 0, track: "0" }]);
    const { video } = renderManager(hls, 1);
    // Manifest not parsed yet: no fallback, that would start a full-file extraction for nothing.
    expect(video.querySelector("track[data-dynamic]")).toBeNull();
    act(() => hls.fire("hlsManifestParsed"));
    expect(hls.subtitleTrack).toBe(-1);
    const track = video.querySelector("track[data-dynamic]") as HTMLTrackElement | null;
    expect(track?.getAttribute("src")).toBe("/api/v1/media/m1/subtitle/1?token=tok");
  });

  it("turns the hls.js track off when subtitles are deselected", () => {
    const hls = fakeHls([{ id: 0, track: "0" }]);
    hls.subtitleTrack = 0;
    const { video } = renderManager(hls, null);
    expect(hls.subtitleTrack).toBe(-1);
    expect(video.querySelector("track[data-dynamic]")).toBeNull();
  });
});
