import type { AudioProbe } from "./audioProbe";

export { formatAudioProbe, MAX_DETAIL_LEN } from "./audioProbe";
export type { AudioProbe } from "./audioProbe";
export { reportAudioProbeOnce, resetAudioProbeLatch } from "./reportProbe";
export type { ProbePoster, ReportOutcome } from "./reportProbe";

interface EveyMediaCapsNative {
  getAudioProbe(): Promise<AudioProbe>;
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
