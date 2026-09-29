import { getBrowserCapabilities } from "./codec";
import { getMaxOutputChannels } from "./outputChannels";

// computeNativeHLSPreference is the pure decision behind prefersNativeHLS,
// factored out so every browser can be exercised in a unit test without a real
// engine. Native HLS (the <video> element's own player) is preferred on Safari
// (macOS laptops + iOS) - lighter on CPU/battery than hls.js+MSE - and is the
// only option where MSE is missing (old iOS). Chrome/Firefox/Edge/Android stay
// on hls.js even when the OS can decode HLS, because that path is the mature one
// and carries our token refresh / CMCD / ABR control.
export function computeNativeHLSPreference(
  canPlayHLS: boolean,
  userAgent: string,
  platform: string,
  maxTouchPoints: number,
  mseSupported: boolean,
): boolean {
  if (!canPlayHLS) return false;
  // Safari, and only Safari, among the engines on Apple hardware: Chrome/Edge/
  // Firefox on macOS carry chrome/crios/fxios/edg tokens and must be excluded.
  const safari = /^((?!chrome|chromium|android|crios|fxios|edg).)*safari/i.test(userAgent);
  const ios =
    /iP(hone|ad|od)/.test(userAgent) ||
    // iPadOS 13+ masquerades as desktop Safari (MacIntel) but is touch-capable.
    (platform === "MacIntel" && maxTouchPoints > 1);
  return safari || ios || !mseSupported;
}

/**
 * Whether this browser could play HLS through hls.js (Media Source
 * Extensions), which is the one question `Hls.isSupported()` answers: a
 * MediaSource implementation, and a baseline H.264 + AAC fMP4 it can take.
 *
 * Asked HERE rather than by importing hls.js, because that import dragged
 * half a megabyte of JavaScript into the first page of every client - a
 * television parses it before it can show a home it may never play from. The
 * player still loads the real library, at the moment it attaches a stream.
 */
export function mseHlsSupported(): boolean {
  const mediaSource = (globalThis as { MediaSource?: { isTypeSupported?: (t: string) => boolean } }).MediaSource
    ?? (globalThis as { ManagedMediaSource?: { isTypeSupported?: (t: string) => boolean } }).ManagedMediaSource;
  if (!mediaSource?.isTypeSupported) return false;
  try {
    return mediaSource.isTypeSupported('video/mp4; codecs="avc1.42E01E,mp4a.40.2"');
  } catch {
    return false;
  }
}

// prefersNativeHLS reports whether this browser should play HLS through the
// <video> element's native engine instead of hls.js/MSE. Memoised: it reads a
// static capability (canPlayType) + UA.
let _prefersNativeHLS: boolean | undefined;
export function prefersNativeHLS(): boolean {
  if (_prefersNativeHLS !== undefined) return _prefersNativeHLS;
  if (typeof document === "undefined" || typeof navigator === "undefined") return false;
  const canPlayHLS =
    document.createElement("video").canPlayType("application/vnd.apple.mpegurl") !== "";
  _prefersNativeHLS = computeNativeHLSPreference(
    canPlayHLS,
    navigator.userAgent,
    navigator.platform,
    navigator.maxTouchPoints,
    mseHlsSupported(),
  );
  return _prefersNativeHLS;
}

