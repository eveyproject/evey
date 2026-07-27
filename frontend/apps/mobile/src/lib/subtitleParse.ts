// Text-subtitle format conversion to WebVTT, ported from the web player's
// PlayerSettingsPanel.tsx (srtToVtt/assToVtt) so both clients accept the same
// upload formats and produce byte-identical cues. Kept format-only (no
// upload/rendering wiring here) - see the mobile player dispatch report for
// why the upload feature stops at parsing until a client subtitle renderer
// exists (expo-video exposes no API to feed an externally-parsed track in).

export function srtToVtt(srt: string): string {
  const vtt = srt
    .replace(/\r\n/g, "\n")
    .replace(/\r/g, "\n")
    // SRT timestamps use comma for ms, VTT uses period
    .replace(/(\d{2}:\d{2}:\d{2}),(\d{3})/g, "$1.$2");
  return `WEBVTT\n\n${vtt.trim()}\n`;
}

// ASS/SSA timestamp is H:MM:SS.cc (centiseconds); VTT wants HH:MM:SS.mmm.
function assTimeToVtt(t: string): string | null {
  const m = t.trim().match(/^(\d+):(\d{2}):(\d{2})\.(\d{2})$/);
  if (!m) return null;
  const [, h, mm, ss, cc] = m;
  return `${h.padStart(2, "0")}:${mm}:${ss}.${cc}0`;
}

// Minimal ASS/SSA -> WebVTT converter: parses the [Events] Format header to
// locate the Start/End/Text columns, then emits one VTT cue per Dialogue
// line, stripping ASS override tags ({\...}) and converting \N hard breaks.
export function assToVtt(ass: string): string {
  const lines = ass.replace(/\r/g, "").split("\n");
  let startIdx = 1, endIdx = 2, textIdx = 9;
  for (const line of lines) {
    if (/^\s*Format\s*:/i.test(line) && /\bText\b/i.test(line)) {
      const cols = line.replace(/^\s*Format\s*:/i, "").split(",").map((c) => c.trim().toLowerCase());
      const si = cols.indexOf("start");
      const ei = cols.indexOf("end");
      const ti = cols.indexOf("text");
      if (si >= 0) startIdx = si;
      if (ei >= 0) endIdx = ei;
      if (ti >= 0) textIdx = ti;
      break;
    }
  }
  const cues: string[] = [];
  for (const line of lines) {
    if (!/^\s*Dialogue\s*:/i.test(line)) continue;
    // Text is the last field and may itself contain commas, so don't over-split.
    const parts = line.replace(/^\s*Dialogue\s*:/i, "").split(",");
    if (parts.length <= textIdx) continue;
    const start = assTimeToVtt(parts[startIdx]);
    const end = assTimeToVtt(parts[endIdx]);
    if (!start || !end) continue;
    const text = parts.slice(textIdx).join(",")
      .replace(/\{[^}]*\}/g, "") // drawing/override tags
      .replace(/\\N/gi, "\n")     // hard line break
      .replace(/\\h/gi, " ")      // hard space
      .trim();
    if (!text) continue;
    cues.push(`${start} --> ${end}\n${text}`);
  }
  return `WEBVTT\n\n${cues.join("\n\n")}\n`;
}

// externalSubtitleKind maps a picked file's extension to the conversion it
// needs before it can be rendered - null for an unsupported extension.
export type ExternalSubtitleKind = "srt" | "ass" | "vtt";

export function externalSubtitleKind(fileName: string): ExternalSubtitleKind | null {
  const ext = fileName.split(".").pop()?.toLowerCase() ?? "";
  if (ext === "srt") return "srt";
  if (ext === "ass" || ext === "ssa") return "ass";
  if (ext === "vtt") return "vtt";
  return null;
}

// toVtt converts raw external subtitle text to WebVTT given its detected
// kind. "vtt" passes through unchanged (already in the target format).
export function toVtt(text: string, kind: ExternalSubtitleKind): string {
  if (kind === "srt") return srtToVtt(text);
  if (kind === "ass") return assToVtt(text);
  return text;
}
