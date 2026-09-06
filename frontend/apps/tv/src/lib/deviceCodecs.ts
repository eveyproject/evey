import { NativeModules, Platform } from "react-native";
import { declarableAudioCodecs, getAudioProbe, type AudioProbe } from "@evey/media-caps";

interface MediaCapsNative {
  getSupportedAudioCodecs(): Promise<string[]>;
}

// Safe floor for Android TV boxes: every Android device decodes these in
// software (AOSP bundled decoders). Deliberately no ac3/eac3: the family is
// declared only on evidence from the sink probe below. A direct-played
// ac3/eac3 track the box can't decode/pass through plays the video SILENT,
// with nothing surfaced anywhere for a user or an operator to diagnose.
// Declaring less only costs a server-side audio-only transcode (video stays
// copy), which is cheap even on the weak-NAS hardware this project targets -
// unlike an undiagnosable silent movie. Mirrors apps/mobile/src/lib/deviceCodecs.ts.
export const ANDROID_TV_BASELINE = ["aac", "mp3", "opus", "flac", "vorbis"];

// AVPlayer (tvOS) decodes AC-3/E-AC-3 on every Apple TV we support; it can't
// demux mkv anyway so the exotic codecs never reach it outside mp4/hls.
export const APPLE_TV_CODECS = ["aac", "mp3", "flac", "ac3", "eac3"];

const BASELINE = Platform.OS === "android" ? ANDROID_TV_BASELINE : APPLE_TV_CODECS;

// Legacy old-bridge probe (apps/mobile's MediaCapsModule.kt). No such module
// ships in this TV project - the Expo module behind getAudioProbe() is the one
// that does - but the lookup stays so a future port is a no-op for callers.
// Optional chaining on NativeModules (not just `.MediaCaps`) also tolerates
// the minimal test double in src/test/__mocks__/react-native.ts.
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

// Audio codecs this box takes as a RAW passthrough inside the server's fMP4
// segments, on top of the always-wired AAC/AC-3/E-AC-3. A DIFFERENT question
// from getDeviceAudioCodecs(): both platforms here decode MP3, but only
// media3/ExoPlayer reads it back out of an fMP4 `mp4a` sample entry. tvOS runs
// on AVPlayer, which does not take MP3 in that container at all, so Apple TV
// declares nothing and keeps the server-side AAC re-encode.
//
// Why ask: re-encoding an already-lossy track cascades. Measured on a 128 kbps
// MP3 DVD rip (2026-09-06), coding noise under the signal at the server's 192k
// rendition was 30.6 dB on dialogue but only 20.3 dB on dense music. A
// passthrough adds none of it. Mirrors apps/mobile/src/lib/deviceCodecs.ts.
const FMP4_AUDIO_PASSTHROUGH = Platform.OS === "android" ? ["mp3"] : [];

/**
 * The audio codecs this box takes muxed in fMP4, for the play session's
 * fmp4AudioPassthrough declaration (proto field + `?fmp4AudioPassthrough=` on
 * the HLS master). Static: it depends on the player engine, not on the box.
 */
export function getFmp4AudioPassthrough(): string[] {
  return FMP4_AUDIO_PASSTHROUGH;
}

export type AudioProbeOutcome =
  | { status: "unavailable" }
  | { status: "error" }
  | { status: "ok"; codecs: string[] };

/**
 * Pure resolution of the codec list to declare to the server. The decoder
 * list comes from the legacy module when it answered, else from the sink
 * probe's own MediaCodecList reading; with no evidence at all the baseline
 * goes through untouched (Apple TV keeps its Dolby, Android TV has none).
 *
 * For the Dolby/DTS family the SINK decides, not the decoder: a decoder that
 * renders choppy (measured on a tablet, 2026-08-31) must not be declared, and
 * a passthrough box without any decoder must be - ExoPlayer hands the
 * bitstream to the receiver. See declarableAudioCodecs in @evey/media-caps.
 */
export function resolveAudioCodecs(
  baseline: string[],
  outcome: AudioProbeOutcome,
  probe: AudioProbe | null,
): string[] {
  const legacy = outcome.status === "ok" && outcome.codecs.length > 0 ? outcome.codecs : null;
  const decoders = legacy ?? (probe && probe.decoders.length > 0 ? probe.decoders : null);
  if (!decoders) return baseline;
  return declarableAudioCodecs(decoders, probe);
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
  // Never throws, null when the Expo module isn't in this build.
  const sink = Platform.OS === "android" ? await getAudioProbe() : null;
  current = resolveAudioCodecs(BASELINE, outcome, sink);
  // Single log line so a "silent audio" report can be diagnosed from device
  // logs without guessing what this box actually detected and declared.
  console.info(
    `[deviceCodecs] audio probe=${outcome.status} sink=${sink ? "ok" : "none"} declared=${current.join(",")}`,
  );
}

void probe();
