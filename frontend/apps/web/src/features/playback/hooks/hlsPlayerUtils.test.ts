import { describe, it, expect, vi } from "vitest";
import type { SubtitleStream } from "../SubtitleSelector";

vi.mock("@/utils/codec", () => ({
  canPlayHevc: false,
  getBrowserCapabilities: () => ({
    videoCodecs: ["h264"],
    audioCodecs: ["aac", "mp3", "opus"],
    containers: ["mp4"],
  }),
}));

import {
  getSelectedBurnInTrack,
  buildTranscodeHlsUrl,
  resolveDefaultAudio,
  resolveDefaultSubtitle,
  shouldRestartTranscodeSeek,
} from "./hlsPlayerUtils";

// Minimal TimeRanges stub: one contiguous buffered range [start, end].
function bufferedRange(start: number, end: number): TimeRanges {
  return {
    length: 1,
    start: (i: number) => (i === 0 ? start : NaN),
    end: (i: number) => (i === 0 ? end : NaN),
  } as TimeRanges;
}
const noBuffer = { length: 0, start: () => NaN, end: () => NaN } as TimeRanges;

const textSub: SubtitleStream = { index: 3, trackIndex: 0, language: "eng", codec: "subrip" };
const pgsSub: SubtitleStream = { index: 4, trackIndex: 1, language: "fre", codec: "hdmv_pgs_subtitle", burnIn: true };
const vobSub: SubtitleStream = { index: 5, trackIndex: 2, language: "jpn", codec: "dvd_subtitle", burnIn: true };
const streams = [textSub, pgsSub, vobSub];

describe("shouldRestartTranscodeSeek", () => {
  // offset 0, currently watching ~30min: the sliding window only buffers near
  // the playhead (1770..1830 local). This is the reported bug case.
  const window = bufferedRange(1770, 1830);

  it("restarts on a backward seek before the buffered window (30min -> 10min)", () => {
    // target 600s, offset 0 -> localTime 600, which is < buffered.start (1770).
    expect(shouldRestartTranscodeSeek(600, 0, window)).toBe(true);
  });

  it("restarts on a forward seek past the buffered window + tolerance", () => {
    expect(shouldRestartTranscodeSeek(1900, 0, window)).toBe(true);
  });

  it("seeks in place within the buffered window", () => {
    expect(shouldRestartTranscodeSeek(1800, 0, window)).toBe(false);
  });

  it("allows a small forward seek inside the 10s live-edge tolerance", () => {
    expect(shouldRestartTranscodeSeek(1838, 0, window)).toBe(false); // local 1838 <= 1830 + 10
    expect(shouldRestartTranscodeSeek(1841, 0, window)).toBe(true); // local 1841 > 1830 + 10
  });

  it("accounts for the transcode offset (local = absolute - offset)", () => {
    // transcode restarted at 600; buffer holds local 0..60 (absolute 600..660).
    const w = bufferedRange(0, 60);
    expect(shouldRestartTranscodeSeek(630, 600, w)).toBe(false); // local 30, in window
    expect(shouldRestartTranscodeSeek(500, 600, w)).toBe(true); // local -100, before start
    expect(shouldRestartTranscodeSeek(1800, 600, w)).toBe(true); // local 1200, past window
  });

  it("restarts when nothing is buffered yet", () => {
    expect(shouldRestartTranscodeSeek(600, 0, noBuffer)).toBe(true);
  });
});

describe("getSelectedBurnInTrack", () => {
  it("returns null when no subtitle selected", () => {
    expect(getSelectedBurnInTrack(null, streams)).toBeNull();
  });

  it("returns null for text-based subtitle", () => {
    expect(getSelectedBurnInTrack(0, streams)).toBeNull();
  });

  it("returns trackIndex for PGS burn-in", () => {
    expect(getSelectedBurnInTrack(1, streams)).toBe(1);
  });

  it("returns trackIndex for VobSub burn-in", () => {
    expect(getSelectedBurnInTrack(2, streams)).toBe(2);
  });

  it("returns null for empty streams array", () => {
    expect(getSelectedBurnInTrack(0, [])).toBeNull();
  });

  it("returns null when trackIndex not found in streams", () => {
    expect(getSelectedBurnInTrack(99, streams)).toBeNull();
  });
});

