import { useEffect, useRef } from "react";
import { AppState } from "react-native";
import type { PlayerEventReporter } from "@evey/api";
import {
  createDecoderWatchdog,
  disablePlaybackDecoder,
  formatDecoderStats,
  framesToSec,
  getPlaybackDecoderStats,
  isSilentRunAlarming,
  markPlaybackDecoderIdle,
  resetPlaybackDecoderStats,
  setPlaybackDecoderMedia,
  type DecoderStats,
} from "@evey/media-caps";

const POLL_MS = 2_000;
/** Seconds of decoded audio after which the first stats beacon goes out. */
const FIRST_REPORT_SEC = 10;
/** Then one every minute: the run's trend, and its last state if the app dies. */
const REPORT_EVERY_MS = 60_000;

export interface DecoderTelemetryOptions {
  /**
   * ac3go cannot keep up on this device: it is already dropped from the
   * declaration, the player restarts the stream where it is so the server
   * transcodes the audio.
   */
  onTooSlow?: (reason: string) => void;
  readStats?: () => Promise<DecoderStats | null>;
  resetStats?: () => Promise<void>;
  /** Drops ac3go natively and from the declaration. */
  disable?: (reason: string) => void;
}

const disableDecoder = (reason: string) => {
  void disablePlaybackDecoder(reason);
  // Lazy: deviceCodecs probes the device as it loads, which a screen that
  // merely mounts this hook (and its tests) should not trigger.
  void import("./deviceCodecs").then((m) => m.dropPlayerAudioCodecs());
};

/**
 * Reports what the app's own AC-3/E-AC-3 decoder (ac3go) handed the speakers:
 * a stats beacon once ten seconds of audio are in and then every minute, an
 * error beacon when the output stays pure silence, and a last stats beacon
 * when the player closes.
 *
 * Without it a decoder that loses the audio without failing is invisible: the
 * picture plays, progress ticks, the server logs nothing (2026-09-27, Dark
 * Matter on two phones). Nothing is sent when ac3go never took a track.
 *
 * It also watches ac3go keep up: on a CPU too slow for it the audio clock
 * falls behind, the picture crawls, then the app stops (2026-10-01, a
 * Chromecast HD). Once sure, it drops ac3go and hands over to onTooSlow.
 */
export function useDecoderTelemetry(
  mediaId: string,
  report: PlayerEventReporter,
  positionSec: () => number,
  options: DecoderTelemetryOptions = {},
): void {
  const reportRef = useRef(report);
  reportRef.current = report;
  const posRef = useRef(positionSec);
  posRef.current = positionSec;
  const optsRef = useRef(options);
  optsRef.current = options;
  const readStats = options.readStats ?? getPlaybackDecoderStats;
  const resetStats = options.resetStats ?? resetPlaybackDecoderStats;

  useEffect(() => {
    if (!mediaId) return;
    let firstSent = false;
    let lastSentAt = 0;
    let alarmSent = false;
    let slowSent = false;
    let last: DecoderStats | null = null;
    let cancelled = false;
    const watchdog = createDecoderWatchdog();
    // The decoder of this playback is created once the audio format is known,
    // well after mount: the previous title's counters are dropped first.
    void resetStats();
    void setPlaybackDecoderMedia(mediaId);
    // This title's reporter: the closing beacon fires after the next title's
    // render has already swapped reportRef.
    const reportTitle = reportRef.current;

    const send = (type: string, stats: DecoderStats, fatal = false) =>
      reportTitle({ type, details: formatDecoderStats(stats), fatal, posSec: Math.round(posRef.current()) });

    const poll = async () => {
      const stats = await readStats();
      if (cancelled || !stats || stats.frames === 0) return;
      last = stats;
      const now = Date.now();
      if (!firstSent && framesToSec(stats.frames, stats.sampleRate) >= FIRST_REPORT_SEC) {
        firstSent = true;
        lastSentAt = now;
        send("ac3go_stats", stats);
      } else if (firstSent && now - lastSentAt >= REPORT_EVERY_MS) {
        lastSentAt = now;
        send("ac3go_stats", stats);
      }
      // Once per playback: the silence is the finding, repeats add nothing.
      if (!alarmSent && isSilentRunAlarming(stats)) {
        alarmSent = true;
        send("ac3go_silence_error", stats);
      }
      const slow = slowSent ? null : watchdog(stats);
      if (slow) {
        slowSent = true;
        reportTitle({
          type: "ac3go_too_slow_error",
          details: `${slow} ${formatDecoderStats(stats)}`,
          fatal: false,
          posSec: Math.round(posRef.current()),
        });
        (optsRef.current.disable ?? disableDecoder)(slow);
        optsRef.current.onTooSlow?.(slow);
      }
    };

    // Leaving the screen is not a crash: the OS may kill a background app.
    const sub = AppState.addEventListener("change", (state) => {
      if (state !== "active") void markPlaybackDecoderIdle();
    });

    const timer = setInterval(() => void poll(), POLL_MS);
    return () => {
      cancelled = true;
      clearInterval(timer);
      sub.remove();
      // The run as a whole, read once more: the last poll can be two seconds old.
      void readStats().then((stats) => {
        const final = stats && stats.frames > 0 ? stats : last;
        if (final) send("ac3go_stats_end", final);
      });
    };
  }, [mediaId, readStats, resetStats]);
}
