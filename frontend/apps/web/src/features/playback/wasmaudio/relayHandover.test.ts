import { describe, it, expect } from "vitest";
import { pcmPlacement, planRelayHandover, type Segment } from "./relayHandover";
import { extractAacAudioURI } from "./wasmAudioShared";

const seg = (start: number, dur: number, extra: Partial<Segment> = {}): Segment => ({
  url: `u${start}`,
  start,
  dur,
  ...extra,
});
const grid = (n: number, dur: number, native = false): Segment[] =>
  Array.from({ length: n }, (_, i) => seg(i * dur, dur, native ? { native: true } : {}));

describe("planRelayHandover", () => {
  it("hands over at the end of the last scheduled native segment on an aligned grid", () => {
    const native = grid(20, 3, true);
    const ac3 = grid(20, 3);
    // Playhead at 4s, native segments 0..3 scheduled (covers up to 12s).
    const plan = planRelayHandover(native, ac3, [0, 1, 2, 3], 4);
    expect(plan).not.toBeNull();
    expect(plan!.keepPrefix).toBe(4);
    // Aligned grids: the first AC-3 segment starts exactly at the boundary, no trim.
    expect(plan!.segments[4]).toMatchObject({ start: 12, dur: 3 });
    expect(plan!.segments[4].trimSec).toBeUndefined();
    expect(plan!.segments[4].native).toBeUndefined();
    // The kept prefix is the native segments, IDENTICAL indices.
    expect(plan!.segments.slice(0, 4)).toEqual(native.slice(0, 4));
  });

  it("trims the AC-3 segment spanning a misaligned boundary", () => {
    const native = grid(20, 3, true); // boundaries at 0,3,6,9,12...
    const ac3 = grid(15, 4); // boundaries at 0,4,8,12,16... but offset content
    const plan = planRelayHandover(native, ac3, [0, 1, 2], 2); // boundary = 9
    expect(plan).not.toBeNull();
    expect(plan!.keepPrefix).toBe(3);
    // AC-3 segment [8,12) spans the 9s boundary: trimmed by 1s.
    expect(plan!.segments[3]).toMatchObject({ start: 8, dur: 4, trimSec: 1 });
    // Following segments ride untrimmed.
    expect(plan!.segments[4]).toMatchObject({ start: 12, dur: 4 });
    expect(plan!.segments[4].trimSec).toBeUndefined();
  });

  it("uses a runway past the playhead when nothing is scheduled yet", () => {
    const native = grid(20, 3, true);
    const ac3 = grid(20, 3);
    const plan = planRelayHandover(native, ac3, [], 0);
    expect(plan).not.toBeNull();
    // Runway is ct+2 -> the native segment [0,3) is kept whole, boundary snaps
    // to its end so the trimmed head can never overlap it.
    expect(plan!.keepPrefix).toBe(1);
    expect(plan!.segments[1]).toMatchObject({ start: 3 });
    expect(plan!.segments[1].trimSec).toBeUndefined();
  });

  it("returns null when the film ends before the boundary", () => {
    const native = grid(4, 3, true); // 12s of media
    const ac3 = grid(4, 3);
    // Everything is already scheduled: boundary = 12 = end of film.
    expect(planRelayHandover(native, ac3, [0, 1, 2, 3], 11)).toBeNull();
  });

  it("keeps a deep-resume window: scheduled indices far into the film", () => {
    const native = grid(1000, 3, true);
    const ac3 = grid(1000, 3);
    const plan = planRelayHandover(native, ac3, [500, 501], 1501);
    expect(plan).not.toBeNull();
    expect(plan!.keepPrefix).toBe(502); // prefix ends at segment 501 (ends 1506s)
    expect(plan!.segments[502]).toMatchObject({ start: 1506 });
  });
});

describe("extractAacAudioURI", () => {
  it("pulls the quoted AAC relay URI and leaves the AC-3 tag alone", () => {
    const master = [
      "#EXTM3U",
      '#EVEY-AUDIO-URI:"od_audio1.m3u8?audioTrack=0&token=abc"',
      '#EVEY-AUDIO-AAC-URI:"audio_0.m3u8?audioTrack=0&token=abc"',
      "stream_0.m3u8",
    ].join("\n");
    expect(extractAacAudioURI(master)).toBe("audio_0.m3u8?audioTrack=0&token=abc");
  });

  it("returns null when the server does not offer the relay", () => {
    const master = '#EXTM3U\n#EVEY-AUDIO-URI:"od_audio1.m3u8"\nstream_0.m3u8';
    expect(extractAacAudioURI(master)).toBeNull();
  });
});

// The decode times of prod segments 110-129 (The Terminal List S02E02, VFF),
// in 32 ms E-AC-3 frames: each one starts on the first frame at or after its
// 6.006 s playlist slot, and holds 188 or 187 frames.
describe("pcmPlacement", () => {
  const FRAME = 1536 / 48000;
  const slots = Array.from({ length: 20 }, (_, k) => ({ url: `s${k}`, start: (109 + k) * 6.006, dur: 6.006 }));
  const starts = slots.map((s) => Math.ceil(Number((s.start / FRAME).toFixed(6))) * FRAME);

  it("plays each segment from its own decode time, not its slot", () => {
    const at = pcmPlacement(slots[10], 188 * FRAME, starts[10]); // segment 120
    expect(slots[10].start * 1000).toBeCloseTo(714714, 3);
    expect(at.startMedia * 1000).toBeCloseTo(714720, 3); // its tfdt, measured on prod
    expect(at.bufferOffset).toBe(0);
    expect(at.playSec).toBeCloseTo(188 * FRAME, 9);
  });

  it("lays consecutive segments end to end, with no repeat and no hole", () => {
    for (let k = 0; k + 1 < slots.length; k++) {
      const frames = Math.round((starts[k + 1] - starts[k]) / FRAME);
      expect([187, 188]).toContain(frames);
      const at = pcmPlacement(slots[k], frames * FRAME, starts[k]);
      expect(at.startMedia + at.playSec).toBeCloseTo(starts[k + 1], 9);
      // The old placement: slot start, the whole buffer - 10 ms twice or a 22 ms hole.
      const oldEnd = slots[k].start + frames * FRAME;
      expect(Math.abs(oldEnd - slots[k + 1].start)).toBeGreaterThan(0.009);
    }
  });

  it("keeps the slot without a decode time, or with one on another timeline", () => {
    expect(pcmPlacement(slots[0], 6.016, undefined)).toEqual({ startMedia: slots[0].start, bufferOffset: 0, playSec: 6.006 });
    expect(pcmPlacement(slots[0], 6.016, slots[0].start + 1.4).startMedia).toBe(slots[0].start);
  });

  it("starts a hand-over segment at the boundary, inside its buffer", () => {
    const s = { ...slots[11], trimSec: 2 };
    const at = pcmPlacement(s, 188 * FRAME, starts[11]);
    expect(at.startMedia).toBeCloseTo(slots[11].start + 2, 9);
    expect(at.bufferOffset).toBeCloseTo(slots[11].start + 2 - starts[11], 9);
    expect(at.startMedia + at.playSec).toBeCloseTo(starts[11] + 188 * FRAME, 9);
  });
});
