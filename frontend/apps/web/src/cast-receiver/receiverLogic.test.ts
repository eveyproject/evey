import { describe, it, expect } from "vitest";
import { absoluteWithToken, appendMaxKbps, bufferedStats, planBackBufferTrim, planFloorRescue, bitrateTestUrl, buildPlayerEventUrl, createDecoderDistressWatch, createStarvationWatch, createStartGate, createStartWatch, describePlayerError, floorMasterUrl, probeMaxAudioChannels, probeReceiverCaps, rewriteCastMasterUrl, shouldMeasure, stereoFallbackUrl, stripWasmAudio, type CanDisplayType } from "./receiverLogic";

// Fake canDisplayType: allows what `yes` lists (substring match on the
// codec string), refuses the rest. 4K resolution is only granted if
// `fourK` is true.
function fakeCDT(yes: string[], fourK = true): CanDisplayType {
  return (_mime, codec, width) => {
    if (width && width > 1920 && !fourK) return false;
    if (!codec) return true;
    return yes.some((y) => codec.startsWith(y));
  };
}

describe("probeReceiverCaps - the receiver probes ITSELF (no more per-model tables)", () => {
  it("a modern device reports hevc/vp9, 4K and system HDR", () => {
    const caps = probeReceiverCaps(fakeCDT(["avc1", "hev1", "vp09"]), true);
    expect(caps.videoCodecs).toEqual(["h264", "hevc", "vp9"]);
    expect(caps.maxHeight).toBe(2160);
    expect(caps.hdr).toBe(true);
  });

  it("an h264-only 1080p SDR device falls back to the floor without ever lying", () => {
    const caps = probeReceiverCaps(fakeCDT(["avc1"], false), false);
    expect(caps.videoCodecs).toEqual(["h264"]);
    expect(caps.maxHeight).toBe(1080);
    expect(caps.hdr).toBe(false);
  });

  it("native Dolby is only reported if the device probes true", () => {
    expect(probeReceiverCaps(fakeCDT(["avc1", "ec-3", "ac-3"]), false).nativeDolby).toBe(true);
    expect(probeReceiverCaps(fakeCDT(["avc1"]), false).nativeDolby).toBe(false);
  });
});

describe("probeMaxAudioChannels - the device answers, nothing is assumed", () => {
  const yes = async () => ({ supported: true });
  const no = async () => ({ supported: false });

  it("keeps 5.1 when the decoder says yes and the sink does not answer", async () => {
    expect(await probeMaxAudioChannels({ decodingInfo: yes })).toBe(6);
  });

  // The Chromecast HD of 2026-08-04: an explicit no, and the title never
  // started because nobody asked.
  it("drops to stereo on an explicit no", async () => {
    expect(await probeMaxAudioChannels({ decodingInfo: no })).toBe(2);
  });

  // decodingInfo answers a DECODE question, the sink answers a RESTITUTION
  // one. Reading the first as the second is what served 5.1 to a 2.0 TV, where
  // the browser's un-normalised fold (+7.66 dB worst case) clips everything
  // that is not the lone attenuated centre channel.
  it("caps a 5.1-capable decoder at the stereo sink behind it", async () => {
    expect(await probeMaxAudioChannels({ decodingInfo: yes, maxChannelCount: 2 })).toBe(2);
  });

  it("keeps 5.1 when the sink itself is multichannel", async () => {
    expect(await probeMaxAudioChannels({ decodingInfo: yes, maxChannelCount: 8 })).toBe(6);
  });

  it("falls back to the sink when decodingInfo is missing or throws", async () => {
    expect(await probeMaxAudioChannels({ maxChannelCount: 8 })).toBe(6);
    expect(await probeMaxAudioChannels({ maxChannelCount: 2 })).toBe(2);
    expect(await probeMaxAudioChannels({})).toBe(2);
    const boom = async () => { throw new Error("no"); };
    expect(await probeMaxAudioChannels({ decodingInfo: boom, maxChannelCount: 8 })).toBe(6);
  });

  it("asks about the codec the server actually sends, at 6 channels", async () => {
    let seen: any = null;
    await probeMaxAudioChannels({ decodingInfo: async (c) => { seen = c; return { supported: true }; } });
    expect(seen.type).toBe("media-source");
    expect(seen.audio.contentType).toContain("mp4a.40.2");
    expect(seen.audio.channels).toBe("6");
  });
});

