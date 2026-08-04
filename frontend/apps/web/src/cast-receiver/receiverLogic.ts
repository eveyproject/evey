// PURE logic for the custom Cast Web Receiver: the receiver runs ON the TV,
// so it can probe its own capabilities (canDisplayType) instead of trusting a
// static per-model table on the sender side - the Mi TV table lied once
// (E-AC-3 "Dolby certified" that the cast runtime does not actually play,
// prod 2026-07-25); a local probe cannot.
//
// Audio policy, aligned with Jellyfin's evidence (their receiver hardcodes
// EAC3/AC3 to false on HLS) AND our incident: Dolby passthrough is only
// attempted if the device itself probes true; otherwise audio is either
// transcoded AAC by the server, or decoded ON THE TV by ac3go wasm
// (wasmAudio=1 - the server only takes this path if the track is AC-3/E-AC-3,
// otherwise it falls back on its own to the muxed AAC master).

export type CanDisplayType = (
  mimeType: string,
  codec?: string,
  width?: number,
  height?: number,
) => boolean;

export interface ReceiverCaps {
  videoCodecs: string[];
  maxHeight: number;
  hdr: boolean;
  nativeDolby: boolean;
  maxAudioChannels: number;
}

// AAC-LC, the codec the server transcodes to. Stereo and 5.1 differ only by
// the channel count, which is why the probe below varies nothing else.
const AAC_LC = 'audio/mp4; codecs="mp4a.40.2"';

// The two device answers that actually exist for audio channels. canDisplayType
// has no channel argument - that is a limit of THAT call, not of the platform,
// and reading it as one is what shipped a blind 5.1 assumption.
export interface AudioProbe {
  // AudioContext.destination.maxChannelCount: how many channels the output
  // chain (HDMI/eARC sink) accepts. Undefined when there is no AudioContext.
  maxChannelCount?: number;
  // navigator.mediaCapabilities.decodingInfo: the ONE standard API that takes
  // a channel count and answers whether this device decodes it.
  decodingInfo?: (config: unknown) => Promise<{ supported?: boolean } | undefined>;
}

// probeMaxAudioChannels asks the device what it can do, instead of assuming.
//
// Only a POSITIVE answer raises the ceiling: a decoder that says it handles
// 5.1 keeps 5.1 (it downmixes to whatever the sink is - that is the browser's
// job, not ours), everything else is served stereo. Never the reverse: a
// stereo output must not demote a decoder that works, or fixing one TV costs
// the surround of another.
export async function probeMaxAudioChannels(probe: AudioProbe): Promise<number> {
  if (probe.decodingInfo) {
    try {
      const info = await probe.decodingInfo({
        type: "media-source",
        audio: { contentType: AAC_LC, channels: "6", bitrate: 384_000, samplerate: 48_000 },
      });
      if (info?.supported === true) return 6;
      // An explicit "no" is the Chromecast HD of 2026-08-04: its SourceBuffer
      // refuses 5.1 AAC and the title never starts. Believe it.
      return 2;
    } catch {
      // Malformed config or an API that throws: fall through to the sink.
    }
  }
  // No decodingInfo (old cast runtime): a sink wired for 6+ channels is still
  // a positive surround signal. Anything else stays stereo.
  return (probe.maxChannelCount ?? 0) >= 6 ? 6 : 2;
}

// Probe codec strings - same shapes as jellyfin-chromecast's
// codecSupportHelper: h264 High L4.0, HEVC Main10 L5.1, VP9 profile 2 10-bit.
const PROBE_H264 = ["video/mp4", "avc1.640028"] as const;
const PROBE_HEVC = ["video/mp4", "hev1.2.4.L153.B0"] as const;
const PROBE_VP9 = ["video/webm", "vp09.02.51.10"] as const;
// HDR10: HEVC Main10 + PQ transfer (SMPTE ST 2084) via the long-form vp09,
// the only canDisplayType syntax that expresses an HDR transfer without Dolby Vision.
const PROBE_HDR = ["video/webm", "vp09.02.51.10.01.09.16.09.01"] as const;

