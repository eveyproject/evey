import type { SubtitleCue } from "@evey/api";

/**
 * The packager's segmented WebVTT renditions (od_subN.m3u8), read by players
 * that draw subtitles themselves. A segment costs the read of its few seconds
 * of film; /subtitle costs a read of the whole file (minutes on a large one).
 */
export interface SubtitleSegment {
  start: number;
  end: number;
  uri: string;
}

const SUBTITLES_MEDIA = /^#EXT-X-MEDIA:TYPE=SUBTITLES,(.*)$/gm;

function attr(attrs: string, key: string): string | null {
  const m = attrs.match(new RegExp(`(?:^|,)${key}="([^"]*)"`));
  return m ? m[1] : null;
}

/** The rendition URI the master tags with the picker's ordinal (X-EVEY-TRACK), or null. */
export function subtitleRenditionURI(master: string, ordinal: number): string | null {
  for (const m of master.matchAll(SUBTITLES_MEDIA)) {
    if (attr(m[1], "X-EVEY-TRACK") === String(ordinal)) return attr(m[1], "URI");
  }
  return null;
}

/** The segments of a rendition playlist, each with its start and end on the film's timeline. */
export function parseSubtitlePlaylist(playlist: string): SubtitleSegment[] {
  const out: SubtitleSegment[] = [];
  let t = 0;
  let dur: number | null = null;
  for (const raw of playlist.split(/\r?\n/)) {
    const line = raw.trim();
    if (line.startsWith("#EXTINF:")) {
      dur = Number.parseFloat(line.slice("#EXTINF:".length));
    } else if (line && !line.startsWith("#") && dur !== null && Number.isFinite(dur)) {
      out.push({ start: t, end: t + dur, uri: line });
      t += dur;
      dur = null;
    }
  }
  return out;
}

/** Indexes of the segments overlapping [positionSec - behindSec, positionSec + aheadSec]. */
export function segmentWindow(segments: SubtitleSegment[], positionSec: number, aheadSec: number, behindSec = 0): number[] {
  const from = positionSec - behindSec;
  const to = positionSec + aheadSec;
  const out: number[] = [];
  for (let i = 0; i < segments.length; i++) {
    if (segments[i].end > from && segments[i].start <= to) out.push(i);
  }
  return out;
}

// A cue that spans a segment boundary is repeated in both segments.
function cueKey(c: SubtitleCue): string {
  return `${c.start}|${c.end}|${c.text}`;
}

/** The union of two cue lists, start-sorted, without the boundary duplicates. */
export function mergeCues(into: SubtitleCue[], add: SubtitleCue[]): SubtitleCue[] {
  if (add.length === 0) return into;
  const seen = new Set(into.map(cueKey));
  const fresh = add.filter((c) => {
    const k = cueKey(c);
    if (seen.has(k)) return false;
    seen.add(k);
    return true;
  });
  if (fresh.length === 0) return into;
  return [...into, ...fresh].sort((a, b) => a.start - b.start);
}