describe("stereoFallbackUrl - the net for a probe that lied", () => {
  it("pins the ceiling to stereo once, then latches", () => {
    const url = "https://evey.example/api/v1/media/m1/hls/master.m3u8?token=t&maxAudioChannels=6";
    const out = stereoFallbackUrl(url)!;
    expect(new URL(out).searchParams.get("maxAudioChannels")).toBe("2");
    expect(new URL(out).searchParams.get("token")).toBe("t");
    expect(stereoFallbackUrl(out)).toBeNull();
  });

  it("returns null on a URL it cannot parse", () => {
    expect(stereoFallbackUrl("not a url")).toBeNull();
  });
});

describe("rewriteCastMasterUrl - negotiation happens on the REAL caps", () => {
  const MASTER = "https://evey.example/api/v1/media/m1/hls/master.m3u8?token=tok&videoCodecs=h264&audioCodecs=aac&containers=mp4&maxHeight=1080";
  const modern = probeReceiverCaps(fakeCDT(["avc1", "hev1", "vp09"]), true);
  const floorSdr = probeReceiverCaps(fakeCDT(["avc1"], false), false);

  it("rewrites the master's video/height/hdr caps with the probed values", () => {
    const out = rewriteCastMasterUrl(MASTER, modern, true);
    expect(out).not.toBeNull();
    const u = new URL(out!.url);
    expect(u.searchParams.get("videoCodecs")).toBe("h264,hevc,vp9");
    expect(u.searchParams.get("maxHeight")).toBe("2160");
    expect(u.searchParams.get("hdr")).toBe("1");
    expect(u.searchParams.get("containers")).toBe("mp4");
    expect(u.searchParams.get("token")).toBe("tok"); // the cast token survives
  });

  it("without native Dolby but with wasm: AAC audio + wasmAudio=1 (ac3go on the TV)", () => {
    const out = rewriteCastMasterUrl(MASTER, modern, true)!;
    const u = new URL(out.url);
    expect(u.searchParams.get("audioCodecs")).toBe("aac,mp3,opus,flac");
    expect(u.searchParams.get("wasmAudio")).toBe("1");
    expect(out.wasmAudio).toBe(true);
  });

  it("with native Dolby probed true: eac3/ac3 passthrough, NO wasm", () => {
    const dolby = probeReceiverCaps(fakeCDT(["avc1", "ec-3", "ac-3"]), false);
    const out = rewriteCastMasterUrl(MASTER, dolby, true)!;
    const u = new URL(out.url);
    expect(u.searchParams.get("audioCodecs")).toBe("aac,mp3,opus,flac,eac3,ac3");
    expect(u.searchParams.has("wasmAudio")).toBe(false);
    expect(out.wasmAudio).toBe(false);
  });

  it("without wasm available or Dolby: transcoded AAC, no wasmAudio (the always-working path)", () => {
    const out = rewriteCastMasterUrl(MASTER, floorSdr, false)!;
    const u = new URL(out.url);
    expect(u.searchParams.get("audioCodecs")).toBe("aac,mp3,opus,flac");
    expect(u.searchParams.has("wasmAudio")).toBe(false);
    expect(u.searchParams.has("hdr")).toBe(false);
  });

  // Prod 2026-08-04: nothing declared a channel ceiling, the server applied
  // its 5.1 web default and emitted `-ac 6`; the Chromecast HD refused the
  // stream and the title never started. Jellyfin played the same file on the
  // same TV that evening with `-ac 2`.
  it("always declares the channel ceiling the device answered", () => {
    expect(new URL(rewriteCastMasterUrl(MASTER, floorSdr, false)!.url).searchParams.get("maxAudioChannels")).toBe("2");
    const surround = probeReceiverCaps(fakeCDT(["avc1"]), false, 6);
    expect(new URL(rewriteCastMasterUrl(MASTER, surround, false)!.url).searchParams.get("maxAudioChannels")).toBe("6");
  });

  // The rescue paths reload the master; dropping the ceiling there would let
  // the server fall back to 5.1 and re-break playback at the worst moment.
  it("keeps the ceiling across the starvation floor and the measured-bitrate reloads", () => {
    const first = rewriteCastMasterUrl(MASTER, floorSdr, false)!.url;
    expect(new URL(floorMasterUrl(first)!).searchParams.get("maxAudioChannels")).toBe("2");
    expect(new URL(appendMaxKbps(first, 4200)!).searchParams.get("maxAudioChannels")).toBe("2");
    expect(new URL(stripWasmAudio(first)).searchParams.get("maxAudioChannels")).toBe("2");
  });

  it("leaves URLs that are not our master untouched (direct play, third-party)", () => {
    expect(rewriteCastMasterUrl("https://evey.example/api/v1/media/m1/stream?token=t", modern, true)).toBeNull();
    expect(rewriteCastMasterUrl("https://example.com/other.m3u8", modern, true)).toBeNull();
    expect(rewriteCastMasterUrl("not a url", modern, true)).toBeNull();
  });
});