// maxAudioChannels comes from probeMaxAudioChannels, which is async and so
// cannot run here; it defaults to stereo, the value that plays everywhere.
export function probeReceiverCaps(
  canDisplayType: CanDisplayType,
  systemHdr: boolean,
  maxAudioChannels = 2,
): ReceiverCaps {
  const videoCodecs: string[] = [];
  // h264 is the universal receiver floor: present even if the High profile
  // probe fails (a receiver without h264 does not exist).
  videoCodecs.push("h264");
  if (canDisplayType(PROBE_HEVC[0], PROBE_HEVC[1])) videoCodecs.push("hevc");
  if (canDisplayType(PROBE_VP9[0], PROBE_VP9[1])) videoCodecs.push("vp9");

  // 4K probed on the most capable codec available; a failure means a 1080p ceiling.
  const fourKProbe = videoCodecs.includes("hevc") ? PROBE_HEVC : PROBE_H264;
  const maxHeight = canDisplayType(fourKProbe[0], fourKProbe[1], 3840, 2160) ? 2160 : 1080;

  // HDR: the system signal (DeviceCapabilities.IS_HDR_SUPPORTED, wired by
  // the caller) OR a positive PQ transfer probe.
  const hdr = systemHdr || canDisplayType(PROBE_HDR[0], PROBE_HDR[1]);

  // Dolby passthrough: only if the device itself probes true (EDID /
  // Google Home setting). This is the probe Jellyfin wrote then commented out;
  // our own fallback if it lies is the reload without wasm/dolby on the receiver side.
  const nativeDolby =
    canDisplayType("audio/mp4", "ec-3") && canDisplayType("audio/mp4", "ac-3");

  return { videoCodecs, maxHeight, hdr, nativeDolby, maxAudioChannels };
}

export interface RewrittenLoad {
  url: string;
  wasmAudio: boolean;
}

const BASE_AUDIO = ["aac", "mp3", "opus", "flac"];

// rewriteCastMasterUrl renegotiates OUR HLS master with the actually probed
// caps. Any other URL (direct play /stream, third-party content) is left
// as-is - returns null.
//
// pageProtocol = the scheme of the receiver PAGE (https: in prod, Cast
// Console requirement). A self-hosted Evey instance on http: on its LAN
// stays playable: the CAF player is allowed to load http: media from an
// https: page (Google Cast doc), but OUR JS fetches (wasm master, ac3go
// segments) are active mixed content and would be blocked - so never
// wasmAudio toward an http: server.
export function rewriteCastMasterUrl(
  rawUrl: string,
  caps: ReceiverCaps,
  wasmCapable: boolean,
  pageProtocol = "https:",
): RewrittenLoad | null {
  let u: URL;
  try {
    u = new URL(rawUrl);
  } catch {
    return null;
  }
  if (!u.pathname.endsWith("/hls/master.m3u8")) return null;

  u.searchParams.set("videoCodecs", caps.videoCodecs.join(","));
  u.searchParams.set("containers", "mp4");
  u.searchParams.set("maxHeight", String(caps.maxHeight));
  if (caps.hdr) u.searchParams.set("hdr", "1");
  else u.searchParams.delete("hdr");

  const mixedContent = pageProtocol === "https:" && u.protocol === "http:";
  // A floored master (abr=floor, the starvation rescue) is survival mode: one
  // muxed low rung, never the wasm engine on top of an already-starving link.
  const floored = u.searchParams.get("abr") === "floor";
  const wasmAudio = !floored && !caps.nativeDolby && wasmCapable && !mixedContent;
  const audio = caps.nativeDolby ? [...BASE_AUDIO, "eac3", "ac3"] : BASE_AUDIO;
  u.searchParams.set("audioCodecs", audio.join(","));
  // Channel ceiling for the server's AAC transcode, declared ALWAYS: left out,
  // the server applies its 5.1 web default, emits `-ac 6`, and a receiver that
  // cannot open multichannel AAC refuses the stream in its SourceBuffer - the
  // title then never starts at all. That is the Chromecast HD of 2026-08-04:
  // name on screen, spinner for ever, `cast_receiver_finished reason=ERROR`
  // 200ms in, while Jellyfin played the same file on the same TV with `-ac 2`.
  u.searchParams.set("maxAudioChannels", String(caps.maxAudioChannels));
  if (wasmAudio) u.searchParams.set("wasmAudio", "1");
  else u.searchParams.delete("wasmAudio");

  return { url: u.toString(), wasmAudio };
}

