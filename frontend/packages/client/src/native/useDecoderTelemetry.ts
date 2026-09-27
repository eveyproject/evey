import { useEffect, useRef } from "react";
import type { PlayerEventReporter } from "@evey/api";
import {
  formatDecoderStats,
  framesToSec,
  getPlaybackDecoderStats,
  isSilentRunAlarming,
  resetPlaybackDecoderStats,
  type DecoderStats,
} from "@evey/media-caps";

const POLL_MS = 2_000;
/** Seconds of decoded audio after which the first stats beacon goes out. */
const FIRST_REPORT_SEC = 10;

/**
 * Reports what the app's own AC-3/E-AC-3 decoder (ac3go) handed the speakers:
 * a stats beacon once ten seconds of audio are in, an error beacon when the
 * output stays pure silence, and a last stats beacon when the player closes.
 *
 * Without it a decoder that loses the audio without failing is invisible: the
 * picture plays, progress ticks, the server logs nothing (2026-09-27, Dark
 * Matter on two phones). Nothing is sent when ac3go never took a track.
 */
export function useDecoderTelemetry(
  mediaId: string,
  report: PlayerEventReporter,
  positionSec: () => number,
  readStats: () => Promise<DecoderStats | null> = getPlaybackDecoderStats,
  resetStats: () => Promise<void> = resetPlaybackDecoderStats,
): void {
  const reportRef = useRef(report);
  reportRef.current = report;
  const posRef = useRef(positionSec);
  posRef.current = positionSec;

  useEffect(() => {
    if (!mediaId) return;
    let firstSent = false;
    let alarmSent = false;
    let last: DecoderStats | null = null;
    let cancelled = false;
    // The decoder of this playback is created once the audio format is known,
    // well after mount: the previous title's counters are dropped first.
    void resetStats();
    // This title's reporter: the closing beacon fires after the next title's
    // render has already swapped reportRef.
    const reportTitle = reportRef.current;

    const send = (type: string, stats: DecoderStats, fatal = false) =>
      reportTitle({ type, details: formatDecoderStats(stats), fatal, posSec: Math.round(posRef.current()) });

    const poll = async () => {
      const stats = await readStats();
      if (cancelled || !stats || stats.frames === 0) return;
      last = stats;
      if (!firstSent && framesToSec(stats.frames, stats.sampleRate) >= FIRST_REPORT_SEC) {
        firstSent = true;
        send("ac3go_stats", stats);
      }
      // Once per playback: the silence is the finding, repeats add nothing.
      if (!alarmSent && isSilentRunAlarming(stats)) {
        alarmSent = true;
        send("ac3go_silence_error", stats);
      }
    };

    const timer = setInterval(() => void poll(), POLL_MS);
    return () => {
      cancelled = true;
      clearInterval(timer);
      // The run as a whole, read once more: the last poll can be two seconds old.
      void readStats().then((stats) => {
        const final = stats && stats.frames > 0 ? stats : last;
        if (final) send("ac3go_stats_end", final);
      });
    };
  }, [mediaId, readStats, resetStats]);
}