describe("stripWasmAudio - the fallback when the wasm engine dies on the TV", () => {
  it("removes wasmAudio and keeps everything else (the server re-serves the muxed AAC master)", () => {
    const url = "https://evey.example/api/v1/media/m1/hls/master.m3u8?token=tok&wasmAudio=1&audioCodecs=aac";
    const out = stripWasmAudio(url);
    const u = new URL(out);
    expect(u.searchParams.has("wasmAudio")).toBe(false);
    expect(u.searchParams.get("token")).toBe("tok");
    expect(u.searchParams.get("audioCodecs")).toBe("aac");
  });
});

describe("absoluteWithToken - the receiver can be hosted OFF the media server", () => {
  // Hosted on GitHub Pages (or any other domain), the receiver resolves
  // playlist/segment URLs against the MEDIA SERVER, never against its own
  // origin - injectToken (web, same-origin) would produce a relative path
  // that goes to github.io and 404s.
  it("keeps the media server's origin and refreshes the token", () => {
    const out = absoluteWithToken(
      "audio_1.m3u8?token=stale",
      "https://evey.example/api/v1/media/m1/hls/master.m3u8?token=fresh",
      "fresh",
    );
    expect(out).toBe("https://evey.example/api/v1/media/m1/hls/audio_1.m3u8?token=fresh");
  });

  it("leaves an already-absolute URL on its own host alone", () => {
    const out = absoluteWithToken("https://evey.example/seg1.m4s?a=1", "https://other.example/x", "tok");
    const u = new URL(out);
    expect(u.host).toBe("evey.example");
    expect(u.searchParams.get("token")).toBe("tok");
    expect(u.searchParams.get("a")).toBe("1");
  });

  it("without a token, resolves absolute without inventing one", () => {
    const out = absoluteWithToken("seg1.m4s", "https://evey.example/hls/audio.m3u8", "");
    expect(out).toBe("https://evey.example/hls/seg1.m4s");
  });
});

