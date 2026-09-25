import { NativeModules, Platform } from "react-native";
import { declarableAudioCodecs, displaySupportsHdr, getAudioProbe, getDisplayHdrTypes, getMaxH264Level, getPlayerAudioDecoders, type AudioProbe } from "@evey/media-caps";
import { clientLogger } from "../log";

const log = clientLogger("deviceCodecs");

interface MediaCapsNative {
  getSupportedAudioCodecs(): Promise<string[]>;
  getHdrTypes?(): Promise<string[]>;
}

// Every Android device (phone, tablet, TV box) decodes these in software
// (AOSP bundled decoders) - the safe floor when no probe answers. Deliberately
// NO ac3/eac3/dts/truehd: the family is declared only on evidence from the
// sink probe below. A direct-played track the device can't decode plays the
// video SILENT, with nothing surfaced anywhere for a user or an operator to
// diagnose. Declaring less only costs a server-side audio-only transcode
// (video stays copy), cheap even on the weak-NAS hardware this project
// targets - unlike an undiagnosable silent movie.
export const ANDROID_BASELINE = ["aac", "mp3", "opus", "flac", "vorbis"];

// AVPlayer decodes AC-3/E-AC-3 on every iOS/tvOS device we support; it can't
// demux mkv anyway so the exotic codecs never reach it outside mp4/hls.
export const APPLE_CODECS = ["aac", "mp3", "flac", "ac3", "eac3"];

// The names the TV app grew up with; same lists.
export const ANDROID_TV_BASELINE = ANDROID_BASELINE;
export const APPLE_TV_CODECS = APPLE_CODECS;

const BASELINE = Platform.OS === "android" ? ANDROID_BASELINE : APPLE_CODECS;

// Legacy old-bridge probe (apps/mobile's MediaCapsModule.kt). The TV project
// ships no such module - the Expo module behind getAudioProbe() is the one
// that does - but the lookup stays so both builds read the same code.
// Optional chaining on NativeModules (not just `.MediaCaps`) also tolerates
// the minimal test double in src/test/__mocks__/react-native.ts.
const native: MediaCapsNative | undefined =
  Platform.OS === "android"
    ? ((NativeModules as Record<string, unknown> | undefined)?.MediaCaps as
        | MediaCapsNative
        | undefined)
    : undefined;

let current: string[] = BASELINE;
let playerCodecs: string[] = [];

/**
 * The audio codecs this device actually decodes (or passes through over
 * HDMI), for the play-session's supportedAudioCodecs declaration. Synchronous:
 * returns the baseline until the one-shot native probe (kicked at module
 * load) lands, which is long before the user reaches a player screen. Worst
 * case of the tiny race: one session transcodes audio it could have
 * direct-played.
 */
export function getDeviceAudioCodecs(): string[] {
  return current;
}

/**
 * The codecs this app's own player decodes (ac3go), [] until the probe lands
 * or when the build has no such decoder. It also downmixes them itself, from
 * the stream's own mix levels.
 */
export function getPlayerAudioCodecs(): string[] {
  return playerCodecs;
}

// Audio codecs this device accepts as a RAW passthrough inside the server's
// fMP4 segments, on top of the always-wired AAC/AC-3/E-AC-3. A DIFFERENT
// question from getDeviceAudioCodecs(): every platform here decodes MP3, but
// only media3/ExoPlayer reads it back out of an fMP4 `mp4a` sample entry.
// AVPlayer does not take MP3 in that container at all, so iOS and tvOS
// declare nothing and keep the server-side AAC re-encode.
//
// Why it is worth asking for: re-encoding an already-lossy track cascades.
// Measured on a 128 kbps MP3 DVD rip (2026-09-06), coding noise under the
// signal at the server's 192k rendition was 30.6 dB on dialogue but only
// 20.3 dB on dense music - which is what "the background is not on the same
// plane as the voices" sounds like. A passthrough adds none of it.
const FMP4_AUDIO_PASSTHROUGH = Platform.OS === "android" ? ["mp3"] : [];

