// Pure fMP4 -> ADTS extraction for the bi-source audio relay: Chrome's
// decodeAudioData refuses fragmented MP4 outright (proven 2026-07-29 on the
// exact bytes ffmpeg's hls muxer emits), but eats a raw ADTS AAC stream. So
// the native leg demuxes the AAC frames out of each fMP4 media segment
// (moof/trun -> sample sizes, mdat -> payload) and wraps each frame in a
// 7-byte ADTS header built from the init segment's AudioSpecificConfig.
//
// The shape handled is exactly what our audio rendition pipeline produces
// (ffmpeg hls_segment_type=fmp4: one track, one trun per traf) - but the
// walkers iterate every moof/mdat pair and every trun so a multi-fragment
// segment stays correct. Extraction validated against ffmpeg's own decoder
// (frame-exact duration, zero errors) in the 2026-07-29 harness.

export interface AacConfig {
  objectType: number; // AudioSpecificConfig audioObjectType (2 = AAC-LC)
  samplingIdx: number; // sampling_frequency_index
  channelCfg: number; // channel_configuration
}

interface Box {
  type: string;
  start: number;
  hdrSize: number;
  size: number;
}

function* boxes(u8: Uint8Array, start: number, end: number): Generator<Box> {
  let off = start;
  while (off + 8 <= end) {
    const dv = new DataView(u8.buffer, u8.byteOffset + off);
    let size = dv.getUint32(0);
    const type = String.fromCharCode(u8[off + 4], u8[off + 5], u8[off + 6], u8[off + 7]);
    let hdr = 8;
    if (size === 1) {
      size = Number(dv.getBigUint64(8));
      hdr = 16;
    } else if (size === 0) {
      size = end - off;
    }
    if (size < hdr || off + size > end) return; // malformed: stop rather than loop
    yield { type, start: off, hdrSize: hdr, size };
    off += size;
  }
}

function findBox(u8: Uint8Array, path: string[], start: number, end: number): Box | null {
  const [head, ...rest] = path;
  for (const b of boxes(u8, start, end)) {
    if (b.type !== head) continue;
    if (rest.length === 0) return b;
    return findBox(u8, rest, b.start + b.hdrSize, b.start + b.size);
  }
  return null;
}

// parseAacConfig reads the AudioSpecificConfig out of the init segment's
// moov/trak/mdia/minf/stbl/stsd/mp4a/esds. Null when the structure is not the
// expected AAC shape - the caller treats that as "relay unavailable", never a
// throw on hostile bytes.
export function parseAacConfig(init: Uint8Array): AacConfig | null {
  const stsd = findBox(init, ["moov", "trak", "mdia", "minf", "stbl", "stsd"], 0, init.length);
  if (!stsd) return null;
  const entriesStart = stsd.start + stsd.hdrSize + 8; // version/flags + entry_count
  const mp4a = [...boxes(init, entriesStart, stsd.start + stsd.size)].find((b) => b.type === "mp4a");
  if (!mp4a) return null;
  // mp4a sample entry: 8-byte box header + 28 bytes of AudioSampleEntry fields.
  const esds = [...boxes(init, mp4a.start + 8 + 28, mp4a.start + mp4a.size)].find((b) => b.type === "esds");
  if (!esds) return null;
  // esds: 4 bytes version/flags then an ES_Descriptor tag soup. Rather than
  // model every descriptor, scan for the DecoderSpecificInfo tag (0x05) and
  // read its expandable length - the ASC is tiny and unambiguous in practice.
  const body = init.subarray(esds.start + esds.hdrSize + 4, esds.start + esds.size);
  for (let i = 0; i < body.length - 1; i++) {
    if (body[i] !== 0x05) continue;
    let len = 0;
    let j = i + 1;
    for (; j < body.length; j++) {
      len = (len << 7) | (body[j] & 0x7f);
      if ((body[j] & 0x80) === 0) {
        j++;
        break;
      }
    }
    if (len >= 2 && j + len <= body.length) {
      const asc = body.subarray(j, j + len);
      const objectType = asc[0] >> 3;
      const samplingIdx = ((asc[0] & 0x07) << 1) | (asc[1] >> 7);
      const channelCfg = (asc[1] >> 3) & 0x0f;
      if (objectType === 0 || samplingIdx > 12 || channelCfg === 0 || channelCfg > 7) return null;
      return { objectType, samplingIdx, channelCfg };
    }
  }
  return null;
}

// parseTrackTimescale reads the media timescale (ticks per second) of the init
// segment's first track, moov/trak/mdia/mdhd. Null on any other shape.
export function parseTrackTimescale(init: Uint8Array): number | null {
  const mdhd = findBox(init, ["moov", "trak", "mdia", "mdhd"], 0, init.length);
  if (!mdhd) return null;
  const body = mdhd.start + mdhd.hdrSize;
  const version = init[body];
  // version 1: 8-byte creation and modification times before the timescale.
  const at = body + (version === 1 ? 20 : 12);
  if (at + 4 > mdhd.start + mdhd.size) return null;
  const ts = new DataView(init.buffer, init.byteOffset + at, 4).getUint32(0);
  return ts > 0 ? ts : null;
}