describe("mixed content - Evey instance on HTTP on the LAN, receiver on HTTPS", () => {
  // The receiver page is necessarily HTTPS (Cast Console requirement). The
  // CAF player is allowed to load http: MEDIA (Google doc), but our JS
  // fetches (wasm master, ac3go segments) are active mixed content: blocked.
  // So: never wasmAudio toward an http: server - the muxed AAC that the CAF
  // player loads natively is the only path that works there.
  const HTTP_MASTER = "http://192.168.1.50:8096/api/v1/media/m1/hls/master.m3u8?token=tok";

  it("never requests wasmAudio from an http: server from an https: page", () => {
    const caps = probeReceiverCaps(fakeCDT(["avc1", "hev1"]), false);
    const out = rewriteCastMasterUrl(HTTP_MASTER, caps, true, "https:")!;
    const u = new URL(out.url);
    expect(u.searchParams.has("wasmAudio")).toBe(false);
    expect(out.wasmAudio).toBe(false);
    // The rest of the negotiation (video) stays active: the CAF player
    // can load this http master.
    expect(u.searchParams.get("videoCodecs")).toBe("h264,hevc");
  });

  it("keeps the wasm path when the server is https:", () => {
    const caps = probeReceiverCaps(fakeCDT(["avc1"]), false);
    const out = rewriteCastMasterUrl(
      "https://evey.example/api/v1/media/m1/hls/master.m3u8?token=tok",
      caps, true, "https:",
    )!;
    expect(out.wasmAudio).toBe(true);
  });
});

describe("buildPlayerEventUrl - the receiver's telemetry door", () => {
  // 2026-07-26: the TV stopped 3-5 min into a cast while the phone slept, and
  // the server logs had NOT ONE line about why - the receiver shows its errors
  // on the TV screen only. This URL is how CAF errors/idle reasons become
  // server-side log lines, authenticated by the cast stream token the sender
  // baked into the master URL.
  it("derives origin, media id and token from the casted master URL", () => {
    expect(
      buildPlayerEventUrl("https://evey.example/api/v1/media/m-42/hls/master.m3u8?token=tok-1&wasmAudio=1"),
    ).toBe("https://evey.example/api/v1/media/m-42/player-event?token=tok-1");
  });

  it("works for direct-play stream URLs too", () => {
    expect(
      buildPlayerEventUrl("https://evey.example/api/v1/media/abc/stream?token=t2"),
    ).toBe("https://evey.example/api/v1/media/abc/player-event?token=t2");
  });

  it("returns null for third-party or tokenless URLs", () => {
    expect(buildPlayerEventUrl("https://example.com/video.mp4")).toBeNull();
    expect(buildPlayerEventUrl("https://evey.example/api/v1/media/m1/hls/master.m3u8")).toBeNull();
    expect(buildPlayerEventUrl("not a url")).toBeNull();
  });
});

describe("createStartGate", () => {
  const mk = () => {
    const calls: string[] = [];
    let armed: (() => void) | null = null;
    const gate = createStartGate(
      () => calls.push("play"),
      4000,
      (fn) => { armed = fn; return 1 as unknown as ReturnType<typeof setTimeout>; },
      () => { armed = null; },
    );
    return { gate, calls, fireTimeout: () => armed?.() };
  };

  it("plays once when the audio is ready, and never twice", () => {
    const { gate, calls, fireTimeout } = mk();
    gate.arm();
    gate.audioReady();
    gate.audioReady();
    fireTimeout();
    expect(calls).toEqual(["play"]);
  });

  it("the safety timeout releases a start whose audio never came", () => {
    const { gate, calls, fireTimeout } = mk();
    gate.arm();
    fireTimeout();
    expect(calls).toEqual(["play"]);
  });

  it("skip releases immediately (server did not take the wasm path)", () => {
    const { gate, calls } = mk();
    gate.arm();
    gate.skip();
    expect(calls).toEqual(["play"]);
  });

  it("cancel disarms without playing (a new LOAD supersedes)", () => {
    const { gate, calls, fireTimeout } = mk();
    gate.arm();
    gate.cancel();
    gate.audioReady();
    fireTimeout();
    expect(calls).toEqual([]);
  });

  it("re-arm after a cancel serves the next load", () => {
    const { gate, calls } = mk();
    gate.arm();
    gate.cancel();
    gate.arm();
    gate.audioReady();
    expect(calls).toEqual(["play"]);
  });
});

