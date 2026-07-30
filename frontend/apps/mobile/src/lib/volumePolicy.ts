// Where the player's 0-2 volume bar actually lands.
//
// The bar used to drive `player.volume` for its 0-100% half, which MULTIPLIES
// with the device volume. On a tablet left at 20% that made the >100% boost
// nearly pointless: the amplifier had an already-attenuated signal to work with,
// so 200% barely differed from 100% (user report, 2026-07-30). Where the device
// volume is controllable, the bar therefore IS the device volume, and the boost
// stays what it should be - the last resort past the maximum.
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

  // Everywhere else playerVolume stays at 1: letting it fade too would square
  // the attenuation (50% of 50% sounds like 25%). The boost only ever engages
  // once the device is at maximum.
  return { deviceFraction: Math.min(1, v), playerVolume: 1, boostGain };
}
