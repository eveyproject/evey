import { NativeModules, Platform } from "react-native";

interface DeviceVolumeNative {
  getDeviceVolume(): Promise<number>;
  setDeviceVolume(fraction: number): Promise<number>;
  getDeviceVolumeSteps?(): Promise<number>;
}

// Same native module as the boost (AudioBoostModule.kt): both are about what
// comes out of the speaker, and one bridge is one place to look. Absent on iOS
// and on any build predating the methods - callers must check
// hasDeviceVolumeControl() and fall back to the player element's own volume.
const native: DeviceVolumeNative | undefined =
  Platform.OS === "android"
    ? (NativeModules.AudioBoost as DeviceVolumeNative | undefined)
    : undefined;

export function hasDeviceVolumeControl(): boolean {
  return typeof native?.setDeviceVolume === "function";
}

// STREAM_MUSIC has a coarse range (commonly 15 steps), so a finger drag emitting
// ~60 events a second would send dozens of writes that land on the same index.
// Coalescing here keeps the bridge quiet without the UI having to know.
const FRACTION_STEP = 1 / 32;

let lastRequested: number | null = null;
let inFlight: Promise<number | null> | null = null;
let pending: number | null = null;

/** Test seam: module state would otherwise leak between cases. */
export function _resetDeviceVolumeForTest(): void {
  lastRequested = null;
  inFlight = null;
  pending = null;
  cachedSteps = undefined;
}

// The step count never changes while the app runs, and the volume policy needs
// it synchronously on every drag event - so it is fetched once and cached.
let cachedSteps: number | null | undefined;

/**
 * Hardware step count of the media stream (commonly 15), or null when the
 * bridge cannot say (older native build, iOS). Feeds volumePolicy's smooth
 * split: the device moves per hardware step, the player element fades the
 * remainder, and adjusting the volume stops sounding like a staircase
 * (user report, 2026-08-31).
 */
export async function readDeviceVolumeSteps(): Promise<number | null> {
  if (cachedSteps !== undefined) return cachedSteps;
  if (!native?.getDeviceVolumeSteps) {
    cachedSteps = null;
    return null;
  }
  try {
    const n = await native.getDeviceVolumeSteps();
    cachedSteps = Number.isFinite(n) && n >= 2 ? Math.floor(n) : null;
  } catch {
    cachedSteps = null;
  }
  return cachedSteps;
}

/** Current device media volume as 0-1, or null when it cannot be read. */
export async function readDeviceVolumeFraction(): Promise<number | null> {
  if (!native?.getDeviceVolume) return null;
  try {
    const v = await native.getDeviceVolume();
    return Number.isFinite(v) ? Math.max(0, Math.min(1, v)) : null;
  } catch {
    return null;
  }
}

async function drain(): Promise<number | null> {
  let applied: number | null = null;
  while (pending !== null) {
    const next = pending;
    pending = null;
    try {
      applied = await native!.setDeviceVolume(next);
      lastRequested = next;
    } catch {
      // Refused (a Do-Not-Disturb policy can forbid it) or unsupported. Forget
      // the last value so a later attempt is not skipped as a no-op.
      lastRequested = null;
      applied = null;
    }
  }
  inFlight = null;
  return applied;
}

/**
 * Set the device media volume from a 0-1 fraction. Resolves with the level the
 * device ACTUALLY reached (it snaps to its own step count), or null when it
 * could not be set - the caller then keeps showing its own value rather than a
 * level that never happened.
 *
 * Coalesced and single-flight, like setAudioBoost: the volume bar and the
 * vertical swipe both emit on every touch move.
 */
export async function setDeviceVolumeFraction(fraction: number): Promise<number | null> {
  if (!native?.setDeviceVolume) return null;
  const next = Number.isFinite(fraction) ? Math.max(0, Math.min(1, fraction)) : 0;
  // 0 and 1 are endpoints the user aims at deliberately: never let the step
  // threshold swallow "silence" or "maximum".
  const isEndpoint = next === 0 || next === 1;
  if (!isEndpoint && lastRequested !== null && Math.abs(next - lastRequested) < FRACTION_STEP) {
    return null;
  }
  pending = next;
  if (inFlight) return inFlight;
  inFlight = drain();
  return inFlight;
}