// The start watch names the failure the 2026-08-04/05 Chromecast HD sessions
// proved: the hybrid master is fetched, then NOTHING - no media request, no
// ERROR event, spinner forever. Every event-driven net stays blind there;
// elapsed time is the only signal a silently dead LOAD leaves.
describe("createStartWatch - a load that never completes is declared dead", () => {
  const mk = () => {
    const calls: string[] = [];
    let armed: (() => void) | null = null;
    const watch = createStartWatch(
      () => calls.push("rescue"),
      12000,
      (fn) => { armed = fn; return 1 as unknown as ReturnType<typeof setTimeout>; },
      () => { armed = null; },
    );
    return { watch, calls, fireTimeout: () => armed?.() };
  };

  it("rescues a load that never reached PLAYER_LOAD_COMPLETE", () => {
    const { watch, calls, fireTimeout } = mk();
    watch.arm();
    fireTimeout();
    expect(calls).toEqual(["rescue"]);
  });

  it("a completed load is never rescued", () => {
    const { watch, calls, fireTimeout } = mk();
    watch.arm();
    watch.loaded();
    fireTimeout();
    expect(calls).toEqual([]);
  });

  it("cancel disarms without rescuing (teardown, new LOAD)", () => {
    const { watch, calls, fireTimeout } = mk();
    watch.arm();
    watch.cancel();
    fireTimeout();
    expect(calls).toEqual([]);
  });

  it("re-arm watches the next load - including the rescue's own reload", () => {
    const { watch, calls, fireTimeout } = mk();
    watch.arm();
    fireTimeout();
    watch.arm();
    fireTimeout();
    expect(calls).toEqual(["rescue", "rescue"]);
  });
});

// The starvation watch names the failure the 2026-08-03 session measured:
// heartbeats alive, position pinned, CAF never diving to the announced rung
// by itself. Sampled every ~5s by the caller; firing means "reload on the
// floor rung NOW".
describe("createStarvationWatch", () => {
  it("fires after a sustained freeze while PLAYING", () => {
    const w = createStarvationWatch();
    expect(w.sample(10, "PLAYING")).toBe(false); // baseline
    expect(w.sample(10, "PLAYING")).toBe(false);
    expect(w.sample(10, "PLAYING")).toBe(false);
    expect(w.sample(10, "PLAYING")).toBe(false);
    expect(w.sample(10, "PLAYING")).toBe(true); // 4 stalled samples = ~20s
  });

  it("fires through a long initial BUFFERING (the 'début difficile')", () => {
    const w = createStarvationWatch();
    expect(w.sample(0, "BUFFERING")).toBe(false);
    expect(w.sample(0, "BUFFERING")).toBe(false);
    expect(w.sample(0, "BUFFERING")).toBe(false);
    expect(w.sample(0, "BUFFERING")).toBe(true);
  });

  it("accumulates repeated micro-freezes but decays on healthy playback", () => {
    const w = createStarvationWatch();
    w.sample(10, "PLAYING"); // baseline
    w.sample(10.2, "PLAYING"); // micro-freeze: +1
    w.sample(15.2, "PLAYING"); // healthy: -1
    w.sample(20.2, "PLAYING"); // healthy
    w.sample(20.3, "PLAYING"); // +1
    w.sample(20.4, "PLAYING"); // +2
    w.sample(20.5, "PLAYING"); // +3
    expect(w.sample(20.6, "PLAYING")).toBe(true); // +4
  });

  it("a pause is a human, not a starvation", () => {
    const w = createStarvationWatch();
    w.sample(10, "PLAYING");
    for (let i = 0; i < 10; i++) expect(w.sample(10, "PAUSED")).toBe(false);
    // Resume: the frozen position right after a pause is not a stall sample.
    expect(w.sample(10, "PLAYING")).toBe(false);
  });

  it("fires exactly once", () => {
    const w = createStarvationWatch();
    for (let i = 0; i < 5; i++) w.sample(10, "PLAYING");
    expect(w.sample(10, "PLAYING")).toBe(false);
  });
});

