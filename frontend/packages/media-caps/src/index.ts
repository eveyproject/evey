import type { AudioProbe } from "./audioProbe";

export { formatAudioProbe, MAX_DETAIL_LEN } from "./audioProbe";
export { declarableAudioCodecs, sinkAcceptsCodec } from "./audioDeclaration";
export type { AudioProbe } from "./audioProbe";
export { reportAudioProbeOnce, resetAudioProbeLatch } from "./reportProbe";
export type { ProbePoster, ReportOutcome } from "./reportProbe";

interface EveyMediaCapsNative {
  getAudioProbe(): Promise<AudioProbe>;
  getPublicDownloadsDir?(): string | null;
  deleteFile?(path: string): boolean;
  moveFile?(from: string, to: string): boolean;
  getMaxH264Level?(): number;
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
