import { describe, it, expect } from "vitest";
import type { SubtitleStream } from "./SubtitleSelector";

// Extract the same logic used in HLSPlayer to detect burn-in track
function getSelectedBurnInTrack(
  currentSubtitle: number | null,
  subtitleStreams: SubtitleStream[],
): number | null {
  if (currentSubtitle === null) return null;
  const sub = subtitleStreams.find((s) => s.trackIndex === currentSubtitle);
  return sub?.burnIn ? currentSubtitle : null;
}

// Extract transcode URL param logic (mirrors the useEffect in HLSPlayer)
function buildTranscodeParams(opts: {
  token: string;
  audioTrack: number;
  quality: number;
  canPlayHevc: boolean;
  startTime: number;
  selectedBurnInTrack: number | null;
}): URLSearchParams {
  const params = new URLSearchParams();
  if (opts.token) params.set("token", opts.token);
  params.set("audioTrack", String(opts.audioTrack));
  if (opts.quality > 0) params.set("videoScale", String(opts.quality));
  if (opts.canPlayHevc) params.set("remux", "1");
  if (opts.startTime > 0) params.set("startTime", String(opts.startTime));
  if (opts.selectedBurnInTrack !== null) params.set("subtitleTrack", String(opts.selectedBurnInTrack));
  return params;
}

// Whether a <track> element should be created for the selected subtitle
function shouldCreateTrackElement(
  currentSubtitle: number | null,
  subtitleStreams: SubtitleStream[],
  hasExternalSub: boolean,
): boolean {
  if (hasExternalSub) return true;
  if (currentSubtitle === null) return false;
  const burnIn = getSelectedBurnInTrack(currentSubtitle, subtitleStreams);
  return burnIn === null; // only create <track> for non-burn-in
}

const textSub: SubtitleStream = { index: 3, trackIndex: 0, language: "eng", codec: "subrip" };
const pgsSub: SubtitleStream = { index: 4, trackIndex: 1, language: "fre", codec: "hdmv_pgs_subtitle", burnIn: true };
const vobSub: SubtitleStream = { index: 5, trackIndex: 2, language: "jpn", codec: "dvd_subtitle", burnIn: true };
const streams = [textSub, pgsSub, vobSub];

describe("burn-in subtitle detection", () => {
  it("returns null when no subtitle selected", () => {
    expect(getSelectedBurnInTrack(null, streams)).toBeNull();
  });

  it("returns null for text subtitle (SRT)", () => {
    expect(getSelectedBurnInTrack(0, streams)).toBeNull();
  });

  it("returns trackIndex for PGS subtitle", () => {
    expect(getSelectedBurnInTrack(1, streams)).toBe(1);
  });

  it("returns trackIndex for VobSub subtitle", () => {
    expect(getSelectedBurnInTrack(2, streams)).toBe(2);
  });
});

describe("transcode URL params with burn-in", () => {
  it("adds subtitleTrack param for burn-in subtitle", () => {
    const params = buildTranscodeParams({
      token: "tok",
      audioTrack: 0,
      quality: 0,
      canPlayHevc: false,
      startTime: 0,
      selectedBurnInTrack: 1,
    });
    expect(params.get("subtitleTrack")).toBe("1");
  });

  it("does not add subtitleTrack when no burn-in selected", () => {
    const params = buildTranscodeParams({
      token: "tok",
      audioTrack: 0,
      quality: 0,
      canPlayHevc: false,
      startTime: 0,
      selectedBurnInTrack: null,
    });
    expect(params.has("subtitleTrack")).toBe(false);
  });

  it("includes subtitleTrack alongside other params", () => {
    const params = buildTranscodeParams({
      token: "tok",
      audioTrack: 1,
      quality: 720,
      canPlayHevc: true,
      startTime: 42,
      selectedBurnInTrack: 2,
    });
    expect(params.get("subtitleTrack")).toBe("2");
    expect(params.get("audioTrack")).toBe("1");
    expect(params.get("videoScale")).toBe("720");
    expect(params.get("remux")).toBe("1");
    expect(params.get("startTime")).toBe("42");
  });
});

describe("track element creation for burn-in", () => {
  it("does not create track for burn-in PGS subtitle", () => {
    expect(shouldCreateTrackElement(1, streams, false)).toBe(false);
  });

  it("does not create track for burn-in VobSub subtitle", () => {
    expect(shouldCreateTrackElement(2, streams, false)).toBe(false);
  });

  it("creates track for text subtitle (SRT)", () => {
    expect(shouldCreateTrackElement(0, streams, false)).toBe(true);
  });

  it("does not create track when no subtitle selected", () => {
    expect(shouldCreateTrackElement(null, streams, false)).toBe(false);
  });

  it("creates track for external subtitle even when burn-in exists", () => {
    expect(shouldCreateTrackElement(null, streams, true)).toBe(true);
  });
});