describe("floorMasterUrl", () => {
  it("appends abr=floor and strips wasmAudio", () => {
    const url = floorMasterUrl("https://evey.test/api/v1/media/m1/hls/master.m3u8?token=t&wasmAudio=1");
    expect(url).toContain("abr=floor");
    expect(url).not.toContain("wasmAudio");
    expect(url).toContain("token=t");
  });

  it("latches: an already-floored URL has nothing lower to offer", () => {
    expect(floorMasterUrl("https://evey.test/api/v1/media/m1/hls/master.m3u8?abr=floor")).toBeNull();
  });

  it("refuses a foreign URL", () => {
    expect(floorMasterUrl("not a url")).toBeNull();
  });
});

// The entry measure: before playing, the receiver times its OWN stack against
// /hls/bitrate-test and asks for a master fitted to the verdict (?maxKbps=).
// Copy-first lives server-side; these helpers only derive URLs.
describe("bitrateTestUrl", () => {
  it("derives the same-media test endpoint with the stream token", () => {
    const url = bitrateTestUrl("https://evey.test/api/v1/media/m1/hls/master.m3u8?token=t&videoCodecs=h264");
    expect(url).toBe("https://evey.test/api/v1/media/m1/hls/bitrate-test?token=t");
  });

  it("refuses tokenless or foreign URLs", () => {
    expect(bitrateTestUrl("https://evey.test/api/v1/media/m1/hls/master.m3u8")).toBeNull();
    expect(bitrateTestUrl("https://elsewhere.test/some/video.m3u8?token=t")).toBeNull();
    expect(bitrateTestUrl("not a url")).toBeNull();
  });
});

describe("appendMaxKbps / shouldMeasure", () => {
  it("appends the measured verdict", () => {
    const u = appendMaxKbps("https://evey.test/api/v1/media/m1/hls/master.m3u8?token=t", 4213);
    expect(u).toContain("maxKbps=4213");
    expect(u).toContain("token=t");
  });

  it("refuses a garbage measure", () => {
    expect(appendMaxKbps("https://evey.test/api/v1/media/m1/hls/master.m3u8?token=t", 0)).toBeNull();
    expect(appendMaxKbps("https://evey.test/api/v1/media/m1/hls/master.m3u8?token=t", NaN)).toBeNull();
  });

  it("measures our masters exactly once, never a rescue reload", () => {
    expect(shouldMeasure("https://evey.test/api/v1/media/m1/hls/master.m3u8?token=t")).toBe(true);
    expect(shouldMeasure("https://evey.test/api/v1/media/m1/hls/master.m3u8?token=t&maxKbps=4000")).toBe(false);
    expect(shouldMeasure("https://evey.test/api/v1/media/m1/hls/master.m3u8?token=t&abr=floor")).toBe(false);
    expect(shouldMeasure("https://elsewhere.test/video.m3u8")).toBe(false);
  });
});

describe("rewriteCastMasterUrl under abr=floor", () => {
  it("never re-engages wasm audio on a floored master", () => {
    const caps = { videoCodecs: ["h264"], maxHeight: 1080, hdr: false, nativeDolby: false };
    const out = rewriteCastMasterUrl(
      "https://evey.test/api/v1/media/m1/hls/master.m3u8?abr=floor&token=t",
      caps,
      true,
    );
    expect(out).not.toBeNull();
    expect(out!.wasmAudio).toBe(false);
    expect(out!.url).not.toContain("wasmAudio");
    expect(out!.url).toContain("abr=floor");
  });
});

describe("bufferedStats - what the MSE pipeline actually retains", () => {
  it("splits buffered media around the playhead and counts ranges", () => {
    const s = bufferedStats([[0, 150]], 140);
    expect(s.behindSec).toBeCloseTo(140);
    expect(s.aheadSec).toBeCloseTo(10);
    expect(s.rangeCount).toBe(1);
  });

  it("sums across a hole and reports the range count", () => {
    const s = bufferedStats([[672, 678], [681, 711]], 678);
    expect(s.behindSec).toBeCloseTo(6);
    expect(s.aheadSec).toBeCloseTo(30);
    expect(s.rangeCount).toBe(2);
  });

  it("reports zeros when nothing is buffered", () => {
    const s = bufferedStats([], 42);
    expect(s.behindSec).toBe(0);
    expect(s.aheadSec).toBe(0);
    expect(s.rangeCount).toBe(0);
  });
});