describe("buildTranscodeHlsUrl", () => {
  it("builds URL with token, audioTrack, and codec params", () => {
    const url = buildTranscodeHlsUrl("m1", "tok", 0, null, 0);
    expect(url).toContain("token=tok");
    expect(url).toContain("audioTrack=0");
    expect(url).toContain("videoCodecs=h264");
    expect(url).toContain("audioCodecs=aac%2Cmp3%2Copus");
    expect(url).toContain("containers=mp4");
  });

  it("includes subtitleTrack when burn-in selected", () => {
    const url = buildTranscodeHlsUrl("m1", "tok", 0, 2, 0);
    expect(url).toContain("subtitleTrack=2");
  });

  it("includes startSec when > 0", () => {
    const url = buildTranscodeHlsUrl("m1", "tok", 0, null, 120);
    expect(url).toContain("startSec=120");
  });

  it("omits subtitleTrack and startSec when not needed", () => {
    const url = buildTranscodeHlsUrl("m1", "tok", 1, null, 0);
    expect(url).not.toContain("subtitleTrack");
    expect(url).not.toContain("startSec");
  });

  it("omits token param when token is empty string", () => {
    const url = buildTranscodeHlsUrl("m1", "", 0, null, 0);
    expect(url).not.toContain("token=");
  });

  it("includes all params together", () => {
    const url = buildTranscodeHlsUrl("m1", "t", 3, 1, 55);
    expect(url).toContain("token=t");
    expect(url).toContain("audioTrack=3");
    expect(url).toContain("subtitleTrack=1");
    expect(url).toContain("startSec=55");
  });
});

describe("resolveDefaultAudio", () => {
  const audioStreams = [
    { index: 0, trackIndex: 0, language: "eng", isDefault: true },
    { index: 1, trackIndex: 1, language: "fre" },
    { index: 2, trackIndex: 2, language: "jpn" },
  ];

  it("returns match for global preferred lang", () => {
    const result = resolveDefaultAudio(audioStreams, undefined, () => null, () => "fre");
    expect(result).toEqual(audioStreams[1]);
  });

  it("series pref overrides global pref", () => {
    const result = resolveDefaultAudio(audioStreams, "s1", () => ({ audioLang: "jpn" }), () => "fre");
    expect(result).toEqual(audioStreams[2]);
  });

  it("falls back to default stream when no pref", () => {
    const result = resolveDefaultAudio(audioStreams, undefined, () => null, () => null);
    expect(result).toEqual(audioStreams[0]);
  });

  it("falls back to first stream when none is default", () => {
    const noDefault = audioStreams.map((s) => ({ ...s, isDefault: false }));
    const result = resolveDefaultAudio(noDefault, undefined, () => null, () => null);
    expect(result).toEqual(noDefault[0]);
  });

  it("matches case-insensitively", () => {
    const result = resolveDefaultAudio(audioStreams, undefined, () => null, () => "ENG");
    expect(result).toEqual(audioStreams[0]);
  });

  it("matches on title when language is missing", () => {
    const titleStreams = [
      { index: 0, trackIndex: 0, title: "English" },
      { index: 1, trackIndex: 1, title: "French" },
    ];
    const result = resolveDefaultAudio(titleStreams, undefined, () => null, () => "french");
    expect(result).toEqual(titleStreams[1]);
  });

  it("ignores series pref when seriesId is undefined", () => {
    const result = resolveDefaultAudio(audioStreams, undefined, () => ({ audioLang: "jpn" }), () => "fre");
    expect(result).toEqual(audioStreams[1]);
  });
});

describe("resolveDefaultSubtitle", () => {
  const subs: SubtitleStream[] = [
    { index: 0, trackIndex: 0, language: "eng", codec: "subrip" },
    { index: 1, trackIndex: 1, language: "fre", codec: "subrip" },
  ];

  it("returns null when no preferences at all", () => {
    expect(resolveDefaultSubtitle(subs, undefined, () => null, () => null)).toBeNull();
  });

  it("returns matching global pref", () => {
    expect(resolveDefaultSubtitle(subs, undefined, () => null, () => "fre")).toBe(1);
  });

  it("returns null when series pref explicitly null", () => {
    expect(resolveDefaultSubtitle(subs, "s1", () => ({ subtitleLang: null }), () => "fre")).toBeNull();
  });

  it("series pref overrides global pref", () => {
    expect(resolveDefaultSubtitle(subs, "s1", () => ({ subtitleLang: "eng" }), () => "fre")).toBe(0);
  });

  it("returns null when global pref has no match", () => {
    expect(resolveDefaultSubtitle(subs, undefined, () => null, () => "ger")).toBeNull();
  });

  it("matches case-insensitively", () => {
    expect(resolveDefaultSubtitle(subs, undefined, () => null, () => "FRE")).toBe(1);
  });

  it("returns null when series pref subtitleLang is undefined", () => {
    expect(resolveDefaultSubtitle(subs, "s1", () => ({ subtitleLang: undefined }), () => "fre")).toBeNull();
  });

  it("matches on title when language missing", () => {
    const titleSubs: SubtitleStream[] = [
      { index: 0, trackIndex: 0, title: "English", codec: "subrip" },
      { index: 1, trackIndex: 1, title: "French", codec: "subrip" },
    ];
    expect(resolveDefaultSubtitle(titleSubs, undefined, () => null, () => "french")).toBe(1);
  });

  it("returns null for empty streams with a pref", () => {
    expect(resolveDefaultSubtitle([], undefined, () => null, () => "eng")).toBeNull();
  });
});