/**
 * The audio codecs this device takes muxed in fMP4, for the play session's
 * fmp4AudioPassthrough declaration (proto field + `?fmp4AudioPassthrough=` on
 * the HLS master). Static: it depends on the player engine, not on the device.
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
 * goes through untouched (Apple keeps its Dolby, Android has none).
 *
 * For the Dolby/DTS family the SINK decides, not the decoder: a decoder that
 * renders choppy (measured on a tablet, 2026-08-31) must not be declared, and
 * a passthrough box without any decoder must be - ExoPlayer hands the
 * bitstream to the receiver. See declarableAudioCodecs in @evey/media-caps.
 * playerDecoders (ac3go in the phone's player) are declared in every case.
 */
export function resolveAudioCodecs(
  baseline: string[],
  outcome: AudioProbeOutcome,
  probe: AudioProbe | null,
  playerDecoders: string[] = [],
): string[] {
  const legacy = outcome.status === "ok" && outcome.codecs.length > 0 ? outcome.codecs : null;
  const decoders = legacy ?? (probe && probe.decoders.length > 0 ? probe.decoders : null);
  if (!decoders) {
    const extra = playerDecoders.filter((c) => !baseline.includes(c));
    return extra.length > 0 ? [...baseline, ...extra] : baseline;
  }
  return declarableAudioCodecs(decoders, probe, playerDecoders);
}

let hdrSupported = false;

/**
 * Whether the display rendered HDR10/HLG natively AT STARTUP. False until the
 * native probe lands (and always false without the MediaCaps module) - the
 * safe default, the server then tonemaps to SDR like before. Dolby Vision
 * alone doesn't count: the server treats a bare SupportsHDR as HDR10-capable
 * and DoVi sources always tonemap server-side anyway.
 *
 * A display changes under a running app (HDMI re-plugged, AVR powered on):
 * probeDisplayHdr in ./displayHdr asks again at every negotiation and is what
 * a player should read. This memoised answer stays for the phone, whose
 * panel does not change.
 */
export function getDeviceHdrSupport(): boolean {
  return hdrSupported;
}

let maxH264Level: number | undefined;

/**
 * Highest H.264 level_idc the hardware decoder takes (41 = 4.1), for the
 * `?maxH264Level=` cap. Undefined until the probe lands or when the device
 * cannot answer: the server then stays level-blind, exactly as before.
 */
export function getDeviceMaxH264Level(): number | undefined {
  return maxH264Level;
}

async function probe(): Promise<void> {
  if (Platform.OS === "android") {
    try {
      maxH264Level = (await getMaxH264Level()) ?? undefined;
    } catch {
      /* stay level-blind */
    }
  }
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
  const player = Platform.OS === "android" ? await getPlayerAudioDecoders() : [];
  playerCodecs = player;
  current = resolveAudioCodecs(BASELINE, outcome, sink, player);
  // Single log line so a "silent audio" report can be diagnosed from device
  // logs without guessing what this device actually detected and declared.
  log.info(`audio probe=${outcome.status} sink=${sink ? "ok" : "none"} player=${player.join(",") || "none"} declared=${current.join(",")}`);

  // The shared Expo probe first, the legacy old-bridge module as a fallback:
  // the rule for what a display list means (HDR10 family yes, Dolby Vision
  // alone no) lives ONCE, in @evey/media-caps.
  try {
    const shared = await getDisplayHdrTypes();
    if (shared) {
      hdrSupported = displaySupportsHdr(shared);
      return;
    }
  } catch {
    /* fall through to the legacy module */
  }
  if (!native?.getHdrTypes) return;
  try {
    hdrSupported = displaySupportsHdr(await native.getHdrTypes());
  } catch {
    /* stay SDR */
  }
}

void probe();