describe("planBackBufferTrim - bound what a 2GB box must retain", () => {
  it("trims everything further behind than the keep window", () => {
    expect(planBackBufferTrim([[0, 150]], 140, 20, 10)).toEqual({ start: 0, end: 120 });
  });

  it("stays quiet under the hysteresis span", () => {
    expect(planBackBufferTrim([[115, 150]], 140, 20, 10)).toBeNull();
  });

  it("stays quiet early in playback", () => {
    expect(planBackBufferTrim([[0, 30]], 25, 20, 10)).toBeNull();
  });

  it("trims across holes in one call", () => {
    expect(planBackBufferTrim([[0, 50], [60, 150]], 140, 20, 10)).toEqual({ start: 0, end: 120 });
  });

  it("has nothing to trim on an empty buffer", () => {
    expect(planBackBufferTrim([], 140, 20, 10)).toBeNull();
  });
});

describe("planFloorRescue - the floor is retried in place before giving up", () => {
  const master = "https://evey.test/api/v1/media/m1/hls/master.m3u8?token=t&wasmAudio=1";
  const floored = "https://evey.test/api/v1/media/m1/hls/master.m3u8?token=t&abr=floor";

  it("first rescue floors the master (wasm stripped)", () => {
    const plan = planFloorRescue(master, 0, 1);
    expect(plan).toMatchObject({ kind: "floor" });
    const u = new URL((plan as { url: string }).url);
    expect(u.searchParams.get("abr")).toBe("floor");
    expect(u.searchParams.has("wasmAudio")).toBe(false);
  });

  it("a starving floor gets one reload in place", () => {
    expect(planFloorRescue(floored, 0, 1)).toEqual({ kind: "floor-reload", url: floored });
  });

  it("the in-place reload budget is bounded, then the rescue declares exhaustion", () => {
    expect(planFloorRescue(floored, 1, 1)).toEqual({ kind: "exhausted" });
  });

  it("a URL we do not understand yields no plan", () => {
    expect(planFloorRescue("not a url", 0, 1)).toBeNull();
  });
});

describe("describePlayerError - an error object must never print as [object Object]", () => {
  it("renders a CAF ErrorData instead of the useless String() cast", () => {
    const out = describePlayerError({ type: "ERROR", detailedErrorCode: 905, reason: "LOAD_FAILED" });
    expect(out).toBe("type=ERROR code=905 reason=LOAD_FAILED");
    expect(out).not.toContain("[object Object]");
  });

  it("keeps the shaka triple that names which parser read past its buffer", () => {
    const out = describePlayerError({
      shakaErrorCode: 3000,
      shakaErrorCategory: 3,
      shakaErrorSeverity: 2,
      shakaErrorData: ["seg.m4s"],
    });
    expect(out).toBe('shaka=3000 cat=3 sev=2 data=["seg.m4s"]');
  });

  it("strips the googleapis prefix every shaka frame repeats", () => {
    const stack = [
      "Error: Shaka Error 3000",
      "    at new T (https://ajax.googleapis.com/ajax/libs/shaka-player/4.3.4/shaka-player.compiled.js:1:2345)",
      "    at ab (https://ajax.googleapis.com/ajax/libs/shaka-player/4.3.4/shaka-player.compiled.js:9:87)",
    ].join("\n");
    const out = describePlayerError({ shakaErrorCode: 3000, shakaErrorStack: stack });
    expect(out).toBe("shaka=3000 at T@1:2345,ab@9:87");
    expect(out).not.toContain("googleapis");
    // The server clamps details at 200 chars: the frames must survive that cut.
    expect(out.length).toBeLessThan(200);
  });

  it("falls back to the raw object rather than losing it, and never throws", () => {
    expect(describePlayerError({ weird: 1 })).toBe('{"weird":1}');
    expect(describePlayerError(undefined)).toBe("?");
    expect(describePlayerError("plain string")).toBe("plain string");
  });
});

