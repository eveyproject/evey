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

// GAIN_STEP is the smallest change worth touching a LIVE audio effect for. 5% of
// gain is inaudible; re-arming the effect is not.
const GAIN_STEP = 0.05;

// lastApplied is the gain the native side is currently on, "" when nothing has
// been applied since the last release. Everything at or below 1.0 collapses to
// the single OFF state: the effect is bypassed there, so 0.9 and 0.2 are the
// same instruction and must not each re-touch it.
let lastApplied: number | null = null;
let inFlight: Promise<boolean> | null = null;
let pending: number | null = null;

function normalize(gain: number): number {
  if (!Number.isFinite(gain)) return 1;
  return gain <= 1 ? 1 : Math.min(gain, 2);
}

function worthApplying(next: number): boolean {
  if (lastApplied === null) return true;
  if (next <= 1) return lastApplied > 1; // leaving the boosted zone: exactly once
  return Math.abs(next - lastApplied) >= GAIN_STEP;
}

/** Test seam: the module-level state would otherwise leak between cases. */
export function _resetAudioBoostForTest(): void {
  lastApplied = null;
  inFlight = null;
  pending = null;
}

async function drain(): Promise<boolean> {
  let result = false;
  while (pending !== null) {
    const next = pending;
    pending = null;
    try {
      result = await native!.setBoost(next);
      lastApplied = next;
    } catch {
      // The device has no global-session effect support, or the effect died.
      // Forget the applied state so a later attempt is not skipped as a no-op,
      // and report "not boosting" so the caller can stay at 100%.
      lastApplied = null;
      result = false;
    }
  }
  inFlight = null;
  return result;
}

/**
 * Apply a VLC-style amplification: 1.0 = 100% (off), 2.0 = 200%. Returns whether
 * the boost is actually active - false where unsupported (iOS for now, or a device
 * whose global audio session has no effect support), so callers can fall back to
 * the native 0-100% volume without amplification.
 *
 * COALESCED on purpose. Both callers - the volume bar's onResponderMove and the
 * vertical swipe gesture - fire on every touch move, around 60 times a second.
 * Each of those used to reach LoudnessEnhancer.setTargetGain + setEnabled on the
 * live output mix; dragging the volume DOWN therefore re-armed a running audio
 * effect dozens of times per second and the sound broke up (reported on a
 * tablet, 2026-07-30). Rate-limiting in the UI would only paper over it, and
 * every future caller would have to remember: the throttle belongs here.
 *
 * Three rules: skip a change too small to hear, never overlap native calls, and
 * always finish on the value the user last asked for (dropping the tail would
 * leave the boost on after they turned it down).
 */
export async function setAudioBoost(gain: number): Promise<boolean> {
  if (!native) return false;
  const next = normalize(gain);
  if (!worthApplying(next)) return lastApplied !== null && lastApplied > 1;
  pending = next;
  if (inFlight) return inFlight;
  inFlight = drain();
  return inFlight;
}

export async function releaseAudioBoost(): Promise<void> {
  lastApplied = null;
  pending = null;
  inFlight = null;
  if (!native) return;
  try {
    await native.release();
  } catch {
    /* effect already gone */
  }
}
