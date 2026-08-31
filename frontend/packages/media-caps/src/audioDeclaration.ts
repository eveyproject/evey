import type { AudioProbe } from "./audioProbe";

// Which audio codecs the app may DECLARE to the server, from the raw probe.
//
// The declaration used to come from MediaCodecList alone: "do I have a
// DECODER". That is the wrong question twice over. A tablet can carry an
// AC-3/E-AC-3 decoder whose decode-and-downmix path is unusable - measured
// live (2026-07-30 and again 2026-08-31, a tablet brand 23043RP34G): direct-played
// AC-3 renders choppy by stretches, while the sink's own answer
// (isDirectPlaybackSupported) says plainly that it does NOT take those
// formats. And a TV box can have NO decoder yet pass the bitstream untouched
// to a receiver that renders it - ExoPlayer plays that fine.
//
// So for the Dolby/DTS family the SINK decides, not the decoder:
//   - directPlayback[codec] (API 29+, AudioManager.isDirectPlaybackSupported)
//   - or the codec's encoding among directProfiles (API 33+)
// No evidence (probe missing, API < 29) means no declaration: the cost is a
// server-side audio-only transcode (video stays copy), the alternative is
// unwatchable sound. Everything outside the family (aac/mp3/opus/flac/...)
// keeps following the decoder list - those are AOSP software decoders that
// render anywhere.

const SINK_GATED_FAMILY = ["ac3", "eac3", "dts", "truehd"] as const;

// AudioFormat.ENCODING_* values for the family, to read directProfiles.
const FAMILY_ENCODINGS: Record<(typeof SINK_GATED_FAMILY)[number], number[]> = {
  ac3: [5], // ENCODING_AC3
  eac3: [6, 18], // ENCODING_E_AC3, ENCODING_E_AC3_JOC
  dts: [7, 8], // ENCODING_DTS, ENCODING_DTS_HD
  truehd: [14], // ENCODING_DOLBY_TRUEHD
};

export function sinkAcceptsCodec(probe: AudioProbe, codec: string): boolean {
  if (probe.directPlayback[codec] === true) return true;
  const encodings = FAMILY_ENCODINGS[codec as (typeof SINK_GATED_FAMILY)[number]];
  if (!encodings) return false;
  return probe.directProfiles.some((p) => encodings.includes(p.encoding));
}

/**
 * The audio codecs to declare, given the decoder list (MediaCodecList side)
 * and the raw output probe. Family codecs need the sink's yes - a decoder is
 * neither sufficient (a tablet brand decode-and-stutter) nor necessary (passthrough
 * box). A null probe (module absent, probe failed) declares none of the
 * family: better one audio transcode too many than a choppy film.
 */
export function declarableAudioCodecs(decoderCodecs: string[], probe: AudioProbe | null): string[] {
  const family = new Set<string>(SINK_GATED_FAMILY);
  const out = decoderCodecs.filter((c) => !family.has(c));
  if (!probe) return out;
  for (const codec of SINK_GATED_FAMILY) {
    if (sinkAcceptsCodec(probe, codec)) out.push(codec);
  }
  return out;
}
