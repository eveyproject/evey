import { describe, it, expect } from "vitest";
import type { SubtitleStream } from "./SubtitleSelector";
import {
  getSelectedBurnInTrack,
  buildTranscodeHlsUrl,
  resolveDefaultAudio,
  resolveDefaultSubtitle,
} from "./hooks/hlsPlayerUtils";

function shouldCreateTrackElement(
  currentSubtitle: number | null,
  subtitleStreams: SubtitleStream[],
  hasExternalSub: boolean,
): boolean {
  if (hasExternalSub) return true;
  if (currentSubtitle === null) return false;
  const burnIn = getSelectedBurnInTrack(currentSubtitle, subtitleStreams);
  return burnIn === null;
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

describe("buildTranscodeHlsUrl", () => {
  it("builds URL with token and audioTrack", () => {
    const url = buildTranscodeHlsUrl("media-1", "tok", 0, null, 0);
    expect(url).toContain("/api/v1/media/media-1/hls/master.m3u8");
    expect(url).toContain("token=tok");
    expect(url).toContain("audioTrack=0");
    expect(url).not.toContain("subtitleTrack");
    expect(url).not.toContain("startSec");
  });

  it("includes subtitleTrack for burn-in subtitle", () => {
    const url = buildTranscodeHlsUrl("media-1", "tok", 0, 1, 0);
    expect(url).toContain("subtitleTrack=1");
  });

  it("includes startSec when offset > 0", () => {
    const url = buildTranscodeHlsUrl("media-1", "tok", 0, null, 42);
    expect(url).toContain("startSec=42");
  });

  it("includes all params together", () => {
    const url = buildTranscodeHlsUrl("media-1", "tok", 1, 2, 42);
    expect(url).toContain("token=tok");
    expect(url).toContain("audioTrack=1");
    expect(url).toContain("subtitleTrack=2");
    expect(url).toContain("startSec=42");
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

describe("resolveDefaultAudio", () => {
  const audioStreams = [
    { index: 0, trackIndex: 0, language: "eng", isDefault: true },
    { index: 1, trackIndex: 1, language: "fre" },
    { index: 2, trackIndex: 2, language: "jpn" },
  ];

  it("returns preferred lang match", () => {
    const result = resolveDefaultAudio(audioStreams, undefined, () => null, () => "fre");
    expect(result?.index).toBe(1);
  });

  it("returns series pref over global pref", () => {
    const result = resolveDefaultAudio(audioStreams, "series-1", () => ({ audioLang: "jpn" }), () => "fre");
    expect(result?.index).toBe(2);
  });

  // A manual pick on one film used to become a device-wide override that beat
  // the profile forever: an account set to French started every next film on
  // the English track remembered from a one-off VO viewing (2026-09-03).
  it("profile langs win over the device-local pick", () => {
    const result = resolveDefaultAudio(audioStreams, undefined, () => null, () => "eng", ["fr"]);
    expect(result?.index).toBe(1);
  });

  it("device-local pick still applies when the profile has no matching lang", () => {
    const result = resolveDefaultAudio(audioStreams, undefined, () => null, () => "jpn", ["deu"]);
    expect(result?.index).toBe(2);
  });

  it("falls back to default stream", () => {
    const result = resolveDefaultAudio(audioStreams, undefined, () => null, () => null);
    expect(result?.index).toBe(0);
  });

  it("falls back to first stream when no default", () => {
    const noDefault = audioStreams.map((s) => ({ ...s, isDefault: false }));
    const result = resolveDefaultAudio(noDefault, undefined, () => null, () => null);
    expect(result?.index).toBe(0);
  });
});

describe("resolveDefaultSubtitle", () => {
  const subs: SubtitleStream[] = [
    { index: 0, trackIndex: 0, language: "eng", codec: "subrip" },
    { index: 1, trackIndex: 1, language: "fre", codec: "subrip" },
  ];

  it("returns null when no pref", () => {
    const result = resolveDefaultSubtitle(subs, undefined, () => null, () => null);
    expect(result).toBeNull();
  });

  it("returns matching lang pref", () => {
    const result = resolveDefaultSubtitle(subs, undefined, () => null, () => "fre");
    expect(result).toBe(1);
  });

  it("returns null when series pref explicitly sets null", () => {
    const result = resolveDefaultSubtitle(subs, "s1", () => ({ subtitleLang: null }), () => "fre");
    expect(result).toBeNull();
  });

  it("returns series pref over global", () => {
    const result = resolveDefaultSubtitle(subs, "s1", () => ({ subtitleLang: "eng" }), () => "fre");
    expect(result).toBe(0);
  });
});