// absoluteWithToken resolves a playlist/segment URL against the MEDIA SERVER
// (the absolute base URL) and injects the freshest token into it. Never
// against the page's origin: the receiver may be hosted elsewhere than the
// instance (GitHub Pages, project domain) - the web's injectToken (same-origin)
// would produce a relative path that goes to the receiver's host and 404s.
export function absoluteWithToken(url: string, baseUrl: string, token: string): string {
  try {
    const abs = new URL(url, baseUrl);
    if (token) abs.searchParams.set("token", token);
    return abs.toString();
  } catch {
    return url;
  }
}

// createStartGate synchronises the visible START of a wasm-audio cast: the
// video-only master gives CAF nothing to wait for, so the picture used to
// start 1-2s before the audio engine had its first decoded segment - a silent
// start the muxed-AAC master never had (CAF holds until BOTH tracks are
// ready). The gate holds autoplay until the engine reports its first decoded
// audio, with a hard timeout so a broken audio path can never hold the
// picture hostage. Pure state machine, injected timers, exported for tests.
export interface StartGate {
  // arm is called at LOAD interception (autoplay was stripped): starts the
  // safety timeout.
  arm(): void;
  // audioReady releases the gate (first audio decoded).
  audioReady(): void;
  // skip releases immediately (the server did not take the wasm path, or the
  // engine cannot run): nothing to wait for.
  skip(): void;
  // cancel disarms without playing (a NEW load superseded this one - its own
  // request carries its own autoplay).
  cancel(): void;
}

export function createStartGate(
  play: () => void,
  timeoutMs: number,
  setTimer: (fn: () => void, ms: number) => ReturnType<typeof setTimeout> = setTimeout,
  clearTimer: (t: ReturnType<typeof setTimeout>) => void = clearTimeout,
): StartGate {
  let timer: ReturnType<typeof setTimeout> | null = null;
  let released = false;
  const release = () => {
    if (released) return;
    released = true;
    if (timer !== null) clearTimer(timer);
    timer = null;
    play();
  };
  return {
    arm() {
      released = false;
      if (timer !== null) clearTimer(timer);
      timer = setTimer(release, timeoutMs);
    },
    audioReady: release,
    skip: release,
    cancel() {
      released = true;
      if (timer !== null) clearTimer(timer);
      timer = null;
    },
  };
}

// buildPlayerEventUrl derives the telemetry endpoint from the casted stream
// URL: same origin, same media id, same stream token (the receiver's only
// identity - the server pins that token to its own media). Null for
// third-party or tokenless URLs: no door, no beacon. Motivated 2026-07-26: a
// TV-side cast death 3-5 min in left the server logs completely blank.
export function buildPlayerEventUrl(streamUrl: string): string | null {
  try {
    const u = new URL(streamUrl);
    const m = /^(.*\/api\/v1\/media\/[^/]+)\//.exec(u.pathname + "/");
    const token = u.searchParams.get("token");
    if (!m || !token) return null;
    return `${u.origin}${m[1]}/player-event?token=${encodeURIComponent(token)}`;
  } catch {
    return null;
  }
}

// stripWasmAudio is the fallback when ac3go does not start on the TV:
// reloading the SAME master without wasmAudio re-serves the muxed AAC audio.
export function stripWasmAudio(rawUrl: string): string {
  try {
    const u = new URL(rawUrl);
    u.searchParams.delete("wasmAudio");
    return u.toString();
  } catch {
    return rawUrl;
  }
}

// floorMasterUrl derives the survival reload of a starving playback: the same
// master with ?abr=floor, which the server answers with a mono-variant master
// holding only its lowest transcoded rung (an old server ignores the param -
// the reload alone still lets CAF re-pick a variant with its collapsed
// bandwidth estimate). Null when there is nothing lower to offer: already
// floored (the latch - a link that cannot carry the floor rung has no next
// step here), or not a URL we understand.
export function floorMasterUrl(rawUrl: string): string | null {
  try {
    const u = new URL(rawUrl);
    if (u.searchParams.get("abr") === "floor") return null;
    u.searchParams.set("abr", "floor");
    u.searchParams.delete("wasmAudio");
    return u.toString();
  } catch {
    return null;
  }
}

