import { NativeModules, Platform } from "react-native";

interface AudioBoostNative {
  setBoost(gain: number): Promise<boolean>;
  release(): Promise<void>;
}

// Android exposes the native LoudnessEnhancer bridge (see AudioBoostModule.kt).
// iOS/tvOS will get their own module later; until then this is a no-op there.
const native: AudioBoostNative | undefined =
  Platform.OS === "android"
    ? (NativeModules.AudioBoost as AudioBoostNative | undefined)
    : undefined;

/** True when >100% amplification is actually available on this platform/build. */
export const audioBoostSupported = native != null;

/**
 * Apply a VLC-style amplification: 1.0 = 100% (off), 2.0 = 200%. Returns whether
 * the boost is actually active — false where unsupported (iOS for now, or a device
 * whose global audio session has no effect support), so callers can fall back to
 * the native 0-100% volume without amplification.
 */
export async function setAudioBoost(gain: number): Promise<boolean> {
  if (!native) return false;
  try {
    return await native.setBoost(gain);
  } catch {
    return false;
  }
}

export async function releaseAudioBoost(): Promise<void> {
  if (!native) return;
  try {
    await native.release();
  } catch {
    /* effect already gone */
  }
}
