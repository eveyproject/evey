import { NativeModules, Platform } from "react-native";

interface AudioBoostNative {
  setBoost(gain: number): Promise<boolean>;
  release(): Promise<void>;
}

// Android sets the gain of the player's own preamp (see AudioBoostModule.kt).
// iOS/tvOS will get their own module later; until then this is a no-op there.
const native: AudioBoostNative | undefined =
  Platform.OS === "android"
    ? (NativeModules.AudioBoost as AudioBoostNative | undefined)
    : undefined;

// GAIN_STEP is the smallest change worth touching a LIVE audio effect for. 5% of
// gain is inaudible; re-arming the effect is not.
const GAIN_STEP = 0.05;

// target is the last gain handed to the native side (in flight or landed),
// null when nothing has been since the last release. Changes are measured
// against it, not against what has landed: a drag back under 100% while a
// boosted value is still on its way must still send "off". Everything at or
// below 1.0 collapses to the single OFF state.
let target: number | null = null;
let inFlight: Promise<boolean> | null = null;
let pending: number | null = null;
// Bumped by every release: a call still on its way belongs to the player that
// was released and must not record itself afterwards.
let generation = 0;

function normalize(gain: number): number {
  if (!Number.isFinite(gain)) return 1;
  return gain <= 1 ? 1 : Math.min(gain, 2);
}

function worthApplying(next: number): boolean {
  if (target === null) return true;
  if (next <= 1) return target > 1; // leaving the boosted zone: exactly once
  return Math.abs(next - target) >= GAIN_STEP;
}

/** Test seam: the module-level state would otherwise leak between cases. */
export function _resetAudioBoostForTest(): void {
  target = null;
  inFlight = null;
  pending = null;
  generation += 1;
}

/** False where nothing can amplify (iOS for now): the bar then stops at 100%. */
export function hasAudioBoost(): boolean {
  return !!native;
}

async function drain(): Promise<boolean> {
  const gen = generation;
  let result = false;
  while (pending !== null && gen === generation) {
    const next = pending;
    pending = null;
    try {
      result = await native!.setBoost(next);
    } catch {
      // The device has no effect support, or the effect died. Forget the
      // state so a later attempt is not skipped as a no-op, and report "not
      // boosting" so the caller can stay at 100%.
      if (gen === generation) target = null;
      result = false;
    }
  }
  if (gen === generation) inFlight = null;
  return result;
}

/**
 * Amplify above 100%: 1.0 = off, 2.0 = 200%. Returns whether the boost is on -
 * false where nothing can amplify (iOS for now).
 *
 * Coalesced: the volume bar and the vertical swipe fire ~60 times a second,
 * and each call used to re-arm a live audio effect (sound broke up on a
 * tablet, 2026-07-30). Skip a change too small to hear, never overlap native
 * calls, always finish on the value asked last.
 */
export async function setAudioBoost(gain: number): Promise<boolean> {
  if (!native) return false;
  const next = normalize(gain);
  if (!worthApplying(next)) return target !== null && target > 1;
  target = next;
  pending = next;
  if (inFlight) return inFlight;
  inFlight = drain();
  return inFlight;
}

export async function releaseAudioBoost(): Promise<void> {
  generation += 1;
  target = null;
  pending = null;
  inFlight = null;
  if (!native) return;
  try {
    await native.release();
  } catch {
    /* effect already gone */
  }
}
