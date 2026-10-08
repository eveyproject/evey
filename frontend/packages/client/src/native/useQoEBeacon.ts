import { useEffect, useRef } from "react";
import { AppState } from "react-native";
import { eveyFetch } from "@evey/api";
import type { PlayingVariant } from "../lib/servedVariant";

// The native players' half of the QoE table (/admin/qoe, "by device").
//
// A browser's hls.js stamps CMCD (CTA-5004) on every segment request and the
// server files it under the browser's device class. ExoPlayer and AVPlayer
// fetch their segments themselves and attach nothing: AVFoundation has no
// CMCD at all, media3 has it but expo-video never wires it. So until
// 2026-10-08 the table listed browsers only, and a phone or a television that
// stalled on every segment was invisible there.
//
// This hook computes the same keys off the player's own events - buffer ahead
// of the playhead (bl), a rebuffer since the last sample (bs), the startup
// sample (su), the rung being decoded (br, tb) - and posts them as a
// qoe_sample player event every ten seconds while playing, plus one at once
// on a stall. The server parses the string like a CMCD header and the sample
// lands under the app's own User-Agent class (android, ios, tv).

const SAMPLE_EVERY_MS = 10_000;
// A stall is sampled the moment it is seen, but a player flapping between
// loading and ready must not turn into a beacon storm (the route refills at
// 30/min per address, shared with the error beacons).
const STALL_SAMPLE_MIN_GAP_MS = 5_000;

/** CMCD `sf`: "h" HLS, "d" DASH. */
export type QoEStreamFormat = "h" | "d";

// Structural: an expo-video VideoPlayer, without importing expo-video here.
export interface QoEBeaconPlayer {
  // any: expo types it per event name, which no structural signature matches.
  addListener(event: any, listener: any): { remove(): void };
}

export interface QoEBeaconOptions {
  mediaId: string;
  /** The play session id, CMCD `sid`: one per transcode session. */
  sessionId: string | null | undefined;
  /** null: direct play or a local file - no segments, nothing to report. */
  format: QoEStreamFormat | null;
  /** The rung the player decodes now (usePlayingVideoTrack). */
  playing?: PlayingVariant | null;
  /** Every rung's BANDWIDTH, bps: `tb` and the variant index. */
  ladderBitrates?: number[];
}

export interface QoESample {
  bufferMs: number;
  starved: boolean;
  startup: boolean;
  format: QoEStreamFormat;
  sessionId: string | null | undefined;
  /** bps of the playing rung and of the top rung; 0 when unknown. */
  playingBps: number;
  topBps: number;
}

/** The CMCD string of one sample, as the server's cmcd.Parse reads it. */
export function formatQoESample(s: QoESample): string {
  const parts = [`bl=${Math.max(0, Math.round(s.bufferMs))}`, `sf=${s.format}`, "st=v", "ot=v", "v=1"];
  if (s.playingBps > 0) parts.push(`br=${Math.round(s.playingBps / 1000)}`);
  if (s.topBps > 0) parts.push(`tb=${Math.round(s.topBps / 1000)}`);
  if (s.sessionId) parts.push(`sid="${s.sessionId.replace(/["\\]/g, "")}"`);
  if (s.starved) parts.push("bs");
  if (s.startup) parts.push("su");
  return parts.join(",");
}

/** Index of the playing rung on the ladder; 0 when the engine lists none. */
export function variantIndexOf(playing: PlayingVariant | null | undefined, ladderBitrates: number[]): number {
  const bps = playing?.bitrate ?? 0;
  const idx = bps > 0 ? ladderBitrates.indexOf(bps) : -1;
  return idx >= 0 ? idx : 0;
}

export function useQoEBeacon(player: QoEBeaconPlayer | null | undefined, options: QoEBeaconOptions): void {
  const { mediaId, sessionId, format } = options;
  const rungRef = useRef({ playing: options.playing ?? null, ladder: options.ladderBitrates ?? [] });
  rungRef.current = { playing: options.playing ?? null, ladder: options.ladderBitrates ?? [] };

  useEffect(() => {
    if (!player || !mediaId || !format) return;
    let ready = false;
    let isPlaying = false;
    let startup = true;
    let starved = false;
    let bufferMs = 0;
    let posSec = 0;
    let lastSentAt = 0;
    let timer: ReturnType<typeof setInterval> | null = null;

    const send = () => {
      const { playing, ladder } = rungRef.current;
      const topBps = ladder.reduce((m, b) => (b > m ? b : m), 0);
      const details = formatQoESample({ bufferMs, starved, startup, format, sessionId, playingBps: playing?.bitrate ?? 0, topBps });
      startup = false;
      starved = false;
      lastSentAt = Date.now();
      eveyFetch<void>({
        url: `/api/v1/media/${encodeURIComponent(mediaId)}/player-event`,
        method: "POST",
        data: { type: "qoe_sample", details, fatal: false, code: 0, variant: variantIndexOf(playing, ladder), segment: 0, posSec: Math.round(posSec) },
      }).catch(() => {
        // Telemetry never breaks playback: rate-limited or offline, the
        // sample is lost and the next one tells the story.
      });
    };

    const tick = () => {
      // A paused player with a full buffer fetches nothing: no segment, no
      // sample - a browser sends none either. A stall it has not recovered
      // from is still worth a row.
      if (!ready || (!isPlaying && !starved)) return;
      send();
    };
    const start = () => {
      if (timer != null) return;
      timer = setInterval(tick, SAMPLE_EVERY_MS);
    };
    const stop = () => {
      if (timer != null) {
        clearInterval(timer);
        timer = null;
      }
    };

    const timeSub = player.addListener("timeUpdate", (ev: { currentTime: number; bufferedPosition: number }) => {
      posSec = ev.currentTime;
      // -1 until the engine has loaded anything.
      bufferMs = ev.bufferedPosition > 0 ? (ev.bufferedPosition - ev.currentTime) * 1000 : 0;
    });
    const statusSub = player.addListener("statusChange", (ev: { status: string }) => {
      if (ev.status === "readyToPlay") {
        ready = true;
        return;
      }
      // "loading" once playback is under way is a rebuffer, the one thing
      // the table is for: sampled at once, not at the next tick.
      if (ev.status === "loading" && ready && isPlaying) {
        starved = true;
        bufferMs = 0;
        if (Date.now() - lastSentAt >= STALL_SAMPLE_MIN_GAP_MS) send();
      }
    });
    const playSub = player.addListener("playingChange", (ev: { isPlaying: boolean }) => {
      isPlaying = ev.isPlaying;
    });

    // Backgrounded, the app stops playing and stops sampling with it.
    const appSub = AppState.addEventListener("change", (state) => {
      if (state === "active") start();
      else stop();
    });
    start();
    return () => {
      stop();
      appSub.remove();
      timeSub.remove();
      statusSub.remove();
      playSub.remove();
    };
  }, [player, mediaId, sessionId, format]);
}
