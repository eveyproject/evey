import type { AudioProbe } from "./audioProbe";

export { formatAudioProbe, MAX_DETAIL_LEN } from "./audioProbe";
export { declarableAudioCodecs, sinkAcceptsCodec } from "./audioDeclaration";
export { displaySupportsHdr } from "./displayHdr";
export type { DisplayHdrType } from "./displayHdr";
export type { AudioProbe } from "./audioProbe";
export { reportAudioProbeOnce, resetAudioProbeLatch } from "./reportProbe";
export { buildCastStreamUrl, castProfileFor, castStreamInfoParams, FLOOR_PROFILE, h264HeightFromLevel, pickCastProfile, profileFromReceiverCaps } from "./castUrl";
export type { CastProfile, CastUrlExtras, ObservedReceiverCaps, ReceiverStreamInfo } from "./castUrl";
export { askReceiverCaps, CAPS_NAMESPACE, CAPS_REQUEST, CAPS_RETRY_MS, CAPS_TIMEOUT_MS, parseCapsReport } from "./castCaps";
export type { AskReceiverCapsOptions, CapsAnswer, CapsTransport } from "./castCaps";
export type { ProbePoster, ReportOutcome } from "./reportProbe";
export { formatDecoderStats, framesToSec, getPlaybackDecoderStats, isSilentRunAlarming, parseDecoderStats, resetPlaybackDecoderStats, SILENT_RUN_ALARM_SEC } from "./decoderStats";
export type { DecoderStats } from "./decoderStats";

interface EveyMediaCapsNative {
  getAudioProbe(): Promise<AudioProbe>;
  getHdrTypes?(): Promise<string[]>;
  getPublicDownloadsDir?(): string | null;
  deleteFile?(path: string): boolean;
  moveFile?(from: string, to: string): boolean;
  getMaxH264Level?(): number;
  getUiScalePercent?(): number | null;
  setUiScalePercent?(percent: number): boolean;
}

/**
 * The UI scale the app's density is scaled by at process start (the TV app's
 * MainApplication reads the same preference), or null when never set - the
 * platform default then applies. Same lazy-import contract: never throws.
 */
export async function getUiScalePercent(): Promise<number | null> {
  try {
    const { requireOptionalNativeModule } = await import("expo-modules-core");
    const native = requireOptionalNativeModule<EveyMediaCapsNative>("EveyMediaCaps");
    const v = native?.getUiScalePercent?.();
    return typeof v === "number" && v > 0 ? v : null;
  } catch {
    return null;
  }
}

/**
 * Persists a UI scale and restarts the app to apply it (React Native reads
 * the density once per process). Resolves false when the module is absent -
 * the caller keeps its current look. Does not resolve at all on success: the
 * process exits.
 */
export async function setUiScalePercent(percent: number): Promise<boolean> {
  try {
    const { requireOptionalNativeModule } = await import("expo-modules-core");
    const native = requireOptionalNativeModule<EveyMediaCapsNative>("EveyMediaCaps");
    return native?.setUiScalePercent?.(Math.round(percent)) === true;
  } catch {
    return false;
  }
}

/**
 * Highest H.264 level_idc (41 = 4.1, 51 = 5.1) the device's hardware decoder
 * takes, or null when unknown (module absent, older build, probe failure).
 * Same lazy-import contract as getAudioProbe: never throws.
 */
export async function getMaxH264Level(): Promise<number | null> {
  try {
    const { requireOptionalNativeModule } = await import("expo-modules-core");
    const native = requireOptionalNativeModule<EveyMediaCapsNative>("EveyMediaCaps");
    const level = native?.getMaxH264Level?.();
    return typeof level === "number" && level > 0 ? level : null;
  } catch {
    return null;
  }
}

/**
 * Codecs the app's own player decodes (the ac3go renderer of the offline-audio
 * module, phone builds only), [] when that module is absent. Never throws.
 */
