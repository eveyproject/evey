// WebVTT cue parsing + active-cue lookup for the client-side subtitle
// renderers. Shared by mobile and TV (and available to the web), which is why
// it lives here rather than in one app: the same server route answers the same
// WebVTT for all three, and a second copy of this parser would drift.
//
// Built ON TOP of subtitleOffset.ts's offset math instead of duplicating it -
// that primitive only knows {start,end} pairs, this file adds the "text" a cue
// carries and the lookup a renderer needs.

import { applyCueOffset, type Cue } from "./subtitleOffset";

export interface SubtitleCue extends Cue {
  text: string;
}

// parseVttTimestamp accepts both HH:MM:SS.mmm and the short MM:SS.mmm form -
// every WEBVTT this renderer sees is produced server-side or by
// subtitleParse.ts's srtToVtt/assToVtt, both of which always emit the full
// HH:MM:SS.mmm form, but the short form is valid WebVTT too.
function parseVttTimestamp(raw: string): number | null {
  const m = raw.trim().match(/^(?:(\d+):)?(\d{2}):(\d{2})\.(\d{3})$/);
  if (!m) return null;
  const [, h, mm, ss, ms] = m;
  return (h ? Number(h) * 3600 : 0) + Number(mm) * 60 + Number(ss) + Number(ms) / 1000;
}

// parseVttCues turns raw WebVTT text into a flat, start-sorted cue list. Cue
// identifier lines (a bare line before the timing line) and NOTE blocks are
// skipped implicitly (neither line contains "-->"); cue settings after the
// timing line (e.g. "align:start line:0") are dropped - SubtitleOverlay
// positions cues itself via the user's style preference, not the track's
// authored positioning. Inline markup tags (<b>, <i>, <c.classname>) are
// stripped: React Native's Text has no VTT tag renderer.
export function parseVttCues(vtt: string): SubtitleCue[] {
  const lines = vtt.replace(/\r\n/g, "\n").replace(/\r/g, "\n").split("\n");
  const cues: SubtitleCue[] = [];
  let i = 0;
  while (i < lines.length) {
    const line = lines[i];
    if (line.includes("-->")) {
      const [startRaw, rest] = line.split("-->");
      const endRaw = (rest ?? "").trim().split(/\s+/)[0];
      const start = parseVttTimestamp(startRaw);
      const end = parseVttTimestamp(endRaw);
      i++;
      const textLines: string[] = [];
      while (i < lines.length && lines[i].trim() !== "") {
        textLines.push(lines[i]);
        i++;
      }
      if (start !== null && end !== null) {
        const text = textLines.join("\n").replace(/<[^>]+>/g, "").trim();
        if (text) cues.push({ start, end, text });
      }
      continue;
    }
    i++;
  }
  return cues.sort((a, b) => a.start - b.start);
}

// shiftCues reuses subtitleOffset's applyCueOffset for the start/end math
// (same clamp-at-zero, exactly-reversible semantics already tested there) and
// re-attaches each cue's text - so the resync primitive isn't re-derived here.
export function shiftCues(cues: SubtitleCue[], offsetMs: number): SubtitleCue[] {
  if (offsetMs === 0) return cues;
  const shifted = applyCueOffset(cues, offsetMs / 1000);
  return cues.map((cue, i) => ({ start: shifted[i].start, end: shifted[i].end, text: cue.text }));
}

// findActiveCueIndex binary-searches the rightmost cue whose start <= timeSec
// (cues are sorted by start), then checks it's still within its end - O(log n)
// instead of scanning the whole list on every player timeUpdate tick. Assumes
// non-overlapping cues, true of every subtitle file this renderer sees
// (SRT/ASS/VTT rips are sequential); a track with genuinely overlapping
// multi-speaker cues would only ever surface the later of the two.
export function findActiveCueIndex(cues: SubtitleCue[], timeSec: number): number {
  let lo = 0;
  let hi = cues.length - 1;
  let candidate = -1;
  while (lo <= hi) {
    const mid = (lo + hi) >> 1;
    if (cues[mid].start <= timeSec) {
      candidate = mid;
      lo = mid + 1;
    } else {
      hi = mid - 1;
    }
  }
  if (candidate === -1) return -1;
  return cues[candidate].end >= timeSec ? candidate : -1;
}
