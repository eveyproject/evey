import type { V1ActiveSession } from "./generated/admin_session.schemas";

// What actually serves each track of a playback session, for the admin
// session cards. The server records videoEngine / videoBackend / audioEngine
// when it renders the master; a session that predates that (or a direct play)
// falls back to the old guess: copy is the packager, transcode is ffmpeg.
// Shared by the web and the phone so the two cards never disagree.
export interface TrackLine {
  id: "video" | "audio";
  decision: string; // copy | transcode | wasm
  engine: string; // mkvgo | ffmpeg | client
  backend: string; // transcode only: software | qsv | vaapi | nvenc | rkmpp
  codec?: string;
  res?: string;
}

export function sessionTrackLines(session: V1ActiveSession): TrackLine[] {
  const vDec = (session.videoDecision ?? "").trim();
  const aDec = (session.audioDecision ?? "").trim();
  const guess = (decision: string) => (decision === "transcode" ? "ffmpeg" : "mkvgo");
  const lines: TrackLine[] = [];
  if (vDec) {
    lines.push({
      id: "video",
      decision: vDec,
      engine: session.videoEngine || guess(vDec),
      backend: vDec === "transcode" ? (session.videoBackend ?? "") : "",
      codec: session.videoCodec,
      res: session.resolution,
    });
  }
  if (aDec) {
    lines.push({
      id: "audio",
      decision: aDec,
      engine: session.audioEngine || (aDec === "wasm" ? "client" : guess(aDec)),
      backend: "",
      codec: session.audioCodec,
    });
  }
  return lines;
}
