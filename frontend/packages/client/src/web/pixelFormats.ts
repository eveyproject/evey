import { getBrowserCapabilities } from "./codec";
import { prefersNativeHLS } from "./hlsCodecParams";

// The pixel formats this browser decodes smoothly beyond the server's built-in
// rule, declared as ?pixelFormats= on the master and supportedPixelFormats on
// the verdict.
//
// Undeclared, the server assumes 4:2:0 (8-bit for H.264, up to 10-bit for
// HEVC, VP9 and AV1) and re-encodes anything else. Chrome decodes a VP9
// profile 3 (4:4:4, 10-bit) in software at 1080p24 without dropping a frame,
// yet such a film was re-encoded for every viewer. So the browser is asked,
// through MediaCapabilities, for the layouts past that rule, and only what it
// calls smooth is declared: a laptop too weak for a software 4:4:4 decode
// keeps the server's encode.
//
// Once declared, the server reads ONLY the list, so it restates the built-in
// rule for every codec the browser plays before adding the extras. Nothing
// past the rule decodes smoothly: nothing is declared and the URL stays as it
// was.

interface Candidate {
  codec: string;
  family: string;
  contentType: string;
}

// The fMP4 codec strings the copy rung advertises for each layout (RFC 6381
// for VP9 and AV1: profile, level, bit depth, chroma subsampling 03 = 4:4:4).
const CANDIDATES: Candidate[] = [
  { codec: "vp9", family: "yuv444p", contentType: 'video/mp4; codecs="vp09.01.40.08.03"' },
  { codec: "vp9", family: "yuv444p10", contentType: 'video/mp4; codecs="vp09.03.40.10.03"' },
  { codec: "av1", family: "yuv444p", contentType: 'video/mp4; codecs="av01.1.08M.08.0.000"' },
  { codec: "av1", family: "yuv444p10", contentType: 'video/mp4; codecs="av01.1.08M.10.0.000"' },
  { codec: "h264", family: "yuv420p10", contentType: 'video/mp4; codecs="avc1.6E0028"' },
];

type DecodingInfo = (config: MediaDecodingConfiguration) => Promise<{ supported: boolean; smooth: boolean }>;

/** The server's built-in rule restated, for the codecs this browser plays. */
export function baselinePixelFormats(videoCodecs: readonly string[]): string[] {
  const out: string[] = [];
  for (const codec of videoCodecs) {
    out.push(`${codec}:yuv420p`);
    if (codec !== "h264") out.push(`${codec}:yuv420p10`);
  }
  return out;
}

/**
 * The declaration for these codecs given a decodingInfo: undefined when
 * nothing past the built-in rule decodes smoothly.
 */
export async function detectPixelFormats(videoCodecs: readonly string[], decodingInfo: DecodingInfo): Promise<string[] | undefined> {
  const extras: string[] = [];
  for (const c of CANDIDATES) {
    if (!videoCodecs.includes(c.codec)) continue;
    try {
      const info = await decodingInfo({
        type: "media-source",
        video: { contentType: c.contentType, width: 1920, height: 1080, bitrate: 10_000_000, framerate: 24 },
      });
      if (info.supported && info.smooth) extras.push(`${c.codec}:${c.family}`);
    } catch {
      // An engine that cannot parse the string has no decoder for it.
    }
  }
  return extras.length > 0 ? [...baselinePixelFormats(videoCodecs), ...extras] : undefined;
}

let probed: Promise<string[] | undefined> | undefined;
let known: { value: string[] | undefined } | undefined;

/** Asks the browser once; later calls share the answer. */
export function probeBrowserPixelFormats(): Promise<string[] | undefined> {
  if (probed) return probed;
  const mc = typeof navigator !== "undefined" ? navigator.mediaCapabilities : undefined;
  // Safari plays our HLS through AVPlayer, whose decoders MediaCapabilities'
  // media-source answers do not describe.
  if (!mc?.decodingInfo || prefersNativeHLS()) {
    known = { value: undefined };
    return (probed = Promise.resolve(undefined));
  }
  probed = detectPixelFormats(getBrowserCapabilities().videoCodecs, (config) => mc.decodingInfo(config)).then(
    (value) => {
      known = { value };
      return value;
    },
    () => {
      known = { value: undefined };
      return undefined;
    },
  );
  return probed;
}

/** The answer once the probe has settled; undefined before, or when there is nothing to declare. */
export function browserPixelFormats(): string[] | undefined {
  return known?.value;
}

/** Whether the probe has settled. */
export function browserPixelFormatsKnown(): boolean {
  return known !== undefined;
}

/** Test hook: forget the probe. */
export function resetBrowserPixelFormatsForTest(): void {
  probed = undefined;
  known = undefined;
}
