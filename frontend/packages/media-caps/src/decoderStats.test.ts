import { describe, expect, it } from "vitest";
import { formatDecoderStats, framesToSec, isSilentRunAlarming, parseDecoderStats, type DecoderStats } from "./decoderStats";

const base: DecoderStats = {
  codec: "eac3",
  sourceChannels: 6,
  outputChannels: 2,
  maxChannels: 2,
  sampleRate: 48000,
  frames: 1924,
  badFrames: 0,
  silentFrames: 0,
  silentRunFrames: 0,
  decoder: "err=0/0/0/0/0/0/0 skip=0 lay=0 mix=1924 last=L,C,R,Ls,Rs,LFE",
};

describe("decoder stats", () => {
  it("parses the native map and refuses anything without a codec", () => {
    expect(parseDecoderStats({ ...base })).toEqual(base);
    expect(parseDecoderStats({ frames: 3 })).toBeNull();
    expect(parseDecoderStats(null)).toBeNull();
    // A missing counter reads 0 rather than NaN in the beacon.
    expect(parseDecoderStats({ codec: "ac3" })?.frames).toBe(0);
    // A build before ac3go's own counters sends no summary.
    expect(parseDecoderStats({ codec: "ac3" })?.decoder).toBe("");
  });

  it("converts frames to seconds at the stream's rate", () => {
    expect(framesToSec(1000, 48000)).toBe(32);
    expect(framesToSec(1000, 0)).toBe(0);
  });

  // 10 s of exact zeros at 48 kHz is 313 frames (312.5 rounded up).
  it("alarms on a ten-second silent run, not before", () => {
    expect(isSilentRunAlarming({ ...base, silentRunFrames: 312 })).toBe(false);
    expect(isSilentRunAlarming({ ...base, silentRunFrames: 313 })).toBe(true);
    expect(isSilentRunAlarming({ ...base, silentRunFrames: 313, sampleRate: 0 })).toBe(false);
  });

  it("formats one short line for the beacon", () => {
    const line = formatDecoderStats({ ...base, silentFrames: 1924, silentRunFrames: 1924 });
    expect(line).toBe(
      "codec=eac3 src=6ch out=2ch max=2ch rate=48000 frames=1924 bad=0 silent=1924 run=1924 " +
        "err=0/0/0/0/0/0/0 skip=0 lay=0 mix=1924 last=L,C,R,Ls,Rs,LFE",
    );
    expect(formatDecoderStats({ ...base, decoder: "" })).not.toContain("err=");
    expect(line.length).toBeLessThan(200);
  });
});
