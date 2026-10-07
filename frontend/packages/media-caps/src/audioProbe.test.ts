import { describe, it, expect } from "vitest";
import { formatAudioProbe, MAX_DETAIL_LEN, type AudioProbe } from "./audioProbe";

function probe(overrides: Partial<AudioProbe> = {}): AudioProbe {
  return {
    apiLevel: 34,
    isTv: true,
    deviceModel: "TV Model A",
    decoders: ["aac", "mp3", "flac"],
    directPlayback: { ac3: false, eac3: false, dts: false },
    directProfiles: [],
    hdmi: { available: true, plugState: 1, encodings: [], maxChannelCount: 2 },
    surroundOutputRaw: 1,
    outputDeviceTypes: [9],
    ...overrides,
  };
}

describe("formatAudioProbe", () => {
  it("reports a box that declares Dolby through HDMI passthrough", () => {
    const out = formatAudioProbe(probe({
      directPlayback: { ac3: true, eac3: true, eac3_joc: false, dts: false },
      hdmi: { available: true, plugState: 1, encodings: [5, 6], maxChannelCount: 8 },
    }));
    expect(out).toContain("dp=ac3+eac3");
    expect(out).toContain("enc=5+6");
    expect(out).toContain("ch=8");
  });

  // The whole point of the beacon: a box with NO Dolby decoder that still
  // passes the bitstream through must not read the same as one that can't.
  it("distinguishes no-decoder-but-passthrough from no-capability-at-all", () => {
    const passthrough = formatAudioProbe(probe({
      decoders: ["aac", "mp3"],
      directPlayback: { ac3: true, eac3: true },
    }));
    const nothing = formatAudioProbe(probe({
      decoders: ["aac", "mp3"],
      directPlayback: { ac3: false, eac3: false },
    }));
    expect(passthrough).toContain("dp=ac3+eac3");
    expect(passthrough).toContain("dec=none");
    expect(nothing).toContain("dp=none");
    expect(nothing).toContain("dec=none");
    expect(passthrough).not.toEqual(nothing);
  });

  // API 33+ reports profiles instead of the deprecated boolean probe; a box on
  // API 29..32 has none, and that absence must read differently from "asked and
  // got nothing back".
  it("renders the API 33+ direct profiles, and says none below that", () => {
    const modern = formatAudioProbe(probe({
      directProfiles: [{ encoding: 5, channelMasks: [252] }, { encoding: 6, channelMasks: [252] }],
    }));
    expect(modern).toContain("prof=5+6");
    expect(formatAudioProbe(probe({ directProfiles: [] }))).toContain("prof=none");
  });

  it("keeps the surround setting, which is what tells a user choice from our bug", () => {
    expect(formatAudioProbe(probe({ surroundOutputRaw: 0 }))).toContain("sur=0");
    expect(formatAudioProbe(probe({ surroundOutputRaw: 3 }))).toContain("sur=3");
  });

  it("marks HDMI as unavailable rather than pretending nothing is plugged in", () => {
    const out = formatAudioProbe(probe({ hdmi: { available: false } }));
    expect(out).toContain("plug=na");
    expect(out).toContain("enc=none");
  });

  it("reports only the codecs the question is about, not aac/mp3/flac noise", () => {
    const out = formatAudioProbe(probe({ decoders: ["aac", "mp3", "flac", "ac3", "eac3"] }));
    expect(out).toContain("dec=ac3+eac3");
    expect(out).not.toContain("aac");
  });

  // The server truncates `details` at 200 chars silently, so an overflowing
  // probe would lose whichever field came last instead of erroring.
  it("never exceeds the server's detail clamp, even on an absurd device", () => {
    const out = formatAudioProbe(probe({
      deviceModel: "M".repeat(120),
      decoders: ["ac3", "eac3", "dts", "truehd"],
      directPlayback: {
        ac3: true, eac3: true, eac3_joc: true, dts: true, dts_hd: true, truehd: true, ac4: true,
      },
      hdmi: {
        available: true,
        plugState: 1,
        encodings: Array.from({ length: 40 }, (_, i) => i),
        maxChannelCount: 8,
      },
      outputDeviceTypes: Array.from({ length: 30 }, (_, i) => i),
      directProfiles: Array.from({ length: 20 }, (_, i) => ({ encoding: i, channelMasks: [252] })),
    }));
    expect(out.length).toBeLessThanOrEqual(MAX_DETAIL_LEN);
    // Dropping a field must not leave a dangling separator or half a token.
    expect(out).not.toMatch(/\s{2,}|=$|\+$/);
    // The highest-value field survives the squeeze.
    expect(out).toContain("dp=");
  });
});
