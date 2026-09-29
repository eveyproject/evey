import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// getFmp4AudioPassthrough memoises, and so does the prefersNativeHLS it reads,
// so every case re-imports the module from scratch.

vi.mock("hls.js", () => ({
  default: { isSupported: () => true },
}));

vi.mock("@/utils/codec", () => ({
  canPlayHevc: () => false,
  getBrowserCapabilities: () => ({ videoCodecs: ["h264"], audioCodecs: ["aac", "mp3"], containers: ["mp4"], hdr: false }),
}));

const MP3_IN_FMP4 = 'audio/mp4; codecs="mp4a.6B"';
const OPUS_IN_FMP4 = 'audio/mp4; codecs="opus"';

function stubMediaSource(answer: (type: string) => boolean) {
  vi.stubGlobal("MediaSource", { isTypeSupported: (t: string) => answer(t) });
}

/** Makes the <video> element claim native HLS, which is what puts Safari/iOS
 *  on the AVPlayer path instead of hls.js. */
function stubNativeHLS(userAgent: string) {
  vi.spyOn(window.HTMLMediaElement.prototype, "canPlayType").mockReturnValue("maybe");
  vi.stubGlobal("navigator", { userAgent, platform: "MacIntel", maxTouchPoints: 0 });
}

beforeEach(() => {
  vi.resetModules();
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("getFmp4AudioPassthrough", () => {
  // The case that fixes the bug: Chrome/Firefox on hls.js, MSE takes MP3 muxed
  // in fMP4, so the server may COPY the track instead of cascading a 128 kbps
  // MP3 through its AAC encoder.
  it("declares mp3 when the MSE path accepts mp4a.6B", async () => {
    stubMediaSource((t) => t === MP3_IN_FMP4);
    const { getFmp4AudioPassthrough } = await import("./hlsPlayerUtils");
    expect(getFmp4AudioPassthrough()).toEqual(["mp3"]);
  });

  // A browser that cannot demux it gets SILENCE, not degraded audio, so the
  // probe's "no" must be honoured rather than assumed away.
  it("declares nothing when the MSE path refuses mp4a.6B", async () => {
    stubMediaSource(() => false);
    const { getFmp4AudioPassthrough } = await import("./hlsPlayerUtils");
    expect(getFmp4AudioPassthrough()).toEqual([]);
  });

  it("declares nothing when there is no MediaSource at all", async () => {
    vi.stubGlobal("MediaSource", undefined);
    vi.stubGlobal("ManagedMediaSource", undefined);
    const { getFmp4AudioPassthrough } = await import("./hlsPlayerUtils");
    expect(getFmp4AudioPassthrough()).toEqual([]);
  });

  // The guard that keeps Safari safe: our HLS goes through AVPlayer there, and
  // AVPlayer does not take MP3 in fMP4 whatever MSE happens to answer. This is
  // exactly why the declaration cannot be derived from a decoder list.
  it("declares nothing on the native-HLS path even when MSE says yes", async () => {
    stubMediaSource(() => true);
    stubNativeHLS("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 Version/17.0 Safari/605.1.15");
    const { getFmp4AudioPassthrough } = await import("./hlsPlayerUtils");
    expect(getFmp4AudioPassthrough()).toEqual([]);
  });

  it("declares nothing on iOS even when MSE says yes", async () => {
    stubMediaSource(() => true);
    stubNativeHLS("Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Version/17.0 Mobile/15E148 Safari/604.1");
    const { getFmp4AudioPassthrough } = await import("./hlsPlayerUtils");
    expect(getFmp4AudioPassthrough()).toEqual([]);
  });

  // The whole point of a separate declaration: "I decode MP3" is answered yes
  // by every browser here, including the ones that must not get a passthrough.
  it("stays independent of the decoder list, which always contains mp3", async () => {
    stubMediaSource(() => false);
    const { getFmp4AudioPassthrough } = await import("./hlsPlayerUtils");
    const { getBrowserCapabilities } = await import("@/utils/codec");
    expect(getBrowserCapabilities().audioCodecs).toContain("mp3");
    expect(getFmp4AudioPassthrough()).toEqual([]);
  });
});

// Opus is the second opt-in: declared when MSE takes it in fMP4 (Chrome,
// Firefox), alongside or without mp3, and never on the native-HLS path.
describe("getFmp4AudioPassthrough / opus", () => {
  it("declares opus when MSE accepts it in fMP4", async () => {
    stubMediaSource((t) => t === OPUS_IN_FMP4);
    const { getFmp4AudioPassthrough } = await import("./hlsPlayerUtils");
    expect(getFmp4AudioPassthrough()).toEqual(["opus"]);
  });

  it("declares both when the browser takes both", async () => {
    stubMediaSource(() => true);
    const { getFmp4AudioPassthrough } = await import("./hlsPlayerUtils");
    expect(getFmp4AudioPassthrough()).toEqual(["mp3", "opus"]);
  });
});

describe("appendCodecParams / fmp4AudioPassthrough", () => {
  it("puts the opt-in on the HLS master URL when the browser accepts it", async () => {
    stubMediaSource((t) => t === MP3_IN_FMP4);
    const { buildTranscodeHlsUrl } = await import("./hlsPlayerUtils");
    expect(buildTranscodeHlsUrl("m1", "tok", 0, null, 0)).toContain("fmp4AudioPassthrough=mp3");
  });

  it("leaves the URL untouched for a browser that refuses it", async () => {
    stubMediaSource(() => false);
    const { buildTranscodeHlsUrl } = await import("./hlsPlayerUtils");
    expect(buildTranscodeHlsUrl("m1", "tok", 0, null, 0)).not.toContain("fmp4AudioPassthrough");
  });
});