export async function getPlayerAudioDecoders(): Promise<string[]> {
  try {
    const { requireOptionalNativeModule } = await import("expo-modules-core");
    const native = requireOptionalNativeModule<{ playbackDecoders?(): string[] }>("EveyOfflineAudio");
    const codecs = native?.playbackDecoders?.();
    return Array.isArray(codecs) ? codecs : [];
  } catch {
    return [];
  }
}

/**
 * Raw audio-output capability evidence, or null when the native module isn't in
 * this build. Never throws: a probe failure must not break playback.
 *
 * expo-modules-core is imported LAZILY, not at module load: its top-level code
 * reads globalThis.expo, which only exists inside a real RN runtime. A static
 * import would make this package unimportable from a plain node test - and the
 * formatter and the report latch, which are the parts worth testing, have no
 * native dependency at all.
 */
export async function getAudioProbe(): Promise<AudioProbe | null> {
  try {
    const { requireOptionalNativeModule } = await import("expo-modules-core");
    const native = requireOptionalNativeModule<EveyMediaCapsNative>("EveyMediaCaps");
    if (!native) return null;
    return await native.getAudioProbe();
  } catch {
    return null;
  }
}

/**
 * HDR formats the DISPLAY currently attached renders ("hdr10", "hlg",
 * "hdr10plus", "dovi"), empty for an SDR panel, null when the native module
 * isn't in this build. Same lazy-import contract as getAudioProbe: never throws.
 *
 * Probe it at every stream negotiation rather than once at startup - the
 * display changes under a running app (HDMI re-plugged, AVR powered on, mode
 * switch), exactly like the audio output does. A playback already running keeps
 * what it negotiated; a new screen takes effect at the next natural boundary.
 */
export async function getDisplayHdrTypes(): Promise<string[] | null> {
  try {
    const { requireOptionalNativeModule } = await import("expo-modules-core");
    const native = requireOptionalNativeModule<EveyMediaCapsNative>("EveyMediaCaps");
    if (!native?.getHdrTypes) return null;
    const types = await native.getHdrTypes();
    return Array.isArray(types) ? types : null;
  } catch {
    return null;
  }
}

/**
 * Public Downloads collection root ("/storage/emulated/0/Download" on most
 * devices), or null when the platform can't offer one the app may write to
 * without extra permissions (Android < 11, iOS, module absent). Same lazy
 * import contract as getAudioProbe: never throws, importable from plain node.
 */
export async function getPublicDownloadsDir(): Promise<string | null> {
  try {
    const { requireOptionalNativeModule } = await import("expo-modules-core");
    const native = requireOptionalNativeModule<EveyMediaCapsNative>("EveyMediaCaps");
    const dir = native?.getPublicDownloadsDir?.();
    return typeof dir === "string" && dir.length > 0 ? dir : null;
  } catch {
    return null;
  }
}

/**
 * Delete a media file via plain java.io.File - the escape hatch for paths
 * expo-file-system's legacy API refuses to touch (anything under the public
 * Downloads collection, even files this app owns). False when the file is
 * still there afterwards or the native module is absent; never throws.
 */
export async function deleteMediaFile(path: string): Promise<boolean> {
  try {
    const { requireOptionalNativeModule } = await import("expo-modules-core");
    const native = requireOptionalNativeModule<EveyMediaCapsNative>("EveyMediaCaps");
    return native?.deleteFile?.(path) === true;
  } catch {
    return false;
  }
}

/**
 * Same-volume rename via plain java.io.File, same rationale and same
 * contract as deleteMediaFile.
 */
export async function moveMediaFile(from: string, to: string): Promise<boolean> {
  try {
    const { requireOptionalNativeModule } = await import("expo-modules-core");
    const native = requireOptionalNativeModule<EveyMediaCapsNative>("EveyMediaCaps");
    return native?.moveFile?.(from, to) === true;
  } catch {
    return false;
  }
}
