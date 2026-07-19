import { describe, it, expect } from "vitest";
import { classifyWhere, classifySource, parseLogLine } from "./logsClassify";

describe("classifyWhere/classifySource", () => {
  const cases: Array<[string, string, string, string]> = [
    // component, msg, where, source
    ["", "hls: ffmpeg start", "player", "ffmpeg"],
    ["software_producer", "software: copy seek landed", "player", "ffmpeg"],
    ["", "transcode: decision", "player", "ffmpeg"],
    ["seek_index", "seek-index job finished", "seek-index", "mkvgo"],
    ["seek-index", "reindex accepted", "seek-index", "mkvgo"],
    ["", "mkvgo remux: dropped track", "seek-index", "mkvgo"],
    ["", "subtitle: mkvgo WebVTT extraction failed", "subtitle", "mkvgo"],
    ["", "remux: produce failed", "remux", "other"],
    ["", "hwdetect: probe failed", "hw-detect", "ffmpeg"],
    ["qoe_sink", "qoe ingest", "qoe", "qoe"],
    ["", "chromecast: session started", "chromecast", "other"],
    ["", "refresh rejected: token reuse detected", "auth", "other"],
    ["grpc", "call", "infra", "grpc"],
    ["", "something random", "other", "other"],
  ];
  for (const [component, msg, where, source] of cases) {
    it(`${component || "-"} / ${msg}`, () => {
      expect(classifyWhere(component, msg)).toBe(where);
      expect(classifySource(component, msg)).toBe(source);
    });
  }
});

describe("parseLogLine", () => {
  it("parses, promotes and normalizes ids", () => {
    const line = `{"time":"2026-07-07T19:22:01Z","level":"info","msg":"hls: ffmpeg start","sessionID":"ps-1","user_id":"usr-2","variant":0}`;
    const e = parseLogLine(line);
    expect(e).not.toBeNull();
    expect(e?.level).toBe("INFO");
    expect(e?.where).toBe("player");
    expect(e?.source).toBe("ffmpeg");
    expect(e?.sessionId).toBe("ps-1");
    expect(e?.userId).toBe("usr-2");
    expect(e?.origin).toBe("server");
    expect(e?.fields?.variant).toBe(0);
    expect(e?.fields?.level).toBeUndefined();
  });

  it("returns null for non-JSON", () => {
    expect(parseLogLine("panic: not json")).toBeNull();
  });
});

describe("parseLogLine uid", () => {
  // The expanded-row-collapses bug: rows were keyed by array index, and live
  // tail PREPENDS each new line, so every existing row's key changed on every
  // tick and React remounted it - throwing away the open state of whatever the
  // operator was reading. Identity has to come from the entry, not its slot.
  it("gives every parsed entry a distinct stable id", () => {
    const line = '{"time":"2026-07-19T18:01:03Z","level":"WARN","msg":"same"}';
    const a = parseLogLine(line);
    const b = parseLogLine(line);
    expect(a?.uid).toBeTruthy();
    expect(b?.uid).toBeTruthy();
    expect(a?.uid).not.toBe(b?.uid);
  });
});
