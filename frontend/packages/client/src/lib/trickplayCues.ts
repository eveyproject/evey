// Trickplay-cue parsing + cue selection, shared by every player that draws
// a seek preview (Android TV, the browser TV client): the sprite-sheet
// WebVTT format (one cue per interval, each carrying a `#xywh=` tile rect) is
// identical server-side for every client, so it is parsed once here.
//
// Kept free of React/fetch on purpose: the TV screen owns the network call
// itself (the response is a raw WebVTT text body, not JSON, so it goes
// through eveyFetchRaw like the web version does) and only needs this module
// for the pure parse + cue-lookup math, which is what's under test here.
export interface TrickplayCue {
  startTime: number; // seconds
  endTime: number; // seconds
  url: string;
  x: number;
  y: number;
  w: number;
  h: number;
}

export function parseTrickplayVTT(text: string): TrickplayCue[] {
  const cues: TrickplayCue[] = [];
  const blocks = text.split("\n\n");
  for (const block of blocks) {
    const lines = block.trim().split("\n");
    for (let i = 0; i < lines.length; i++) {
      const match = lines[i].match(
        /(\d{2}):(\d{2}):(\d{2})\.(\d{3})\s*-->\s*(\d{2}):(\d{2}):(\d{2})\.(\d{3})/,
      );
      if (!match) continue;
      const startTime =
        parseInt(match[1]) * 3600 + parseInt(match[2]) * 60 + parseInt(match[3]) + parseInt(match[4]) / 1000;
      const endTime =
        parseInt(match[5]) * 3600 + parseInt(match[6]) * 60 + parseInt(match[7]) + parseInt(match[8]) / 1000;
      const urlLine = lines[i + 1]?.trim();
      if (!urlLine) continue;
      const xywh = urlLine.match(/#xywh=(\d+),(\d+),(\d+),(\d+)/);
      if (!xywh) continue;
      cues.push({
        startTime,
        endTime,
        url: urlLine.split("#")[0],
        x: parseInt(xywh[1]),
        y: parseInt(xywh[2]),
        w: parseInt(xywh[3]),
        h: parseInt(xywh[4]),
      });
    }
  }
  return cues;
}

// The cue covering timeSec, or null when it falls before the first cue, after
// the last one, or the cue list is empty (no trickplay generated yet) - every
// one of those is a silent "no preview available" to the caller, never an
// error.
export function cueForTime(cues: TrickplayCue[], timeSec: number): TrickplayCue | null {
  return cues.find((c) => timeSec >= c.startTime && timeSec < c.endTime) ?? null;
}
