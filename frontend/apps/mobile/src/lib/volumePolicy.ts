// Where the player's 0-2 volume bar actually lands.
//
// The bar used to drive `player.volume` for its 0-100% half, which MULTIPLIES
// with the device volume. On a tablet left at 20% that made the >100% boost
// nearly pointless (user report, 2026-07-30), so the 0-100% half drives the
// DEVICE media volume where a control exists.
//
// Second report (2026-08-31): adjusting the volume sounded LUMPY. STREAM_MUSIC
// has ~15 hardware steps of 2-6 dB each, and a drag was riding them raw - every
// index change mid-gesture was an audible lurch. The device cannot fade any
// finer than its steps, but the player element can: with the step count known,
// the device is set to the step AT OR ABOVE the requested level and the player
// element trims the remainder, so the audible product tracks the finger
// CONTINUOUSLY (product == requested level, exactly) and the bridge is written
// at most once per hardware step. This is not the "two attenuations stack"
// mistake the earlier code guarded against - the product is the requested
// level by construction, not its square.
//
// Pure on purpose: this is the one place the split is decided, so both callers
// (the slider and the vertical swipe) and both platforms agree without either
// re-deriving it.

export interface VolumeTargets {
  /** Device media volume as 0-1, or null when this platform has no control. */
  deviceFraction: number | null;
  /** What to set on the video player element. */
  playerVolume: number;
  /** Amplification above 100%: 1 = off. */
  boostGain: number;
}

export interface VolumePolicyOptions {
  /** True when the native device-volume bridge is available (Android). */
  hasDeviceControl: boolean;
  /**
   * Hardware step count of the media stream (AudioManager.getStreamMaxVolume),
   * when known. Enables the smooth split above; absent, the raw fraction is
   * handed to the device as before (the bridge still coalesces).
   */
  deviceSteps?: number;
}

export function resolveVolume(level: number, opts: VolumePolicyOptions): VolumeTargets {
  const v = Number.isFinite(level) ? Math.max(0, Math.min(2, level)) : 1;
  const boostGain = v > 1 ? v : 1;

  if (!opts.hasDeviceControl) {
    // Historical path: the player element carries the whole 0-100% range.
    return { deviceFraction: null, playerVolume: Math.min(1, v), boostGain };
  }

  // Zero is the one level that must NOT reach the system: "no sound from Evey"
  // is not "mute the tablet". Writing 0 to the shared media stream would also
  // silence notifications and every other app, and leave the device muted after
  // the user closes the player. So silence is produced by the player itself and
  // the device keeps whatever level it was on.
  if (v === 0) {
    return { deviceFraction: null, playerVolume: 0, boostGain: 1 };
  }

  // At and above 100% the device is pinned to its maximum first - the boost
  // only ever amplifies a full-scale signal.
  if (v >= 1) {
    return { deviceFraction: 1, playerVolume: 1, boostGain };
  }

  const steps = opts.deviceSteps;
  if (!steps || !Number.isFinite(steps) || steps < 2) {
    // Step count unknown: hand the raw fraction to the device, player at full
    // (letting it fade too would square the attenuation).
    return { deviceFraction: v, playerVolume: 1, boostGain };
  }

  // Smooth split: device on the step at or above v, player trims down to v.
  const index = Math.min(steps, Math.max(1, Math.ceil(v * steps)));
  const deviceFraction = index / steps;
  return { deviceFraction, playerVolume: v / deviceFraction, boostGain };
}

/**
 * The player element's volume against the level the device is ON (read back or
 * last written), not the one on its way: while a step change is in flight the
 * product can only dip under the level, never overshoot it.
 */
export function playerTrim(level: number, deviceNow: number | null, opts: VolumePolicyOptions): number {
  const targets = resolveVolume(level, opts);
  if (targets.deviceFraction === null || !(level > 0 && level < 1) || !deviceNow) return targets.playerVolume;
  return Math.min(1, level / deviceNow);
}
