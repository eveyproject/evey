/**
 * What the app's own AC-3/E-AC-3 decoder (ac3go, in the player's renderer) has
 * handed the audio sink during the current playback.
 *
 * A decoder that loses the audio without failing leaves no other trace: the
 * video plays, progress reports tick, the server logs nothing. That is how a
 * silent E-AC-3 downmix reached two phones unseen (2026-09-27). These counters
 * are the player's own evidence, reported by the telemetry beacon.
 */
export interface DecoderStats {
  codec: string;
  sourceChannels: number;
  outputChannels: number;
  maxChannels: number;
  sampleRate: number;
  /** Frames handed to the sink, damaged ones included. */
  frames: number;
  /** Frames that failed to decode and were replaced by silence. */
  badFrames: number;
  /** Frames handed back as pure digital silence, damaged ones included. */
  silentFrames: number;
  /** Consecutive silent frames ending at the last one. */
  silentRunFrames: number;
  /**
   * ac3go's own account, one line: failures by cause, dependent substreams
   * skipped, layout changes, frames downmixed, last coded layout. "" on a
   * build before it.
   */
  decoder: string;
}

const SAMPLES_PER_FRAME = 1536;

/**
 * A run this long of exact zeros is a lost track, not a quiet scene: ac3go
 * dithers the bands a quiet frame leaves empty, so even a whisper is not
 * bit-for-bit zero.
 */
export const SILENT_RUN_ALARM_SEC = 10;

/** Seconds of audio in `frames` frames at the stream's rate (0 when unknown). */
export function framesToSec(frames: number, sampleRate: number): number {
  return sampleRate > 0 ? (frames * SAMPLES_PER_FRAME) / sampleRate : 0;
}

/** True when the current silent run has lasted long enough to be reported. */
export function isSilentRunAlarming(stats: DecoderStats): boolean {
  return framesToSec(stats.silentRunFrames, stats.sampleRate) >= SILENT_RUN_ALARM_SEC;
}

/** One line for the beacon's details, well under the server's 200-char clamp. */
export function formatDecoderStats(stats: DecoderStats): string {
  return [
    `codec=${stats.codec}`,
    `src=${stats.sourceChannels}ch`,
    `out=${stats.outputChannels}ch`,
    `max=${stats.maxChannels}ch`,
    `rate=${stats.sampleRate}`,
    `frames=${stats.frames}`,
    `bad=${stats.badFrames}`,
    `silent=${stats.silentFrames}`,
    `run=${stats.silentRunFrames}`,
    ...(stats.decoder ? [stats.decoder] : []),
  ].join(" ");
}

function toStats(raw: unknown): DecoderStats | null {
  if (!raw || typeof raw !== "object") return null;
  const r = raw as Record<string, unknown>;
  const num = (k: string) => (typeof r[k] === "number" ? (r[k] as number) : 0);
  if (typeof r.codec !== "string") return null;
  return {
    codec: r.codec,
    sourceChannels: num("sourceChannels"),
    outputChannels: num("outputChannels"),
    maxChannels: num("maxChannels"),
    sampleRate: num("sampleRate"),
    frames: num("frames"),
    badFrames: num("badFrames"),
    silentFrames: num("silentFrames"),
    silentRunFrames: num("silentRunFrames"),
    decoder: typeof r.decoder === "string" ? r.decoder : "",
  };
}

/**
 * The counters of the current (or last) ac3go playback, null when ac3go never
 * took a track or the module is absent (iOS, web, an older build). Never throws.
 */
export async function getPlaybackDecoderStats(): Promise<DecoderStats | null> {
  try {
    // Lazy for the same reason as getAudioProbe: expo-modules-core reads
    // globalThis.expo at import time.
    const { requireOptionalNativeModule } = await import("expo-modules-core");
    const native = requireOptionalNativeModule<{ playbackDecoderStats?(): unknown }>("EveyOfflineAudio");
    return toStats(native?.playbackDecoderStats?.());
  } catch {
    return null;
  }
}

/**
 * Forget the previous playback's counters; a player calls it as it opens.
 * Never throws.
 */
export async function resetPlaybackDecoderStats(): Promise<void> {
  try {
    const { requireOptionalNativeModule } = await import("expo-modules-core");
    requireOptionalNativeModule<{ resetPlaybackDecoderStats?(): void }>("EveyOfflineAudio")?.resetPlaybackDecoderStats?.();
  } catch {
    /* module absent: nothing to forget */
  }
}

/** Test seam for the parsing of the native map. */
export const parseDecoderStats = toStats;
