/**
 * Parsing the bitmap-subtitle blob the server sends for a PGS track.
 *
 * A bitmap subtitle carries no text, so it cannot become WebVTT. Until now that
 * left exactly one way to show it - the server burning it into the video - and
 * a burn-in denies the video copy rung: one forced French subtitle, the track a
 * retail rip flags DEFAULT, turned a 4K remux that direct-played fine into a
 * full transcode. Drawing the bitmaps in the player is what removes that cost.
 *
 * Wire format (subtitle_pgs.go builds it):
 *
 *   "EVEYPGS1" | uint32be header length | header JSON | PNG bytes, back to back
 *
 * One blob per track rather than one request per cue: a track is around a
 * thousand cues.
 */

const MAGIC = "EVEYPGS1";

/** A cue's placement, in the coordinates of the SUBTITLE plane (see PGSTrack). */
export interface PGSCue {
  startMs: number;
  endMs: number;
  x: number;
  y: number;
  w: number;
  h: number;
  /**
   * True for a cue the disc wants shown whatever the viewer chose - a
   * translated sign, a line in another language. Per cue, not per track: a full
   * track commonly mixes both (one measured disc carries 13 forced cues inside
   * a 2032-cue track), so the track-level flag cannot answer this.
   */
  forced: boolean;
  /** The cue's PNG. Each platform turns it into whatever its <img> takes. */
  png: Uint8Array;
}

export interface PGSTrack {
  /**
   * The plane the cue coordinates refer to, which is NOT necessarily the video
   * resolution: every 2160p disc measured carries a 1920x1080 subtitle plane.
   * Scaling a cue by the video size instead offsets and mis-sizes it by a
   * factor of two, so always scale by this.
   */
  screenW: number;
  screenH: number;
  cues: PGSCue[];
}

/** True when these bytes are a bitmap-subtitle blob rather than WebVTT text. */
export function isPGSBlob(buf: ArrayBuffer | Uint8Array): boolean {
  const b = buf instanceof Uint8Array ? buf : new Uint8Array(buf);
  if (b.length < MAGIC.length) return false;
  for (let i = 0; i < MAGIC.length; i++) {
    if (b[i] !== MAGIC.charCodeAt(i)) return false;
  }
  return true;
}

/**
 * Parse the blob. Throws on anything it cannot verify rather than returning a
 * partial track: the offsets a malformed header names would index into whatever
 * follows it, and a viewer is better served by no subtitle than by another
 * cue's pixels.
 */
export function parsePGSBlob(buf: ArrayBuffer | Uint8Array): PGSTrack {
  const b = buf instanceof Uint8Array ? buf : new Uint8Array(buf);
  if (!isPGSBlob(b)) throw new Error("pgs: bad magic");
  if (b.length < MAGIC.length + 4) throw new Error("pgs: truncated");

  const view = new DataView(b.buffer, b.byteOffset, b.byteLength);
  const headerLen = view.getUint32(MAGIC.length, false);
  const headerStart = MAGIC.length + 4;
  const bodyStart = headerStart + headerLen;
  if (bodyStart > b.length) throw new Error("pgs: header length out of range");

  const json = new TextDecoder().decode(b.subarray(headerStart, bodyStart));
  const raw = JSON.parse(json) as {
    screenW?: number;
    screenH?: number;
    cues?: { s: number; e: number; x: number; y: number; w: number; h: number; o: number; l: number; f?: boolean }[];
  };

  const cues: PGSCue[] = [];
  for (const c of raw.cues ?? []) {
    const from = bodyStart + c.o;
    const to = from + c.l;
    // A cue pointing past the body is the one case where continuing would draw
    // the wrong pixels, so it fails the whole track rather than being skipped.
    if (to > b.length || from < bodyStart) throw new Error("pgs: cue image out of range");
    cues.push({
      startMs: c.s,
      endMs: c.e,
      x: c.x,
      y: c.y,
      w: c.w,
      h: c.h,
      forced: c.f === true,
      png: b.subarray(from, to),
    });
  }
  return { screenW: raw.screenW ?? 0, screenH: raw.screenH ?? 0, cues };
}

/**
 * The cue to show at a given time, or -1.
 *
 * Linear from a hint index, like the WebVTT lookup next door: playback advances
 * monotonically, so the answer is almost always the current cue or the next one.
 */
export function findActivePGSCue(cues: PGSCue[], timeMs: number, hint = 0): number {
  if (cues.length === 0) return -1;
  const start = hint >= 0 && hint < cues.length && cues[hint].startMs <= timeMs ? hint : 0;
  for (let i = start; i < cues.length; i++) {
    if (cues[i].startMs > timeMs) break;
    if (timeMs < cues[i].endMs) return i;
  }
  // Fall back to a full scan: a seek backwards invalidates the hint.
  for (let i = 0; i < cues.length; i++) {
    if (cues[i].startMs <= timeMs && timeMs < cues[i].endMs) return i;
  }
  return -1;
}

const B64 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";

/**
 * A `data:` URI for one cue's PNG.
 *
 * The encoder is written out rather than taken from the platform because the
 * two targets disagree: a browser has btoa, React Native does not reliably, and
 * neither has a Buffer. Twenty lines here beat a polyfill that works in one app
 * and silently renders nothing in the other.
 */
export function pgsCueDataURI(cue: PGSCue): string {
  const b = cue.png;
  let out = "";
  let i = 0;
  for (; i + 2 < b.length; i += 3) {
    const n = (b[i] << 16) | (b[i + 1] << 8) | b[i + 2];
    out += B64[(n >> 18) & 63] + B64[(n >> 12) & 63] + B64[(n >> 6) & 63] + B64[n & 63];
  }
  const rest = b.length - i;
  if (rest === 1) {
    const n = b[i] << 16;
    out += B64[(n >> 18) & 63] + B64[(n >> 12) & 63] + "==";
  } else if (rest === 2) {
    const n = (b[i] << 16) | (b[i + 1] << 8);
    out += B64[(n >> 18) & 63] + B64[(n >> 12) & 63] + B64[(n >> 6) & 63] + "=";
  }
  return `data:image/png;base64,${out}`;
}
