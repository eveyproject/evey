// canPlayHevc reports whether this browser can actually decode HEVC on the
// streaming (MSE) path. Lazily memoized ON PURPOSE, not computed at module load:
// the MediaSource.isTypeSupported probe below makes Firefox log "no decoder for
// video/mp4; codecs=hvc1/hev1" to the console every time it runs on a machine
// whose OS advertises HEVC on the plain <video> element while its MSE cannot
// decode it. This module is pulled into the initial bundle (via the eagerly
// imported /watch route → HLSPlayer), so an eager IIFE fired that log on EVERY
// page at startup - home included - with no video present. Deferring the probe
// to the first call (only reached when a playback session starts, via
// getBrowserCapabilities) keeps it and its console log off non-playback views.
let hevcSupport: boolean | undefined;
export function canPlayHevc(): boolean {
  if (hevcSupport !== undefined) return hevcSupport;
  const v = document.createElement("video");
  // "maybe" is unreliable (Firefox returns it without actual HEVC support), so
  // require "probably" on the plain element first.
  const byElement =
    v.canPlayType('video/mp4; codecs="hvc1"') === "probably" ||
    v.canPlayType('video/mp4; codecs="hev1"') === "probably";
  if (!byElement) return (hevcSupport = false);
  // The streaming path is hls.js → Media Source Extensions. Firefox on Windows
  // reports HEVC "probably" on the plain <video> element (via the OS decoder)
  // while its MSE cannot decode HEVC - advertising hevc then makes the server
  // direct-play a stream MSE chokes on ("no decoder for video/mp4; codecs=hvc1").
  // Gate on isTypeSupported so we only claim HEVC the actual (MSE) path can do.
  // No MSE at all (native/progressive path) → keep the element result.
  const MS =
    typeof window !== "undefined"
      ? (window.MediaSource ??
        (window as unknown as { ManagedMediaSource?: typeof MediaSource }).ManagedMediaSource)
      : undefined;
  if (!MS || typeof MS.isTypeSupported !== "function") return (hevcSupport = true);
  return (hevcSupport =
    MS.isTypeSupported('video/mp4; codecs="hvc1"') ||
    MS.isTypeSupported('video/mp4; codecs="hev1"'));
}

// canDisplayHdr is true only when the display reports an HDR-capable dynamic
// range. Conservative on purpose: a false negative just transcodes/tonemaps
// (a perf cost), while a false positive would direct-play raw HDR to an SDR
// panel and render it washed-out. Dolby Vision is still transcoded server-side
// regardless (this bool can't tell DoVi-capable from HDR10-only).
export const canDisplayHdr = (() => {
  try {
    return typeof window !== "undefined"
      && typeof window.matchMedia === "function"
      && window.matchMedia("(dynamic-range: high)").matches;
  } catch {
    return false;
  }
})();

// canWasmDecodeAC3 reports whether this browser should decode AC-3/E-AC-3 in
// WebAssembly (the ac3go decoder) rather than natively. True only when the
// browser CANNOT play those codecs itself AND can run WebAssembly: Chrome,
// Firefox and Edge (which gate Dolby decoding off by default) get the wasm path;
// Safari, which decodes AC-3 natively, keeps the native path. Deliberately kept
// OUT of getBrowserCapabilities().audioCodecs - advertising ac3/eac3 there would
// make the server COPY the audio into the MUXED stream, which MSE then rejects.
// The wasm path is signalled by a separate `?wasmAudio=1` flag instead, so the
// server serves it as an out-of-band rendition the wasm engine decodes.
let _wasmAC3: boolean | undefined;
export function canWasmDecodeAC3(): boolean {
  if (_wasmAC3 !== undefined) return _wasmAC3;
  if (typeof window === "undefined" || typeof WebAssembly === "undefined") {
    return (_wasmAC3 = false);
  }
  const v = document.createElement("video");
  // "" means "cannot play" - both the plain element and MSE must lack it, or we
  // keep the cheaper native path.
  const nativeAC3 =
    v.canPlayType('audio/mp4; codecs="ac-3"') !== "" ||
    v.canPlayType('video/mp4; codecs="ac-3"') !== "" ||
    v.canPlayType('audio/mp4; codecs="ec-3"') !== "" ||
    v.canPlayType('video/mp4; codecs="ec-3"') !== "";
  return (_wasmAC3 = !nativeAC3);
}

// isWasmDecodableAudioCodec reports whether a source audio codec is one the
// ac3go wasm decoder handles (AC-3 / E-AC-3, in every spelling the prober and
// RFC/MIME forms use).
export function isWasmDecodableAudioCodec(codec: string | undefined): boolean {
  switch ((codec ?? "").toLowerCase()) {
    case "ac3":
    case "ac-3":
    case "eac3":
    case "e-ac-3":
    case "ec-3":
      return true;
    default:
      return false;
  }
}

export interface BrowserCapabilities {
  videoCodecs: string[];
  audioCodecs: string[];
  containers: string[];
  hdr: boolean;
}

export function getBrowserCapabilities(): BrowserCapabilities {
  const v = document.createElement("video");
  const canPlay = (type: string) => v.canPlayType(type) !== "";

  const videoCodecs: string[] = [];
  if (canPlay('video/mp4; codecs="avc1.42E01E"')) videoCodecs.push("h264");
  if (canPlayHevc()) videoCodecs.push("hevc");
  if (canPlay('video/webm; codecs="vp9"')) videoCodecs.push("vp9");
  if (canPlay('video/mp4; codecs="av01.0.00M.08"')) videoCodecs.push("av1");

  const audioCodecs: string[] = ["aac", "mp3", "opus"];
  if (canPlay('audio/mp4; codecs="ac-3"') || canPlay('video/mp4; codecs="ac-3"')) audioCodecs.push("ac3");
  if (canPlay('audio/mp4; codecs="ec-3"') || canPlay('video/mp4; codecs="ec-3"')) audioCodecs.push("eac3");
  if (canPlay("audio/flac")) audioCodecs.push("flac");

  const containers: string[] = [];
  if (canPlay("video/mp4")) containers.push("mp4");
  if (canPlay("video/webm")) containers.push("webm");
  if (canPlay("video/ogg")) containers.push("ogg");

  return { videoCodecs, audioCodecs, containers, hdr: canDisplayHdr };
}
