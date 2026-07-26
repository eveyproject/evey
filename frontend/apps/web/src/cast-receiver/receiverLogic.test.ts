import { describe, it, expect } from "vitest";
import { absoluteWithToken, buildPlayerEventUrl, probeReceiverCaps, rewriteCastMasterUrl, stripWasmAudio, type CanDisplayType } from "./receiverLogic";

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
