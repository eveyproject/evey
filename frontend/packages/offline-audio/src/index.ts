/**
 * Post-download audio compatibility pass.
 *
 * A downloaded film whose only audio is AC-3/E-AC-3 plays silent on devices
 * that cannot render those codecs. This package rewrites the file once, right
 * after download: the embedded Go pipeline (ac3go + mkvgo) decodes and
 * downmixes the audio to stereo, Android's hardware encoder turns it into
 * AAC, and the file is remuxed with the video untouched. After the pass the
 * file plays everywhere, sound included, online and offline.
 *
 * Android-only by design: on iOS and in tests the module is absent and
 * `isOfflineAudioAvailable` says so. The pass must also never run for a TV
 * profile (it would destroy surround that passes through natively), but that
 * gate belongs to the caller which knows what it is running on.
 */

interface NativeSummary {
  sampleRate: number;
  sourceCodec: string;
  sourceChannels: number;
  aacFrames: number;
  skippedFrames: number;
}

export interface OptimizeProgress {
  /**
   * "transcode" walks the source (decode+encode); "remux" writes the output;
   * "join" reassembles a sliced reduced-quality download.
   */
  stage: "transcode" | "remux" | "join";
  /** 0..1000 within the stage. */
  permille: number;
}

export type OptimizeSummary = NativeSummary;

/**
 * Machine-readable failure codes. The Go side encodes them as error message
 * prefixes; the Kotlin side lifts the prefix into the exception code.
 * - "busy": another pass is running (one at a time, by design)
 * - "cancelled": cancelOptimize() was called
 * - "not-matroska": the file is not an MKV (mp4 downloads are not handled yet)
 * - "unsupported-codec": DTS/TrueHD - only the server-side fallback can help
 * - "no-convertible-track" / "no-audio" / "no-decodable-audio": nothing to do
 * - "unavailable": native module absent (iOS, tests, Expo Go)
 *
 * The slice join adds two of its own:
 * - "no-slices" / "slice-missing": the collected set has a hole, so the join
 *   was refused rather than silently producing half a film
 * - "join-failed": mkvgo refused the slices (mismatched codec configuration)
 *   or the write failed
 */
export type OptimizeErrorCode =
  | "busy"
  | "cancelled"
  | "not-matroska"
  | "no-audio"
  | "unsupported-codec"
  | "no-convertible-track"
  | "no-decodable-audio"
  | "no-slices"
  | "slice-missing"
  | "join-failed"
  | "unavailable"
  | "failed";

export class OptimizeError extends Error {
  readonly code: OptimizeErrorCode;
  constructor(code: OptimizeErrorCode, message: string) {
    super(message);
    this.name = "OptimizeError";
    this.code = code;
  }
}

interface NativeJoinSummary {
  slices: number;
  path: string;
}

interface NativeModuleShape {
  optimize(inPath: string, outPath: string, label: string): Promise<NativeSummary>;
  joinSlices(dir: string, count: number, outPath: string, durationMs: number, label: string): Promise<NativeJoinSummary>;
  deleteSlices(dir: string, count: number): void;
  cancel(): void;
  isRunning(): boolean;
  /** Absent on a native build older than the translated notification title. */
  setNotificationTitle?(title: string): void;
  addListener(event: "progress", listener: (p: OptimizeProgress) => void): { remove(): void };
}

async function nativeModule(): Promise<NativeModuleShape | null> {
  try {
    // Lazy for the same reason as @evey/media-caps: expo-modules-core reads
    // globalThis.expo at import time, which only exists in a real RN runtime.
    const { requireOptionalNativeModule } = await import("expo-modules-core");
    return requireOptionalNativeModule<NativeModuleShape>("EveyOfflineAudio");
  } catch {
    return null;
  }
}

/** True when the native pass exists in this build (Android app builds only). */
export async function isOfflineAudioAvailable(): Promise<boolean> {
  return (await nativeModule()) != null;
}