// stereoFallbackUrl is the net for a probe that LIED - and one already did:
// the Mi TV probed itself Dolby-capable through EDID and the CAF player still
// failed the load (2026-07-25). A device that answered "yes, 5.1" and then
// refuses the stream gets one reload with the ceiling pinned to stereo.
// Null when it is already stereo: the latch, so a title failing for any other
// reason cannot loop here.
export function stereoFallbackUrl(rawUrl: string): string | null {
  try {
    const u = new URL(rawUrl);
    if (u.searchParams.get("maxAudioChannels") === "2") return null;
    u.searchParams.set("maxAudioChannels", "2");
    return u.toString();
  } catch {
    return null;
  }
}

// bitrateTestUrl derives the /hls/bitrate-test endpoint from the master URL:
// same origin, same media, same stream token - the receiver times its OWN
// network stack (old cast runtimes cap far under the wire speed, and the
// measure captures that too). Null for tokenless or foreign URLs: no measure,
// today's behaviour.
export function bitrateTestUrl(masterUrl: string): string | null {
  try {
    const u = new URL(masterUrl);
    const m = /^(.*\/api\/v1\/media\/[^/]+\/hls)\/master\.m3u8$/.exec(u.pathname);
    const token = u.searchParams.get("token");
    if (!m || !token) return null;
    return `${u.origin}${m[1]}/bitrate-test?token=${encodeURIComponent(token)}`;
  } catch {
    return null;
  }
}

// appendMaxKbps hands the measured verdict to the server, which picks the
// variant SERVER-SIDE (copy-first, then the highest fitting rung): the
// receiver's own ABR is never relied upon.
export function appendMaxKbps(masterUrl: string, kbps: number): string | null {
  if (!Number.isFinite(kbps) || kbps <= 0) return null;
  try {
    const u = new URL(masterUrl);
    u.searchParams.set("maxKbps", String(Math.round(kbps)));
    return u.toString();
  } catch {
    return null;
  }
}

// shouldMeasure: our masters only, once per playback - a reload that already
// carries a verdict (maxKbps) or a rescue (abr=floor) is never re-measured.
export function shouldMeasure(rawUrl: string): boolean {
  try {
    const u = new URL(rawUrl);
    if (!u.pathname.endsWith("/hls/master.m3u8")) return false;
    return !u.searchParams.has("maxKbps") && u.searchParams.get("abr") !== "floor";
  } catch {
    return false;
  }
}

// createStarvationWatch names a starving playback from the only two signals
// the receiver has: the playhead and the player state. Born 2026-08-03: a
// Bouygtel4K sat frozen for NINE minutes on a collapsed link, heartbeats
// alive, while the master advertised a rescue rung CAF never dove onto by
// itself - the descent has to be ours, and it starts with naming the famine.
//
// Sampled at a fixed cadence by the caller (~5s). A stalled sample is
// BUFFERING, or PLAYING with the playhead barely moving; healthy playback
// decays the score, so scattered micro-freezes accumulate but a recovered
// stream never fires. PAUSED/IDLE is a human, not a famine - it resets the
// baseline and scores nothing. Fires exactly once: the reload it triggers is
// a latch, not a loop.
export interface StarvationWatch {
  // sample returns true exactly once, when the accumulated starvation crosses
  // the threshold: reload on the floor rung NOW.
  sample(posSec: number, playerState: string): boolean;
}

export function createStarvationWatch(fireScore = 4): StarvationWatch {
  let lastPos: number | null = null;
  let score = 0;
  let fired = false;
  return {
    sample(posSec: number, playerState: string): boolean {
      if (fired) return false;
      if (playerState === "PAUSED" || playerState === "IDLE") {
        lastPos = null;
        return false;
      }
      if (playerState === "BUFFERING") {
        lastPos = posSec;
        score++;
      } else if (playerState === "PLAYING") {
        const prev = lastPos;
        lastPos = posSec;
        if (prev === null) return false; // fresh baseline (start, or post-pause)
        const advance = posSec - prev;
        if (advance < 1) score++;
        else if (advance > 2.5) score = Math.max(0, score - 1);
      } else {
        return false;
      }
      if (score >= fireScore) {
        fired = true;
        return true;
      }
      return false;
    },
  };
}
