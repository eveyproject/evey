import { describe, expect, it } from "vitest";
import { assToVtt, externalSubtitleKind, srtToVtt, toVtt } from "./subtitleParse";

describe("srtToVtt", () => {
  it("converts SRT comma timestamps to VTT periods under a WEBVTT header", () => {
    const srt = "1\n00:00:01,000 --> 00:00:03,500\nHello\n";
    const vtt = srtToVtt(srt);
    expect(vtt).toMatch(/^WEBVTT/);
    expect(vtt).toContain("00:00:01.000 --> 00:00:03.500");
    expect(vtt).toContain("Hello");
  });
});

describe("assToVtt", () => {
  const ass = [
    "[Script Info]",
    "Title: Sample",
    "[Events]",
    "Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text",
    "Dialogue: 0,0:00:01.00,0:00:03.50,Default,,0,0,0,,{\\i1}Hello{\\i0} world",
    "Dialogue: 0,0:00:04.00,0:00:05.00,Default,,0,0,0,,Line one\\NLine two, with comma",
    "Comment: 0,0:00:09.00,0:00:10.00,Default,,0,0,0,,ignored",
  ].join("\n");

  it("emits a WEBVTT header and one cue per Dialogue line (Comment ignored)", () => {
    const vtt = assToVtt(ass);
    expect(vtt).toMatch(/^WEBVTT/);
    expect((vtt.match(/-->/g) ?? []).length).toBe(2);
    expect(vtt).not.toContain("ignored");
  });

  it("converts centisecond timestamps to VTT milliseconds", () => {
    expect(assToVtt(ass)).toContain("00:00:01.000 --> 00:00:03.500");
  });

  it("strips ASS override tags and converts \\N to newlines, keeping commas in text", () => {
    const vtt = assToVtt(ass);
    expect(vtt).toContain("Hello world");
    expect(vtt).not.toContain("\\i1");
    expect(vtt).toContain("Line one\nLine two, with comma");
  });
});

describe("externalSubtitleKind", () => {
  it("recognizes srt/ass/ssa/vtt case-insensitively", () => {
    expect(externalSubtitleKind("movie.srt")).toBe("srt");
    expect(externalSubtitleKind("movie.SRT")).toBe("srt");
    expect(externalSubtitleKind("movie.ass")).toBe("ass");
    expect(externalSubtitleKind("movie.ssa")).toBe("ass");
    expect(externalSubtitleKind("movie.vtt")).toBe("vtt");
  });

  it("returns null for an unsupported extension", () => {
    expect(externalSubtitleKind("movie.txt")).toBeNull();
    expect(externalSubtitleKind("movie")).toBeNull();
  });
});

describe("toVtt", () => {
  it("converts srt and ass, and passes vtt through unchanged", () => {
    const srt = "1\n00:00:01,000 --> 00:00:03,500\nHello\n";
    expect(toVtt(srt, "srt")).toMatch(/^WEBVTT/);
    const vtt = "WEBVTT\n\n00:00:01.000 --> 00:00:03.500\nHello\n";
    expect(toVtt(vtt, "vtt")).toBe(vtt);
  });
});
