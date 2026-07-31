import type { AudioProbe } from "./audioProbe";

export { formatAudioProbe, MAX_DETAIL_LEN } from "./audioProbe";
export type { AudioProbe } from "./audioProbe";
export { reportAudioProbeOnce, resetAudioProbeLatch } from "./reportProbe";
export type { ProbePoster, ReportOutcome } from "./reportProbe";

interface EveyMediaCapsNative {
  getAudioProbe(): Promise<AudioProbe>;
  getPublicDownloadsDir?(): string | null;
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
