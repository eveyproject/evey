import { getAudioProbe, type AudioProbe } from "@evey/media-caps";

// Twin of apps/mobile/src/lib/outputChannels.ts - keep the SEMANTICS in sync
// when touching either side, like deviceCodecs.ts and usePlaySession.ts already
// are across the two apps.
//
// Channel ceiling of the RESTITUTION chain (what the speakers reproduce), not
// of the decoder. ExoPlayer decodes 5.1 on any box and then folds it to a
// stereo output; the fold sums the surrounds and the centre onto L/R without
// normalisation, so the dialogue (centre, attenuated, alone) survives while
// music and effects clip - the reported symptom. Declaring the real ceiling
// moves the down-mix to ffmpeg, which normalises it. On a TV this is also what
// keeps a 5.1 amplifier at 5.1: the HDMI sink states its own maximum.
//
// Android has no single call for "how many channels will come out". What the
// existing native probe already reports is used instead, in decreasing order
// of certainty. Nothing is inferred beyond it: an output we cannot describe
// returns undefined and the parameter is omitted, leaving the server profile
// untouched rather than demoting a real surround rig.

// Output device types that physically cannot carry more than two channels.
// AudioDeviceInfo.TYPE_* raw values, the same encoding the native probe emits.
// Anything absent from this list (HDMI, USB device, dock, line out) may well be
// multichannel, so it is treated as unknown rather than folded to stereo.
const STEREO_ONLY_TYPES = new Set([
  1, // BUILTIN_EARPIECE
  2, // BUILTIN_SPEAKER
  3, // WIRED_HEADSET
  4, // WIRED_HEADPHONES
  7, // BLUETOOTH_SCO
  8, // BLUETOOTH_A2DP
  18, // TELEPHONY
  22, // USB_HEADSET
  23, // HEARING_AID
  24, // BUILTIN_SPEAKER_SAFE
  26, // BLE_HEADSET
  27, // BLE_SPEAKER
  30, // BLE_BROADCAST
]);

// The server clamps the query to 1..8 (maxAudioChannelsQuery); mirror it so an
// out-of-range platform reading is dropped instead of rejected server-side.
const MAX_DECLARABLE = 8;

/**
 * The channel ceiling to declare, or undefined when the probe does not let us
 * answer. Pure, so the whole decision table is testable without a device.
 */
export function resolveMaxOutputChannels(probe: AudioProbe | null | undefined): number | undefined {
  if (!probe) return undefined;
  // A plugged HDMI sink states its own maximum (EXTRA_MAX_CHANNEL_COUNT): the
  // strongest evidence available, and the one that keeps a 5.1/7.1 amplifier
  // at 5.1/7.1. plugState 0 means nothing is connected, so it says nothing.
  if (probe.hdmi?.available && probe.hdmi.plugState === 1) {
    const n = probe.hdmi.maxChannelCount;
    if (typeof n === "number" && Number.isFinite(n) && n >= 1) {
      return Math.min(Math.floor(n), MAX_DECLARABLE);
    }
    return undefined;
  }
  // No HDMI in the picture: if every output this device offers is a stereo-only
  // type, whichever one is routed is stereo. Covers a box on Bluetooth and the
  // built-in speakers of an Android TV panel.
  const types = probe.outputDeviceTypes ?? [];
  if (types.length > 0 && types.every((t) => STEREO_ONLY_TYPES.has(t))) return 2;
  return undefined;
}

/**
 * Probe the current output. Called at every stream negotiation rather than
 * cached at startup: the output changes mid-session (an amplifier powered on,
 * a soundbar switched in). A playback already running keeps the layout it negotiated -
 * re-negotiating mid-scene would reload the manifest - so a new output takes
 * effect at the next natural boundary (track switch, quality change, next
 * episode, replay).
 */
export async function probeMaxOutputChannels(): Promise<number | undefined> {
  return resolveMaxOutputChannels(await getAudioProbe());
}
