import { framesToSec, type DecoderStats } from "./decoderStats";

/**
 * The safety net around ac3go, the app's own AC-3/E-AC-3 decoder. It runs on
 * the CPU, and a box too slow for it does not fail cleanly: the audio clock
 * falls behind, the picture crawls, then the app stops (2026-10-01, a
 * Chromecast HD). Every doubt routes to the server's fallback instead - video
 * copied, audio transcoded to AAC - which is how such a box played before.
 * The verdicts live natively (Ac3goGuard.kt), so they hold across launches.
 */

/** What the launch check found, null when the module is absent. */
export interface DecoderCheck {
  /** The codecs to declare: [] when ac3go is not trusted on this device. */
  decoders: string[];
  /** Why not ("bench", "suspect:exit:crash_native", "watchdog:..."), "" when trusted. */
  blockedBy: string;
  /** The launch benchmark: rtf (median), rtfCold, rtfMax, maxChannels, wallMs, slow. */
  bench: Record<string, unknown> | null;
  /** The previous run, when it died while ac3go was decoding. */
  abnormalExit: Record<string, unknown> | null;
}

interface GuardNative {
  checkPlaybackDecoder?(): Promise<unknown>;
  disablePlaybackDecoder?(reason: string): void;
  markPlaybackDecoderIdle?(): void;
  setPlaybackDecoderMedia?(mediaId: string): void;
}

async function guardNative(): Promise<GuardNative | null> {
  // Lazy, like every native call of this package: expo-modules-core reads
  // globalThis.expo at import time.
  const { requireOptionalNativeModule } = await import("expo-modules-core");
  return requireOptionalNativeModule<GuardNative>("EveyOfflineAudio");
}

const asRecord = (v: unknown): Record<string, unknown> | null =>
  v && typeof v === "object" ? (v as Record<string, unknown>) : null;

/** Test seam for the parsing of the native map. */
export function parseDecoderCheck(raw: unknown): DecoderCheck | null {
  const r = asRecord(raw);
  if (!r || !Array.isArray(r.decoders)) return null;
  return {
    decoders: r.decoders.filter((c): c is string => typeof c === "string"),
    blockedBy: typeof r.blockedBy === "string" ? r.blockedBy : "",
    bench: asRecord(r.bench),
    abnormalExit: asRecord(r.abnormalExit),
  };
}

/**
 * Benchmarks ac3go once per launch (about a second on a slow box, off the JS
 * thread) and says whether to declare it. Null on a build without the check:
 * the caller then falls back to the plain decoder list. Never throws.
 */
export async function checkPlaybackDecoder(): Promise<DecoderCheck | null> {
  try {
    const native = await guardNative();
    if (!native?.checkPlaybackDecoder) return null;
    return parseDecoderCheck(await native.checkPlaybackDecoder());
  } catch {
    return null;
  }
}

/** The watchdog saw ac3go fall behind: not declared again on this device for a while. Never throws. */
export async function disablePlaybackDecoder(reason: string): Promise<void> {
  try {
    (await guardNative())?.disablePlaybackDecoder?.(reason);
  } catch {
    /* module absent */
  }
}

/** The app left the screen: an OS kill from now on is not ac3go's doing. Never throws. */
export async function markPlaybackDecoderIdle(): Promise<void> {
  try {
    (await guardNative())?.markPlaybackDecoderIdle?.();
  } catch {
    /* module absent */
  }
}

/** Names the title in the crash marker, so the next launch's report says what was playing. Never throws. */
export async function setPlaybackDecoderMedia(mediaId: string): Promise<void> {
  try {
    (await guardNative())?.setPlaybackDecoderMedia?.(mediaId);
  } catch {
    /* module absent */
  }
}

const num = (v: unknown, digits: number) => (typeof v === "number" ? v.toFixed(digits) : "?");
const str = (v: unknown) => (typeof v === "string" || typeof v === "number" || typeof v === "boolean" ? String(v) : "?");

/** One beacon line for the launch check. */
export function formatDecoderCheck(check: DecoderCheck): string {
  const b = check.bench;
  return [
    `declared=${check.decoders.join(",") || "none"}`,
    `blocked=${check.blockedBy || "no"}`,
    ...(b
      ? b.error
        ? [`bench_error=${str(b.error)}`]
        : [`rtf=${num(b.rtf, 3)}`, `cold=${num(b.rtfCold, 3)}`, `max=${num(b.rtfMax, 3)}`, `ch=${str(b.maxChannels)}`, `wall=${str(b.wallMs)}ms`]
      : ["bench=none"]),
  ].join(" ");
}

/** One beacon line for a run that died while ac3go was decoding. */
export function formatAbnormalExit(exit: Record<string, unknown>): string {
  const at = typeof exit.since === "number" ? new Date(exit.since).toISOString() : "?";
  return [
    `kind=${str(exit.kind)}`,
    `reason=${str(exit.reason ?? "unknown")}`,
    `status=${str(exit.status ?? "?")}`,
    `suspect=${str(exit.suspect)}`,
    `since=${at}`,
    `version=${str(exit.version)}`,
    ...(typeof exit.rssKb === "number" ? [`rss=${Math.round(exit.rssKb / 1024)}MB`] : []),
    ...(exit.description ? [`desc=${str(exit.description)}`] : []),
  ].join(" ");
}

/**
 * When the watchdog gives up on ac3go. A window is at least WINDOW_SEC of
 * decoded audio; STRIKES windows in a row must be over the line.
 *
 * - decode time above RTF_SLOW of the audio's own duration: on a real box
 *   the decoder shares the CPU with the video path and the UI, and a decoder
 *   that already needs 70% of real time has no margin left;
 * - or the output running dry UNDERRUNS times in the window while the decoder
 *   is visibly busy (RTF_BUSY): a network stall also starves the output, but
 *   it pauses the player after one underrun and leaves the decoder idle.
 */
export const WATCHDOG = { WINDOW_SEC: 4, STRIKES: 2, RTF_SLOW: 0.7, RTF_BUSY: 0.3, UNDERRUNS: 3 } as const;

type Sample = Pick<DecoderStats, "frames" | "sampleRate" | "decodeMs" | "underruns">;

/**
 * Feeds on successive stats snapshots; returns why ac3go cannot keep up once
 * it is sure, null meanwhile. A decoder recreated (counters going back) starts
 * a new window.
 */
export function createDecoderWatchdog(): (stats: Sample) => string | null {
  let base: Sample | null = null;
  let strikes: string[] = [];
  return (stats) => {
    if (!base || stats.frames < base.frames || stats.underruns < base.underruns) {
      base = stats;
      strikes = [];
      return null;
    }
    const sec = framesToSec(stats.frames - base.frames, stats.sampleRate);
    if (sec < WATCHDOG.WINDOW_SEC) return null;
    const rtf = (stats.decodeMs - base.decodeMs) / 1000 / sec;
    const under = stats.underruns - base.underruns;
    base = stats;
    const behind = rtf > WATCHDOG.RTF_SLOW || (under >= WATCHDOG.UNDERRUNS && rtf > WATCHDOG.RTF_BUSY);
    if (!behind) {
      strikes = [];
      return null;
    }
    strikes.push(`rtf=${rtf.toFixed(2)}/ur=${under}`);
    return strikes.length >= WATCHDOG.STRIKES ? strikes.join(",") : null;
  };
}
