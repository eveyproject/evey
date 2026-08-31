import { NativeModules, Platform } from "react-native";
import { declarableAudioCodecs, getAudioProbe } from "@evey/media-caps";

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

async function probe(): Promise<void> {
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
  try {
    const hdr = await native.getHdrTypes();
    hdrSupported = Array.isArray(hdr) && (hdr.includes("hdr10") || hdr.includes("hlg"));
  } catch {
    /* stay SDR */
  }
}

void probe();
