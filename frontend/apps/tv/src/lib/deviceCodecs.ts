import { NativeModules, Platform } from "react-native";

interface MediaCapsNative {
  getSupportedAudioCodecs(): Promise<string[]>;
}

// Safe floor for Android TV boxes: every Android device decodes these in
// software (AOSP bundled decoders). Deliberately no ac3/eac3: this Expo-managed
// TV project ships no native probe module today (no android/ios folder at all -
// see the task report for the exact plan to port apps/mobile's MediaCapsModule.kt
// here), so there is no way to tell a box with a real Dolby decoder or HDMI
// passthrough from one without. A direct-played ac3/eac3 track the box can't
// decode/pass through plays the video SILENT, with nothing surfaced anywhere
// for a user or an operator to diagnose. Declaring less only costs a
// server-side audio-only transcode (video stays copy), which is cheap even on
// the weak-NAS hardware this project targets - unlike an undiagnosable silent
// movie. Mirrors the same call already made for the mobile app, see
// apps/mobile/src/lib/deviceCodecs.ts.
export const ANDROID_TV_BASELINE = ["aac", "mp3", "opus", "flac", "vorbis"];

// AVPlayer (tvOS) decodes AC-3/E-AC-3 on every Apple TV we support; it can't
// demux mkv anyway so the exotic codecs never reach it outside mp4/hls.
export const APPLE_TV_CODECS = ["aac", "mp3", "flac", "ac3", "eac3"];

const BASELINE = Platform.OS === "android" ? ANDROID_TV_BASELINE : APPLE_TV_CODECS;

// NATIVE PROBE EXTENSION POINT: no native module ships in this project yet.
// Reads NativeModules.MediaCaps with the same shape as apps/mobile's lookup so
// wiring the real ExoPlayer AudioCapabilities probe later is a no-op for every
// caller of getDeviceAudioCodecs() - only this file changes. Optional chaining
// on NativeModules (not just `.MediaCaps`) also tolerates the minimal test
// double in src/test/__mocks__/react-native.ts, which does not export it.
const native: MediaCapsNative | undefined =
  Platform.OS === "android"
    ? ((NativeModules as Record<string, unknown> | undefined)?.MediaCaps as
        | MediaCapsNative
        | undefined)
    : undefined;

let current: string[] = BASELINE;

/**
 * The audio codecs this TV box actually decodes (or passes through over
 * HDMI), for the play-session's supportedAudioCodecs declaration. Synchronous:
 * returns the baseline until the one-shot native probe (kicked at module
 * load) lands, which is long before the user reaches a player screen.
 */
export function getDeviceAudioCodecs(): string[] {
  return current;
}

export type AudioProbeOutcome =
  | { status: "unavailable" }
  | { status: "error" }
  | { status: "ok"; codecs: string[] };

/**
 * Pure resolution of the codec list to declare to the server, given what the
 * native probe reported. No module installed, a thrown probe, and an empty
 * response all fall back to the conservative baseline; only a non-empty codec
 * list from a working probe overrides it (whether or not it includes Dolby).
 */
export function resolveAudioCodecs(baseline: string[], outcome: AudioProbeOutcome): string[] {
  if (outcome.status !== "ok" || outcome.codecs.length === 0) return baseline;
  return outcome.codecs;
}

async function probe(): Promise<void> {
  let outcome: AudioProbeOutcome = { status: "unavailable" };
  if (native) {
    try {
      const codecs = await native.getSupportedAudioCodecs();
      outcome = { status: "ok", codecs: Array.isArray(codecs) ? codecs : [] };
    } catch {
      outcome = { status: "error" };
    }
  }
  current = resolveAudioCodecs(BASELINE, outcome);
  // Single log line so a "silent audio" report can be diagnosed from device
  // logs without guessing what this box actually detected and declared.
  console.info(`[deviceCodecs] audio probe=${outcome.status} declared=${current.join(",")}`);
}

void probe();
