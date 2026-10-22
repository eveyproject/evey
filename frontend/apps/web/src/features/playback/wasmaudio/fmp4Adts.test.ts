import { describe, it, expect } from "vitest";
import { adtsFromFrames, extractAacFrames, parseAacConfig, parseSegmentDecodeTime, parseTrackTimescale, type AacConfig } from "./fmp4Adts";

// Box builders: minimal but structurally exact MP4, the same shapes ffmpeg's
// hls muxer emits (validated against real ffmpeg output in the 2026-07-29
// harness - extraction there decoded frame-exact under ffmpeg itself).
function box(type: string, ...payload: Uint8Array[]): Uint8Array {
  const size = 8 + payload.reduce((n, p) => n + p.length, 0);
  const out = new Uint8Array(size);
  new DataView(out.buffer).setUint32(0, size);
  out[4] = type.charCodeAt(0);
  out[5] = type.charCodeAt(1);
  out[6] = type.charCodeAt(2);
  out[7] = type.charCodeAt(3);
  let off = 8;
  for (const p of payload) {
    out.set(p, off);
    off += p.length;
  }
  return out;
}
const u32 = (n: number): Uint8Array => {
  const b = new Uint8Array(4);
  new DataView(b.buffer).setUint32(0, n);
  return b;
};
const bytes = (...vals: number[]): Uint8Array => new Uint8Array(vals);

// esds carrying an AudioSpecificConfig: AAC-LC (2), 44100 (idx 4), stereo (2).
// Tag soup mimics ffmpeg: ES_Descriptor(0x03) > DecoderConfig(0x04) >
// DecoderSpecificInfo(0x05, the 2-byte ASC).
function esdsBox(asc: Uint8Array): Uint8Array {
  const dsi = bytes(0x05, asc.length, ...asc);
  const dcd = bytes(0x04, 13 + dsi.length, 0x40, 0x15, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, ...dsi);
  const esd = bytes(0x03, 3 + dcd.length, 0, 0, 0, ...dcd);
  return box("esds", u32(0), esd);
}

function initSegment(asc: Uint8Array): Uint8Array {
  // mp4a sample entry: 28 bytes of AudioSampleEntry fields then child boxes.
  const mp4a = box("mp4a", new Uint8Array(28), esdsBox(asc));
  const stsd = box("stsd", u32(0), u32(1), mp4a);
  const stbl = box("stbl", stsd);
  const minf = box("minf", stbl);
  const mdia = box("mdia", minf);
  const trak = box("trak", mdia);
  const moov = box("moov", trak);
  return new Uint8Array([...box("ftyp", u32(0)), ...moov]);
}

// A media fragment: trun with explicit sizes (flags 0x200) or tfhd default.
function fragment(frames: Uint8Array[], useDefaultSize = false): Uint8Array {
  const tfhdFlags = useDefaultSize ? 0x10 : 0;
  const tfhd = box(
    "tfhd",
    u32(tfhdFlags),
    u32(1), // track_ID
    ...(useDefaultSize ? [u32(frames[0].length)] : []),
  );
  const trunFlags = useDefaultSize ? 0x01 : 0x201; // data-offset (+ sizes)
  const trun = box(
    "trun",
    u32(trunFlags),
    u32(frames.length),
    u32(0), // data-offset (unused by the extractor: mdat is read sequentially)
    ...(useDefaultSize ? [] : frames.map((f) => u32(f.length))),
  );
  const traf = box("traf", tfhd, trun);
  const moof = box("moof", box("mfhd", u32(0), u32(1)), traf);
  const mdat = box("mdat", ...frames);
  return new Uint8Array([...moof, ...mdat]);
}

const ASC_LC_44100_STEREO = bytes(0x12, 0x10); // objectType 2, samplingIdx 4, channels 2

describe("parseAacConfig", () => {
  it("reads the AudioSpecificConfig out of a real-shaped init", () => {
    const cfg = parseAacConfig(initSegment(ASC_LC_44100_STEREO));
    expect(cfg).toEqual({ objectType: 2, samplingIdx: 4, channelCfg: 2 });
  });

  it("returns null on a non-AAC or malformed init", () => {
    expect(parseAacConfig(new Uint8Array([0, 0, 0, 8, 0x66, 0x74, 0x79, 0x70]))).toBeNull();
    expect(parseAacConfig(new Uint8Array(0))).toBeNull();
    // Sampling index out of range must be rejected, not trusted.
    expect(parseAacConfig(initSegment(bytes(0x17, 0x90)))).toBeNull();
  });
});

