// Shared implementation lives in @evey/client (packages/client/src/native/deviceCodecs.ts).
// This module keeps the path the screens and their tests import.
export { ANDROID_BASELINE, APPLE_CODECS, ANDROID_TV_BASELINE, APPLE_TV_CODECS, getDeviceAudioCodecs, getFmp4AudioPassthrough, resolveAudioCodecs, getDeviceHdrSupport, getDeviceMaxH264Level } from "@evey/client/native/deviceCodecs";
export type { AudioProbeOutcome } from "@evey/client/native/deviceCodecs";
