import { NativeModules, Platform } from "react-native";
import { declarableAudioCodecs, displaySupportsHdr, getAudioProbe, getDisplayHdrTypes, getMaxH264Level } from "@evey/media-caps";

interface MediaCapsNative {
  getSupportedAudioCodecs(): Promise<string[]>;
  getHdrTypes(): Promise<string[]>;
}

// Every Android device decodes these in software (AOSP bundled decoders) -
// the safe floor when the native MediaCaps module isn't in the installed build
// (Expo Go, older APK). Deliberately NO ac3/eac3/dts/truehd: lots of devices
// ship without Dolby/DTS decoders and a direct-played track the device can't
// decode plays the video SILENT. Declaring less only costs a server-side
// audio-only transcode (video stays copy).
const ANDROID_BASELINE = ["aac", "mp3", "opus", "flac", "vorbis"];

// AVPlayer decodes AC-3/E-AC-3 on every iOS/tvOS device we support; it can't
// demux mkv anyway so the exotic codecs never reach it outside mp4/hls.
const IOS_CODECS = ["aac", "mp3", "flac", "ac3", "eac3"];

const native: MediaCapsNative | undefined =
  Platform.OS === "android"
    ? (NativeModules.MediaCaps as MediaCapsNative | undefined)
    : undefined;

let current: string[] = Platform.OS === "android" ? ANDROID_BASELINE : IOS_CODECS;

/**
 * The audio codecs this device actually decodes, for the play-session's
 * supportedAudioCodecs declaration. Synchronous: returns the baseline until the
 * one-shot native probe (kicked at module load) lands, which is long before the
 * user reaches a player screen. Worst case of the tiny race: one session
 * transcodes audio it could have direct-played.
 */
export function getDeviceAudioCodecs(): string[] {
  return current;
}

// Audio codecs this device accepts as a RAW passthrough inside the server's
// fMP4 segments, on top of the always-wired AAC/AC-3/E-AC-3. A DIFFERENT
// question from getDeviceAudioCodecs(): every platform here decodes MP3, but
// only media3/ExoPlayer reads it back out of an fMP4 `mp4a` sample entry.
// AVPlayer does not take MP3 in that container at all, so iOS declares nothing
// and keeps the server-side AAC re-encode.
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

let hdrSupported = false;

/**
 * Whether the display renders HDR10/HLG natively. False until the native probe
 * lands (and always false without the MediaCaps module) - the safe default,
 * the server then tonemaps to SDR like before. Dolby Vision alone doesn't
 * count: the server treats a bare SupportsHDR as HDR10-capable and DoVi
 * sources always tonemap server-side anyway.
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
  if (!native) return;
  try {
    const codecs = await native.getSupportedAudioCodecs();
    // The decoder list alone answers the wrong question for the Dolby/DTS
    // family: a a tablet brand tablet DECODES ac3/eac3 but renders them choppy, while
    // its sink says plainly it does not take those formats (audio_caps_probe,
    // 2026-08-31 - same story on 2026-07-30). The SINK decides for that
    // family; see declarableAudioCodecs. Without probe evidence the family
    // stays out, and the server transcodes the audio only (video stays copy).
    const audioProbe = await getAudioProbe();
    if (Array.isArray(codecs) && codecs.length > 0) {
      current = declarableAudioCodecs(codecs, audioProbe);
    }
  } catch {
    /* keep the baseline */
  }
  // The shared Expo probe first, the legacy old-bridge module as a fallback:
  // the rule for what a display list means (HDR10 family yes, Dolby Vision
  // alone no) now lives ONCE, in @evey/media-caps, and the TV reads the very
  // same one. It used to be this inline expression, which also silently missed
  // hdr10plus-only panels.
  try {
    const shared = await getDisplayHdrTypes();
    if (shared) {
      hdrSupported = displaySupportsHdr(shared);
      return;
    }
  } catch {
    /* fall through to the legacy module */
  }
  try {
    hdrSupported = displaySupportsHdr(await native.getHdrTypes());
  } catch {
    /* stay SDR */
  }
}

void probe();