const KNOWN_CODES: readonly OptimizeErrorCode[] = [
  "busy",
  "cancelled",
  "not-matroska",
  "no-audio",
  "unsupported-codec",
  "no-convertible-track",
  "no-decodable-audio",
  "no-slices",
  "slice-missing",
  "join-failed",
];

function toOptimizeError(e: unknown): OptimizeError {
  const raw = (e as { code?: string })?.code ?? "";
  const code = KNOWN_CODES.find((c) => c === raw) ?? "failed";
  return new OptimizeError(code, e instanceof Error ? e.message : String(e));
}

/**
 * Run the pass on one file. `inPath`/`outPath` are plain filesystem paths (no
 * file:// scheme - they cross into Go's os.Open). The output is written next
 * to the source by the caller's convention so the final rename stays on one
 * volume. Rejects with an OptimizeError carrying a routable `code`.
 *
 * One pass runs at a time; a second call rejects "busy" instead of queueing.
 * Queueing (and its charger/Wi-Fi gating) is the caller's job.
 */
export async function optimizeAudio(
  inPath: string,
  outPath: string,
  onProgress?: (p: OptimizeProgress) => void,
  /** Shown in the pass's foreground-service notification (the media title). */
  label?: string,
  /** That notification's title, in the app's language. */
  notificationTitle?: string,
): Promise<OptimizeSummary> {
  const native = await nativeModule();
  if (!native) {
    throw new OptimizeError("unavailable", "offline audio pass not available in this build");
  }
  if (notificationTitle) native.setNotificationTitle?.(notificationTitle);
  const sub = onProgress ? native.addListener("progress", onProgress) : null;
  try {
    return await native.optimize(inPath, outPath, label ?? "");
  } catch (e) {
    throw toOptimizeError(e);
  } finally {
    sub?.remove();
  }
}

/** Ask the running pass (if any) to stop; it rejects with code "cancelled". */
export async function cancelOptimize(): Promise<void> {
  const native = await nativeModule();
  native?.cancel();
}

/**
 * The on-disk name of one slice of a reduced-quality download. The server's
 * segment muxer chose it and the Go side rebuilds it, so the downloader must
 * save under exactly this - a mismatch is a join that finds nothing.
 */
export function sliceFileName(index: number): string {
  return `slice-${String(index).padStart(5, "0")}.mkv`;
}

/**
 * Reassemble a size-reduced (720p) download from the slices in `dir` into
 * `outPath` (plain filesystem paths, no file:// scheme - they cross into Go).
 *
 * `durationMs` is the film's true length, which ONLY the server knows: mkvgo's
 * join copies the first slice's duration into the output, so without this the
 * finished download would announce ten minutes and refuse to seek past it.
 *
 * The slices are deliberately left in place - call `deleteJoinedSlices` once
 * the finished file is recorded. They are the only thing standing between a
 * crash and re-downloading gigabytes.
 */
export async function joinDownloadSlices(
  dir: string,
  count: number,
  outPath: string,
  durationMs: number,
  onProgress?: (p: OptimizeProgress) => void,
  /** Shown in the foreground-service notification (the media title). */
  label?: string,
  /** That notification's title, in the app's language. */
  notificationTitle?: string,
): Promise<void> {
  const native = await nativeModule();
  if (!native) {
    throw new OptimizeError("unavailable", "slice join not available in this build");
  }
  if (notificationTitle) native.setNotificationTitle?.(notificationTitle);
  const sub = onProgress ? native.addListener("progress", onProgress) : null;
  try {
    await native.joinSlices(dir, count, outPath, durationMs, label ?? "");
  } catch (e) {
    throw toOptimizeError(e);
  } finally {
    sub?.remove();
  }
}

/** Drop the collected slices once the joined file is recorded. Best effort. */
export async function deleteJoinedSlices(dir: string, count: number): Promise<void> {
  const native = await nativeModule();
  native?.deleteSlices(dir, count);
}