// Incident the owner 2026-08-23 : gel du cast a tampon PLEIN. La veille de
// famine est structurellement AVEUGLE a ce cas, et la mesure prod le prouve
// beacon par beacon : pendant les 5 min 30 de degradation la position avance
// a exactement 1,000x (19:29:08 pos=62,64 -> 19:33:38 pos=332,65, soit 30,00 s
// par battement de 30 s) et le tampon reste plein (buf=31/21 constant). Aucun
// signal existant ne bouge - "floor" n'apparait ZERO fois dans les logs de la
// soiree. Seule la qualite de RENDU nomme un decodeur qui decroche : des
// frames perdues qui montent pendant que l'horloge audio tient la cadence.
describe("createDecoderDistressWatch - le decodeur decroche, tampon plein", () => {
  it("declenche apres une souffrance soutenue, position parfaite et tampon plein", () => {
    const w = createDecoderDistressWatch();
    // 5s de video ~ 120 frames a 23,976 fps ; 10 % de perte = le decodeur lache.
    expect(w.sample("PLAYING", 18, 120)).toBe(false);
    expect(w.sample("PLAYING", 24, 120)).toBe(false);
    expect(w.sample("PLAYING", 31, 120)).toBe(true); // ~15 s de souffrance
  });

  it("ne declenche JAMAIS sur une lecture saine - le cas nominal 6 Mbps", () => {
    const w = createDecoderDistressWatch();
    for (let i = 0; i < 200; i++) expect(w.sample("PLAYING", 0, 120)).toBe(false);
    expect(w.sample("PLAYING", 1, 120)).toBe(false); // une frame perdue isolee n'est rien
  });

  it("une reprise fait redescendre le score : un pic passager ne declenche pas", () => {
    const w = createDecoderDistressWatch();
    expect(w.sample("PLAYING", 30, 120)).toBe(false);
    expect(w.sample("PLAYING", 30, 120)).toBe(false);
    expect(w.sample("PLAYING", 0, 120)).toBe(false); // decodeur repris
    expect(w.sample("PLAYING", 0, 120)).toBe(false);
    expect(w.sample("PLAYING", 30, 120)).toBe(false); // reparti de bas
  });

  it("un runtime aveugle (-1) ne score ni ne decroit : pas de mesure, pas de verdict", () => {
    const w = createDecoderDistressWatch();
    for (let i = 0; i < 10; i++) expect(w.sample("PLAYING", -1, -1)).toBe(false);
    expect(w.sample("PLAYING", 30, 120)).toBe(false);
    expect(w.sample("PLAYING", 30, 120)).toBe(false);
    expect(w.sample("PLAYING", 30, 120)).toBe(true);
  });

  it("un echantillon trop maigre ne decide rien (1 frame perdue sur 3)", () => {
    const w = createDecoderDistressWatch();
    for (let i = 0; i < 10; i++) expect(w.sample("PLAYING", 1, 3)).toBe(false);
  });

  it("PAUSED / IDLE / BUFFERING ne sont pas son affaire", () => {
    const w = createDecoderDistressWatch();
    expect(w.sample("PAUSED", 60, 120)).toBe(false);
    expect(w.sample("IDLE", 60, 120)).toBe(false);
    expect(w.sample("BUFFERING", 60, 120)).toBe(false); // la famine, pas le decodeur
    expect(w.sample("PLAYING", 60, 120)).toBe(false);
    expect(w.sample("PLAYING", 60, 120)).toBe(false);
    expect(w.sample("PLAYING", 60, 120)).toBe(true);
  });

  it("se verrouille apres avoir tire : le repli est un cliquet, pas une boucle", () => {
    const w = createDecoderDistressWatch();
    w.sample("PLAYING", 30, 120);
    w.sample("PLAYING", 30, 120);
    expect(w.sample("PLAYING", 30, 120)).toBe(true);
    expect(w.sample("PLAYING", 60, 120)).toBe(false);
  });
});