// parseSegmentDecodeTime reads the first moof/traf/tfdt of an fMP4 media
// segment: when its first sample starts, in timescale ticks. Null without one.
export function parseSegmentDecodeTime(seg: Uint8Array): number | null {
  const tfdt = findBox(seg, ["moof", "traf", "tfdt"], 0, seg.length);
  if (!tfdt) return null;
  const body = tfdt.start + tfdt.hdrSize;
  const version = seg[body];
  const len = version === 1 ? 8 : 4;
  if (body + 4 + len > tfdt.start + tfdt.size) return null;
  const dv = new DataView(seg.buffer, seg.byteOffset + body + 4, len);
  return version === 1 ? Number(dv.getBigUint64(0)) : dv.getUint32(0);
}

// extractAacFrames returns the raw AAC access units of an fMP4 media segment,
// one Uint8Array per frame, walking every moof/mdat pair and every trun.
// Sample sizes come from each trun (or the tfhd default); payload is read
// sequentially from the pair's mdat, which is how ffmpeg lays segments out.
export function extractAacFrames(seg: Uint8Array): Uint8Array[] {
  const frames: Uint8Array[] = [];
  let pendingSizes: number[] = [];
  for (const top of boxes(seg, 0, seg.length)) {
    if (top.type === "moof") {
      pendingSizes = [];
      for (const traf of boxes(seg, top.start + top.hdrSize, top.start + top.size)) {
        if (traf.type !== "traf") continue;
        let defaultSize = 0;
        const tfhd = findBox(seg, ["tfhd"], traf.start + traf.hdrSize, traf.start + traf.size);
        if (tfhd) {
          // DataView bounded to the BOX, not the whole segment buffer: a box
          // whose declared size undercounts its fields must fail its own read
          // (RangeError, caught by the decode path) instead of silently
          // reading the neighbouring box's bytes as field values.
          const dv = new DataView(seg.buffer, seg.byteOffset + tfhd.start + tfhd.hdrSize, tfhd.size - tfhd.hdrSize);
          const flags = dv.getUint32(0) & 0xffffff;
          let off = 8; // version/flags + track_ID
          if (flags & 0x01) off += 8;
          if (flags & 0x02) off += 4;
          if (flags & 0x08) off += 4;
          if ((flags & 0x10) && off + 4 <= dv.byteLength) defaultSize = dv.getUint32(off);
        }
        for (const trun of boxes(seg, traf.start + traf.hdrSize, traf.start + traf.size)) {
          if (trun.type !== "trun") continue;
          const dv = new DataView(seg.buffer, seg.byteOffset + trun.start + trun.hdrSize, trun.size - trun.hdrSize);
          if (dv.byteLength < 8) continue;
          const flags = dv.getUint32(0) & 0xffffff;
          const count = dv.getUint32(4);
          // ~141 AAC frames in a 3s segment; a count in the millions is a lie
          // (and would balloon pendingSizes on the no-per-sample-fields shape).
          if (count > 100_000) continue;
          let p = 8;
          if (flags & 0x01) p += 4; // data-offset
          if (flags & 0x04) p += 4; // first-sample-flags
          for (let i = 0; i < count; i++) {
            if (flags & 0x100) p += 4; // duration
            if (flags & 0x200) {
              // A count that overruns the box's actual bytes is malformed:
              // stop at the box edge rather than trusting it.
              if (p + 4 > dv.byteLength) break;
              pendingSizes.push(dv.getUint32(p));
              p += 4;
            } else {
              pendingSizes.push(defaultSize);
            }
            if (flags & 0x400) p += 4; // flags
            if (flags & 0x800) p += 4; // cts offset
          }
        }
      }
    } else if (top.type === "mdat" && pendingSizes.length > 0) {
      let off = top.start + top.hdrSize;
      const end = top.start + top.size;
      for (const size of pendingSizes) {
        if (size <= 0 || off + size > end) break; // malformed: keep what we have
        frames.push(seg.subarray(off, off + size));
        off += size;
      }
      pendingSizes = [];
    }
  }
  return frames;
}

// adtsFromFrames wraps AAC access units in ADTS headers (MPEG-4, no CRC),
// producing the byte stream decodeAudioData accepts.
export function adtsFromFrames(frames: Uint8Array[], cfg: AacConfig): Uint8Array {
  const total = frames.reduce((n, f) => n + f.length + 7, 0);
  const out = new Uint8Array(total);
  let w = 0;
  for (const frame of frames) {
    const frameLen = frame.length + 7;
    out[w] = 0xff;
    out[w + 1] = 0xf1;
    out[w + 2] = ((cfg.objectType - 1) << 6) | (cfg.samplingIdx << 2) | (cfg.channelCfg >> 2);
    out[w + 3] = ((cfg.channelCfg & 3) << 6) | ((frameLen >> 11) & 0x03);
    out[w + 4] = (frameLen >> 3) & 0xff;
    out[w + 5] = ((frameLen & 7) << 5) | 0x1f;
    out[w + 6] = 0xfc;
    out.set(frame, w + 7);
    w += frameLen;
  }
  return out;
}

// AAC-LC frames carry 1024 PCM samples each. Used to convert a preroll frame
// count into the decoded samples to drop.
export const AAC_SAMPLES_PER_FRAME = 1024;