describe("extractAacFrames", () => {
  const f1 = bytes(1, 2, 3, 4);
  const f2 = bytes(5, 6, 7);
  const f3 = bytes(8, 9);

  it("splits the mdat by the trun's explicit sample sizes", () => {
    const frames = extractAacFrames(fragment([f1, f2, f3]));
    expect(frames.map((f) => [...f])).toEqual([[...f1], [...f2], [...f3]]);
  });

  it("falls back to the tfhd default sample size", () => {
    const a = bytes(1, 2, 3);
    const b = bytes(4, 5, 6);
    const frames = extractAacFrames(fragment([a, b], true));
    expect(frames.map((f) => [...f])).toEqual([[...a], [...b]]);
  });

  it("walks multiple moof/mdat pairs in one segment", () => {
    const seg = new Uint8Array([...fragment([f1, f2]), ...fragment([f3])]);
    const frames = extractAacFrames(seg);
    expect(frames.map((f) => [...f])).toEqual([[...f1], [...f2], [...f3]]);
  });

  it("returns nothing (not garbage) on truncated input", () => {
    const whole = fragment([f1, f2]);
    expect(extractAacFrames(whole.subarray(0, 10))).toEqual([]);
  });
});

describe("adtsFromFrames", () => {
  it("wraps each frame in a 7-byte ADTS header with the right config bits", () => {
    const cfg: AacConfig = { objectType: 2, samplingIdx: 4, channelCfg: 2 };
    const frame = bytes(0xaa, 0xbb, 0xcc);
    const out = adtsFromFrames([frame], cfg);
    expect(out.length).toBe(10);
    // Syncword + MPEG-4 + no-CRC.
    expect(out[0]).toBe(0xff);
    expect(out[1]).toBe(0xf1);
    // profile(objectType-1)<<6 | samplingIdx<<2 | channelCfg>>2.
    expect(out[2]).toBe(((2 - 1) << 6) | (4 << 2) | (2 >> 2));
    // frame_length = 10 rides bits across bytes 3-5.
    const frameLen = ((out[3] & 0x03) << 11) | (out[4] << 3) | (out[5] >> 5);
    expect(frameLen).toBe(10);
    expect([...out.subarray(7)]).toEqual([...frame]);
  });

  it("concatenates multiple frames into one stream", () => {
    const cfg: AacConfig = { objectType: 2, samplingIdx: 3, channelCfg: 6 };
    const out = adtsFromFrames([bytes(1), bytes(2, 3)], cfg);
    expect(out.length).toBe(8 + 9);
    expect(out[0]).toBe(0xff);
    expect(out[8]).toBe(0xff); // second header right after the first frame
  });
});

// The two clocks the AC-3 placement reads: the track's timescale from the
// init, the segment's first decode time from its tfdt. Shapes as mkvgo's
// audio renditions carry them (mdhd v0, tfdt v1).
describe("parseTrackTimescale / parseSegmentDecodeTime", () => {
  const mdhd = (version: number, timescale: number): Uint8Array =>
    version === 1
      ? box("mdhd", bytes(1, 0, 0, 0), new Uint8Array(16), u32(timescale), new Uint8Array(12))
      : box("mdhd", u32(0), new Uint8Array(8), u32(timescale), new Uint8Array(8));
  const init = (m: Uint8Array): Uint8Array => box("moov", box("trak", box("mdia", m)));

  it("reads the timescale of a v0 and a v1 mdhd", () => {
    expect(parseTrackTimescale(init(mdhd(0, 48000)))).toBe(48000);
    expect(parseTrackTimescale(init(mdhd(1, 44100)))).toBe(44100);
  });

  it("refuses an init without one", () => {
    expect(parseTrackTimescale(box("moov", box("trak")))).toBeNull();
    expect(parseTrackTimescale(init(mdhd(0, 0)))).toBeNull();
  });

  it("reads the decode time of a v1 and a v0 tfdt", () => {
    const v1 = new Uint8Array(12);
    v1[0] = 1;
    new DataView(v1.buffer).setBigUint64(4, 34306560n); // prod, S02E02 segment 120
    expect(parseSegmentDecodeTime(box("moof", box("traf", box("tfdt", v1))))).toBe(34306560);
    expect(parseSegmentDecodeTime(box("moof", box("traf", box("tfdt", u32(0), u32(288768)))))).toBe(288768);
  });

  it("refuses a segment without a tfdt, or one cut short", () => {
    expect(parseSegmentDecodeTime(box("moof", box("traf")))).toBeNull();
    expect(parseSegmentDecodeTime(box("moof", box("traf", box("tfdt", bytes(1, 0, 0, 0), u32(1)))))).toBeNull();
  });
});