// getFmp4AudioPassthrough reports the audio codecs this browser accepts as a
// RAW passthrough inside the server's fMP4 segments, beyond the always-wired
// AAC/AC-3/E-AC-3. Today that means one question: MP3.
//
// Why ask at all: without it the server re-encodes an MP3 source to AAC, and on
// an already-lossy track that cascade is audible. Measured on a 128 kbps MP3
// DVD rip (2026-09-06), coding noise under the signal at the 192k rendition was
// 30.6 dB on dialogue but 20.3 dB on dense music - "the background is not on
// the same plane as the voices". A passthrough adds none of it.
//
// Why PROBE rather than assume: a browser that cannot demux MP3 out of fMP4
// gets SILENCE, not degraded audio. getBrowserCapabilities().audioCodecs lists
// "mp3" unconditionally and always has - that answers "can you decode an MP3
// file", which is a different question from "can you take MP3 inside fMP4".
//
// Both conditions are required:
//   - the playback path must be hls.js/MSE. Safari and iOS run our HLS through
//     AVPlayer, which does not take MP3 in fMP4 at all, and canPlayType is not
//     a trustworthy proxy for what that engine accepts inside a segment.
//   - that MSE must claim the type itself. Same isTypeSupported gate, and the
//     same reasoning, as canPlayHevc: a capability the plain <video> element
//     advertises is worthless if the MSE path is the one doing the decoding.
//
// mp4a.6B is the RFC 6381 form of MP3 in an MP4 sample entry (esds
// objectTypeIndication 0x6B, MPEG-1 Audio) - read off the bytes ffmpeg writes
// for `-c:a copy` into fMP4, and what the server's master advertises.
let _fmp4AudioPassthrough: string[] | undefined;
export function getFmp4AudioPassthrough(): string[] {
  if (_fmp4AudioPassthrough !== undefined) return _fmp4AudioPassthrough;
  if (prefersNativeHLS()) return (_fmp4AudioPassthrough = []);
  const MS =
    typeof window !== "undefined"
      ? (window.MediaSource ??
        (window as unknown as { ManagedMediaSource?: typeof MediaSource }).ManagedMediaSource)
      : undefined;
  if (!MS || typeof MS.isTypeSupported !== "function") return (_fmp4AudioPassthrough = []);
  const out: string[] = [];
  if (MS.isTypeSupported('audio/mp4; codecs="mp4a.6B"')) out.push("mp3");
  // Opus muxed in fMP4 (the Opus-in-ISOBMFF binding): Chrome and Firefox take
  // it, Safari does not - so it is declared, never assumed, like mp3.
  if (MS.isTypeSupported('audio/mp4; codecs="opus"')) out.push("opus");
  return (_fmp4AudioPassthrough = out);
}

export function appendCodecParams(params: URLSearchParams): void {
  const caps = getBrowserCapabilities();
  if (caps.videoCodecs.length > 0) params.set("videoCodecs", caps.videoCodecs.join(","));
  // The short side of the physical screen: the tallest rung worth encoding
  // for this viewer (a 2160p rung for a 768 px laptop is bandwidth for nothing).
  const screen = typeof window !== "undefined" ? window.screen : undefined;
  if (screen && screen.width > 0 && screen.height > 0) {
    const dpr = window.devicePixelRatio || 1;
    params.set("screenHeight", String(Math.round(Math.min(screen.width, screen.height) * dpr)));
  }
  if (caps.audioCodecs.length > 0) params.set("audioCodecs", caps.audioCodecs.join(","));
  if (caps.containers.length > 0) params.set("containers", caps.containers.join(","));
  // What this browser takes muxed in fMP4, NOT what it decodes - see
  // getFmp4AudioPassthrough. Lets the server COPY an MP3 track (old DVD rips)
  // instead of cascading it through its AAC encoder. Omitted when empty so the
  // URL of a browser that cannot take it stays byte-identical to before.
  const passthrough = getFmp4AudioPassthrough();
  if (passthrough.length > 0) params.set("fmp4AudioPassthrough", passthrough.join(","));
  // HDR display capability: without it the server decision is HDR-blind and
  // forces a tonemap transcode on every HDR title - even for clients that
  // decode HEVC+HDR10 natively (the copy rung). Only sent when true: the
  // server default is SDR (conservative, mirrors the gRPC supportsHdr param).
  if (caps.hdr) params.set("hdr", "1");
  // Channel ceiling of the OUTPUT, not of the decoder - see outputChannels.ts.
  // Omitted when the browser cannot answer, which leaves the server profile's
  // own default in place: an unknown output must not silently lose surround.
  const channels = getMaxOutputChannels();
  if (channels !== undefined) params.set("maxAudioChannels", String(channels));
}
