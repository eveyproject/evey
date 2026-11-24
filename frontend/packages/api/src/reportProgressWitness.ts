import { playbackServiceReportProgress } from "./generated/playback-service/playback-service";
import type { V1ReportProgressRequest } from "./generated/playback.schemas";

// A player's progress tick carries its "played" report (what it decodes).
// A server older than the field refuses the whole request ("unknown field"),
// and a refused tick loses the resume point: the app store build can reach an
// older server. The tick is then sent again without the report, and the
// report is not sent to that server again until the app reloads.
let playedRefused = false;

const UNKNOWN_PLAYED = /unknown field\s+\\?"played\\?"/i;

export async function reportProgressWithWitness(body: V1ReportProgressRequest) {
  const send = playedRefused && body.played ? { ...body, played: undefined } : body;
  try {
    return await playbackServiceReportProgress(send);
  } catch (err) {
    if (!send.played || !UNKNOWN_PLAYED.test(String((err as Error)?.message ?? err))) throw err;
    playedRefused = true;
    return playbackServiceReportProgress({ ...send, played: undefined });
  }
}

// For tests: forget that a server refused the report.
export function resetPlayedRefusal() {
  playedRefused = false;
}
