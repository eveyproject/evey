import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { resetOutputChannelsProbe } from "@/utils/outputChannels";
import { startPlayback, planCastLoad, pickCastTapPath, castVideoPreload, pickPlayOnFallback, pickTranscodeAudioSwitchPath, pickPinnedLevel, liftForcedMuteOnNextGesture } from "./hlsPlayerUtils";
import type { SubtitleStream } from "../SubtitleSelector";

const mockCaps = vi.hoisted(() => ({
  videoCodecs: ["h264"],
  audioCodecs: ["aac", "mp3", "opus"],
  containers: ["mp4"],
  hdr: false,
}));

vi.mock("@/utils/codec", () => ({
  canPlayHevc: () => false,
  getBrowserCapabilities: () => mockCaps,
}));

import {
  getSelectedBurnInTrack,
  buildTranscodeHlsUrl,
  resolveDefaultAudio,
  resolveDefaultSubtitle,
  shouldRestartTranscodeSeek,
  computeNativeHLSPreference,
} from "./hlsPlayerUtils";

// Minimal TimeRanges stub: one contiguous buffered range [start, end].
function bufferedRange(start: number, end: number): TimeRanges {
  return {
    length: 1,
    start: (i: number) => (i === 0 ? start : NaN),
    end: (i: number) => (i === 0 ? end : NaN),
  } as TimeRanges;
}
const noBuffer = { length: 0, start: () => NaN, end: () => NaN } as TimeRanges;

const textSub: SubtitleStream = { index: 3, trackIndex: 0, language: "eng", codec: "subrip" };
const pgsSub: SubtitleStream = { index: 4, trackIndex: 1, language: "fre", codec: "hdmv_pgs_subtitle", burnIn: true };
const vobSub: SubtitleStream = { index: 5, trackIndex: 2, language: "jpn", codec: "dvd_subtitle", burnIn: true };
const streams = [textSub, pgsSub, vobSub];

describe("shouldRestartTranscodeSeek", () => {
  // offset 0, currently watching ~30min: the sliding window only buffers near
  // the playhead (1770..1830 local). This is the reported bug case.
  const window = bufferedRange(1770, 1830);

  it("restarts on a backward seek before the buffered window (30min -> 10min)", () => {
    // target 600s, offset 0 -> localTime 600, which is < buffered.start (1770).
    expect(shouldRestartTranscodeSeek(600, 0, window)).toBe(true);
  });

  it("restarts on a forward seek past the buffered window + tolerance", () => {
    expect(shouldRestartTranscodeSeek(1900, 0, window)).toBe(true);
  });

  it("seeks in place within the buffered window", () => {
    expect(shouldRestartTranscodeSeek(1800, 0, window)).toBe(false);
  });

  it("allows a small forward seek inside the 10s live-edge tolerance", () => {
    expect(shouldRestartTranscodeSeek(1838, 0, window)).toBe(false); // local 1838 <= 1830 + 10
    expect(shouldRestartTranscodeSeek(1841, 0, window)).toBe(true); // local 1841 > 1830 + 10
  });

  it("accounts for the transcode offset (local = absolute - offset)", () => {
    // transcode restarted at 600; buffer holds local 0..60 (absolute 600..660).
    const w = bufferedRange(0, 60);
    expect(shouldRestartTranscodeSeek(630, 600, w)).toBe(false); // local 30, in window
    expect(shouldRestartTranscodeSeek(500, 600, w)).toBe(true); // local -100, before start
    expect(shouldRestartTranscodeSeek(1800, 600, w)).toBe(true); // local 1200, past window
  });

  it("restarts when nothing is buffered yet", () => {
    expect(shouldRestartTranscodeSeek(600, 0, noBuffer)).toBe(true);
  });
});

describe("getSelectedBurnInTrack", () => {
  it("returns null when no subtitle selected", () => {
    expect(getSelectedBurnInTrack(null, streams)).toBeNull();
  });

  it("returns null for text-based subtitle", () => {
    expect(getSelectedBurnInTrack(0, streams)).toBeNull();
  });

  it("returns trackIndex for PGS burn-in", () => {
    expect(getSelectedBurnInTrack(1, streams)).toBe(1);
  });

  it("returns trackIndex for VobSub burn-in", () => {
    expect(getSelectedBurnInTrack(2, streams)).toBe(2);
  });

  it("returns null for empty streams array", () => {
    expect(getSelectedBurnInTrack(0, [])).toBeNull();
  });

  it("returns null when trackIndex not found in streams", () => {
    expect(getSelectedBurnInTrack(99, streams)).toBeNull();
  });
});

// The channel ceiling of the OUTPUT, which no playback request used to carry:
// the server profile pinned MaxAudioChannels = 6 and the browser folded 5.1
// onto two speakers with its un-normalised "speakers" matrix (worst case
// +7.66 dB) - voices fine, music and effects clipping.
describe("buildTranscodeHlsUrl: maxAudioChannels", () => {
  class FakeAudioContext {
    static channels = 2;
    destination = { maxChannelCount: FakeAudioContext.channels };
    close() { return Promise.resolve(); }
  }

  beforeEach(() => { resetOutputChannelsProbe(); });
  afterEach(() => { vi.unstubAllGlobals(); resetOutputChannelsProbe(); });

  it("declares the stereo ceiling the output actually has", () => {
    FakeAudioContext.channels = 2;
    vi.stubGlobal("AudioContext", FakeAudioContext);
    expect(buildTranscodeHlsUrl("m1", "tok", 0, null, 0)).toContain("maxAudioChannels=2");
  });

  it("leaves a real 5.1 output at 5.1", () => {
    FakeAudioContext.channels = 6;
    vi.stubGlobal("AudioContext", FakeAudioContext);
    expect(buildTranscodeHlsUrl("m1", "tok", 0, null, 0)).toContain("maxAudioChannels=6");
  });

  it("omits the param when the browser cannot answer, keeping the server default", () => {
    vi.stubGlobal("AudioContext", undefined);
    vi.stubGlobal("webkitAudioContext", undefined);
    expect(buildTranscodeHlsUrl("m1", "tok", 0, null, 0)).not.toContain("maxAudioChannels");
  });
});

describe("buildTranscodeHlsUrl", () => {
  it("builds URL with token, audioTrack, and codec params", () => {
    const url = buildTranscodeHlsUrl("m1", "tok", 0, null, 0);
    expect(url).toContain("token=tok");
    expect(url).toContain("audioTrack=0");
    expect(url).toContain("videoCodecs=h264");
    expect(url).toContain("audioCodecs=aac%2Cmp3%2Copus");
    expect(url).toContain("containers=mp4");
  });

  it("includes subtitleTrack when burn-in selected", () => {
    const url = buildTranscodeHlsUrl("m1", "tok", 0, 2, 0);
    expect(url).toContain("subtitleTrack=2");
  });

  it("omits hdr on an SDR display, sends hdr=1 on an HDR one", () => {
    // SDR (default mock): the server must keep its conservative SDR default.
    expect(buildTranscodeHlsUrl("m1", "tok", 0, null, 0)).not.toContain("hdr=");
    // HDR-capable display: the decision needs it to allow the HEVC copy rung
    // (and HDR direct-play) instead of forcing a tonemap transcode.
    mockCaps.hdr = true;
    try {
      expect(buildTranscodeHlsUrl("m1", "tok", 0, null, 0)).toContain("hdr=1");
    } finally {
      mockCaps.hdr = false;
    }
  });

  it("includes startSec when > 0", () => {
    const url = buildTranscodeHlsUrl("m1", "tok", 0, null, 120);
    expect(url).toContain("startSec=120");
  });

  it("omits subtitleTrack and startSec when not needed", () => {
    const url = buildTranscodeHlsUrl("m1", "tok", 1, null, 0);
    expect(url).not.toContain("subtitleTrack");
    expect(url).not.toContain("startSec");
  });

  it("omits token param when token is empty string", () => {
    const url = buildTranscodeHlsUrl("m1", "", 0, null, 0);
    expect(url).not.toContain("token=");
  });

  it("includes all params together", () => {
    const url = buildTranscodeHlsUrl("m1", "t", 3, 1, 55);
    expect(url).toContain("token=t");
    expect(url).toContain("audioTrack=3");
    expect(url).toContain("subtitleTrack=1");
    expect(url).toContain("startSec=55");
  });

  it("omits gapless by default (today's per-episode master)", () => {
    const url = buildTranscodeHlsUrl("m1", "tok", 0, null, 0);
    expect(url).not.toContain("gapless");
  });

  it("includes gapless=1 when the caller opts in", () => {
    const url = buildTranscodeHlsUrl("m1", "tok", 0, null, 0, 0, true);
    expect(url).toContain("gapless=1");
  });

  it("omits gapless=1 when explicitly false", () => {
    const url = buildTranscodeHlsUrl("m1", "tok", 0, null, 0, 0, false);
    expect(url).not.toContain("gapless");
  });
});

describe("resolveDefaultAudio", () => {
  const audioStreams = [
    { index: 0, trackIndex: 0, language: "eng", isDefault: true },
    { index: 1, trackIndex: 1, language: "fre" },
    { index: 2, trackIndex: 2, language: "jpn" },
  ];

  it("returns match for global preferred lang", () => {
    const result = resolveDefaultAudio(audioStreams, undefined, () => null, () => "fre");
    expect(result).toEqual(audioStreams[1]);
  });

  it("series pref overrides global pref", () => {
    const result = resolveDefaultAudio(audioStreams, "s1", () => ({ audioLang: "jpn" }), () => "fre");
    expect(result).toEqual(audioStreams[2]);
  });

  it("falls back to default stream when no pref", () => {
    const result = resolveDefaultAudio(audioStreams, undefined, () => null, () => null);
    expect(result).toEqual(audioStreams[0]);
  });

  it("falls back to first stream when none is default", () => {
    const noDefault = audioStreams.map((s) => ({ ...s, isDefault: false }));
    const result = resolveDefaultAudio(noDefault, undefined, () => null, () => null);
    expect(result).toEqual(noDefault[0]);
  });

  it("matches case-insensitively", () => {
    const result = resolveDefaultAudio(audioStreams, undefined, () => null, () => "ENG");
    expect(result).toEqual(audioStreams[0]);
  });

  it("matches on title when language is missing", () => {
    const titleStreams = [
      { index: 0, trackIndex: 0, title: "English" },
      { index: 1, trackIndex: 1, title: "French" },
    ];
    const result = resolveDefaultAudio(titleStreams, undefined, () => null, () => "french");
    expect(result).toEqual(titleStreams[1]);
  });

  it("ignores series pref when seriesId is undefined", () => {
    const result = resolveDefaultAudio(audioStreams, undefined, () => ({ audioLang: "jpn" }), () => "fre");
    expect(result).toEqual(audioStreams[1]);
  });

  // Regression: user preferences are stored as ISO 639-1 (LanguageSettings.tsx
  // writes "fr"), but the manifest carries whatever the source file's mkvgo/
  // ffprobe tags used - almost always ISO 639-2, and for French specifically
  // either the bibliographic "fre" or the terminological "fra" form. A strict
  // string compare between "fr" and "fre"/"fra" never matches, so the user's
  // pick was silently ignored.
  it("matches an ISO 639-1 preference against a 639-2/B stream language (fre)", () => {
    const bStreams = [
      { index: 0, trackIndex: 0, language: "eng" },
      { index: 1, trackIndex: 1, language: "fre" },
    ];
    const result = resolveDefaultAudio(bStreams, undefined, () => null, () => "fr");
    expect(result).toEqual(bStreams[1]);
  });

  it("matches an ISO 639-1 preference against a 639-2/T stream language (fra)", () => {
    const tStreams = [
      { index: 0, trackIndex: 0, language: "eng" },
      { index: 1, trackIndex: 1, language: "fra" },
    ];
    const result = resolveDefaultAudio(tStreams, undefined, () => null, () => "fr");
    expect(result).toEqual(tStreams[1]);
  });

  it("matches an ISO 639-1 preference against a BCP-47 region-tagged stream language (fr-FR)", () => {
    const bcp47Streams = [
      { index: 0, trackIndex: 0, language: "en-US" },
      { index: 1, trackIndex: 1, language: "fr-FR" },
    ];
    const result = resolveDefaultAudio(bcp47Streams, undefined, () => null, () => "fr");
    expect(result).toEqual(bcp47Streams[1]);
  });

  it("does not use a naive 2-letter slice: German (ger/deu -> de) must not collide with Georgian (ge)", () => {
    const streams = [
      { index: 0, trackIndex: 0, language: "ger" },
      { index: 1, trackIndex: 1, language: "geo" },
    ];
    expect(resolveDefaultAudio(streams, undefined, () => null, () => "de")).toEqual(streams[0]);
    expect(resolveDefaultAudio(streams, undefined, () => null, () => "ka")).toEqual(streams[1]);
  });
});

describe("resolveDefaultSubtitle", () => {
  const subs: SubtitleStream[] = [
    { index: 0, trackIndex: 0, language: "eng", codec: "subrip" },
    { index: 1, trackIndex: 1, language: "fre", codec: "subrip" },
  ];

  it("returns null when no preferences at all", () => {
    expect(resolveDefaultSubtitle(subs, undefined, () => null, () => null)).toBeNull();
  });

  it("returns matching global pref", () => {
    expect(resolveDefaultSubtitle(subs, undefined, () => null, () => "fre")).toBe(1);
  });

  it("returns null when series pref explicitly null", () => {
    expect(resolveDefaultSubtitle(subs, "s1", () => ({ subtitleLang: null }), () => "fre")).toBeNull();
  });

  it("series pref overrides global pref", () => {
    expect(resolveDefaultSubtitle(subs, "s1", () => ({ subtitleLang: "eng" }), () => "fre")).toBe(0);
  });

  it("returns null when global pref has no match", () => {
    expect(resolveDefaultSubtitle(subs, undefined, () => null, () => "ger")).toBeNull();
  });

  it("matches case-insensitively", () => {
    expect(resolveDefaultSubtitle(subs, undefined, () => null, () => "FRE")).toBe(1);
  });

  it("returns null when series pref subtitleLang is undefined", () => {
    expect(resolveDefaultSubtitle(subs, "s1", () => ({ subtitleLang: undefined }), () => "fre")).toBeNull();
  });

  it("matches on title when language missing", () => {
    const titleSubs: SubtitleStream[] = [
      { index: 0, trackIndex: 0, title: "English", codec: "subrip" },
      { index: 1, trackIndex: 1, title: "French", codec: "subrip" },
    ];
    expect(resolveDefaultSubtitle(titleSubs, undefined, () => null, () => "french")).toBe(1);
  });

  it("returns null for empty streams with a pref", () => {
    expect(resolveDefaultSubtitle([], undefined, () => null, () => "eng")).toBeNull();
  });
});

// Plan B P2: which browsers get native <video> HLS (Safari macOS laptops + iOS,
// the CPU/battery win) vs hls.js (everything else). Real User-Agent strings
// stand in for a live engine so every device is exercised deterministically -
// the Safari/Mac "simulation" a WebKit binary can't reliably give us on WSL.
describe("computeNativeHLSPreference", () => {
  const UA = {
    safariMac:
      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15",
    chromeMac:
      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
    edgeMac:
      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0",
    firefoxMac:
      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:121.0) Gecko/20100101 Firefox/121.0",
    iosSafari:
      "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
    // iPadOS 13+ reports the desktop-Safari UA; only maxTouchPoints betrays it.
    iPadOS:
      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15",
    iosChrome:
      "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/120.0.0.0 Mobile/15E148 Safari/604.1",
    androidChrome:
      "Mozilla/5.0 (Linux; Android 13; Phone) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36",
  };

  it("prefers native on macOS Safari (laptop battery win)", () => {
    expect(computeNativeHLSPreference(true, UA.safariMac, "MacIntel", 0, true)).toBe(true);
  });

  it("stays on hls.js for macOS Chrome / Edge / Firefox", () => {
    expect(computeNativeHLSPreference(true, UA.chromeMac, "MacIntel", 0, true)).toBe(false);
    expect(computeNativeHLSPreference(true, UA.edgeMac, "MacIntel", 0, true)).toBe(false);
    expect(computeNativeHLSPreference(true, UA.firefoxMac, "MacIntel", 0, true)).toBe(false);
  });

  it("prefers native on iOS Safari", () => {
    expect(computeNativeHLSPreference(true, UA.iosSafari, "iPhone", 0, true)).toBe(true);
  });

  it("prefers native on iPadOS masquerading as desktop Safari (touch-capable)", () => {
    expect(computeNativeHLSPreference(true, UA.iPadOS, "MacIntel", 5, true)).toBe(true);
  });

  it("stays on hls.js for Chrome-on-iOS (CriOS)", () => {
    expect(computeNativeHLSPreference(false, UA.iosChrome, "iPhone", 5, true)).toBe(false);
  });

  it("stays on hls.js for Android Chrome even if it reports HLS support", () => {
    expect(computeNativeHLSPreference(true, UA.androidChrome, "Linux armv8l", 5, true)).toBe(false);
  });

  it("falls back to native when MSE is unavailable (old iOS without hls.js)", () => {
    expect(computeNativeHLSPreference(true, UA.safariMac, "MacIntel", 0, false)).toBe(true);
  });

  it("never native when the element can't play HLS at all", () => {
    expect(computeNativeHLSPreference(false, UA.safariMac, "MacIntel", 0, true)).toBe(false);
  });
});

// Browsers block SOUNDED autoplay, never a muted one. So a refused start is
// retried muted and the mute lifted straight away - the policy gates play(), not
// muted. Stopping at the muted start (as the player once did) is how a title
// opened in a fresh browser played as a silent film with no clue why.
describe("startPlayback", () => {
  function fakeVideo(opts: {
    soundedAutoplay: boolean;   // does the browser allow a sounded play()?
    pauseOnUnmute?: boolean;    // does it re-pause when we lift the mute?
  }): HTMLVideoElement {
    const v = {
      muted: false,
      paused: true,
      play: vi.fn(async () => {
        if (!v.muted && !opts.soundedAutoplay) throw new DOMException("blocked", "NotAllowedError");
        v.paused = false;
      }),
    };
    // Lifting the mute on a media the browser deems autoplaying can re-pause it.
    Object.defineProperty(v, "muted", {
      get: () => v._muted,
      set: (next: boolean) => {
        v._muted = next;
        if (!next && !v.paused && opts.pauseOnUnmute) v.paused = true;
      },
    });
    v._muted = false;
    return v as unknown as HTMLVideoElement;
  }

  it("plays with sound outright when the browser allows it", async () => {
    const video = fakeVideo({ soundedAutoplay: true });

    await expect(startPlayback(video)).resolves.toBe(true);
    expect(video.muted).toBe(false);
    expect(video.paused).toBe(false);
    expect(video.play).toHaveBeenCalledTimes(1); // no muted detour needed
  });

  it("blocked autoplay: starts muted, then gets the sound back", async () => {
    const video = fakeVideo({ soundedAutoplay: false });

    await expect(startPlayback(video)).resolves.toBe(true);
    expect(video.paused, "it must actually be playing").toBe(false);
    expect(video.muted, "the mute is a means, not the end state").toBe(false);
  });

  it("browser re-pauses on unmute: stays muted and PLAYING rather than dead", async () => {
    const video = fakeVideo({ soundedAutoplay: false, pauseOnUnmute: true });

    await expect(startPlayback(video)).resolves.toBe(true);
    expect(video.paused, "muted playback beats no playback").toBe(false);
    expect(video.muted).toBe(true);
  });
});

describe("planCastLoad", () => {
  it("transcode: resume rides the load request, never a baked startSec", () => {
    // No server handler parses startSec, and the transcode manifest is a
    // complete VOD - a baked startSec silently started the receiver at 0
    // while the UI added the offset back and believed the resume had landed.
    const plan = planCastLoad(undefined, "tok", "m1", 0, 742.9, false);
    expect(plan.url).not.toContain("startSec=");
    expect(plan.resumeSec).toBe(742);
  });

  it("transcode: cold start resumes at 0", () => {
    const plan = planCastLoad(undefined, "tok", "m1", 1, 0, true);
    expect(plan.url).toContain("audioTrack=1");
    expect(plan.url).toContain("gapless=1");
    expect(plan.resumeSec).toBe(0);
  });

  it("direct play: appends the session token when the URL has none", () => {
    const plan = planCastLoad("/files/movie.mp4", "tok", "m1", 0, 30, false);
    expect(plan.url).toBe("/files/movie.mp4?token=tok");
    expect(plan.resumeSec).toBe(30);
  });

  it("direct play: keeps a URL that already carries a token", () => {
    const plan = planCastLoad("/files/movie.mp4?token=abc", "tok", "m1", 0, 12.4, false);
    expect(plan.url).toBe("/files/movie.mp4?token=abc");
    expect(plan.resumeSec).toBe(12);
  });
});

// --- Cast button safety net -------------------------------------------------
// The cast button broke silently twice in prod (2026-07-22, 2026-07-24). These
// tests lock its two contracts: the tap routing (one tap = one path) and the
// fling element's preload (Chromium refuses to open the picker while the
// element's duration is NaN). If one of these fails, the button is broken on a
// real phone - fix the regression, not the test.
describe("pickCastTapPath (cast button routing)", () => {
  const base = { castConnected: false, remoteConnected: false, preferFling: false, castSdkReady: false };

  it("a live SDK session always wins the tap and stops itself", () => {
    expect(pickCastTapPath({ ...base, castConnected: true, remoteConnected: true, preferFling: true, castSdkReady: true })).toBe("stop-sdk");
    expect(pickCastTapPath({ ...base, castConnected: true })).toBe("stop-sdk");
  });

  it("a live fling session stops via the hook, never by re-prompting", () => {
    // Re-prompting mid-session reloads the receiver instead of stopping it.
    expect(pickCastTapPath({ ...base, remoteConnected: true, preferFling: true, castSdkReady: true })).toBe("stop-fling");
  });

  it("Android (preferFling) goes to the fling even when the SDK claims ready", () => {
    // Chrome Android reports the SDK as ready but cannot complete a session
    // (measured in prod 2026-07-22: sdk=true then session error, same phone
    // connects fine in desktop-site mode). The fling is the only mobile path.
    expect(pickCastTapPath({ ...base, preferFling: true, castSdkReady: true })).toBe("fling");
    expect(pickCastTapPath({ ...base, preferFling: true })).toBe("fling");
  });

  it("desktop goes to the SDK when ready, else falls back to the fling", () => {
    expect(pickCastTapPath({ ...base, castSdkReady: true })).toBe("sdk");
    expect(pickCastTapPath(base)).toBe("fling");
  });
});

describe("castVideoPreload (fling element preload)", () => {
  it("loads metadata where the fling is the tap's path", () => {
    // Chromium's remote_playback.cc drops every availability URL while the
    // element's duration is NaN, and prompt() then rejects INSTANTLY with
    // NotAllowedError - no picker, 18ms, measured in prod 2026-07-24. With
    // preload="none" the metadata never loaded, so the fling was structurally
    // dead on Android. This value is the fix; do not "optimize" it back.
    expect(castVideoPreload(true)).toBe("metadata");
  });

  it("stays lazy where the SDK owns the tap (desktop, weak-NAS bandwidth)", () => {
    expect(castVideoPreload(false)).toBe("none");
  });
});

describe("pickPlayOnFallback (NAT-proof cast fallback)", () => {
  // Remote users don't live on the server's LAN: their Chromecast is
  // unreachable from the server AND Chrome Android refuses both browser cast
  // APIs. Their own Evey clients (TV app) on the Play On WebSocket are the
  // one channel that works from anywhere - the cast tap must use it.
  it("sends straight to a single connected device - that is what the tap meant", () => {
    const d = { id: "tv1", name: "Salon" };
    expect(pickPlayOnFallback([d])).toEqual({ action: "send", device: d });
  });

  it("never guesses between several devices - route to the picker", () => {
    expect(pickPlayOnFallback([{ id: "a" }, { id: "b" }])).toEqual({ action: "picker" });
  });

  it("offers nothing when no device is connected", () => {
    expect(pickPlayOnFallback([])).toEqual({ action: "none" });
    expect(pickPlayOnFallback(undefined)).toEqual({ action: "none" });
    expect(pickPlayOnFallback(null)).toEqual({ action: "none" });
  });
});

describe("pickTranscodeAudioSwitchPath (audio switch = client-side rendition select)", () => {
  // Le master transcode expose TOUTES les pistes audio en renditions : changer
  // de piste = hls.audioTrack, instantané, découplé du flag DEFAULT (le restart
  // de session rejouait l'ancienne piste tant que le serveur marquait la 1re
  // rendition DEFAULT=YES - « je reste en français »).
  const base = {
    hasHls: true,
    wasmActiveNow: false,
    wasmActiveForTarget: false,
    audioRenditionCount: 2,
    targetTrackIndex: 1,
  };

  it("switches client-side when hls.js already has the target rendition", () => {
    expect(pickTranscodeAudioSwitchPath(base)).toBe("hls-rendition");
  });

  it("restarts on native HLS (Safari): no hls.js instance to drive", () => {
    expect(pickTranscodeAudioSwitchPath({ ...base, hasHls: false })).toBe("restart");
  });

  it("restarts when wasm audio is active now: the engine is wired per session", () => {
    expect(pickTranscodeAudioSwitchPath({ ...base, wasmActiveNow: true })).toBe("restart");
  });

  it("restarts when the TARGET track needs the wasm pipeline", () => {
    expect(pickTranscodeAudioSwitchPath({ ...base, wasmActiveForTarget: true })).toBe("restart");
  });

  it("restarts when the rendition is missing from the parsed manifest", () => {
    expect(pickTranscodeAudioSwitchPath({ ...base, targetTrackIndex: 2 })).toBe("restart");
    expect(pickTranscodeAudioSwitchPath({ ...base, targetTrackIndex: -1 })).toBe("restart");
    expect(pickTranscodeAudioSwitchPath({ ...base, audioRenditionCount: 0 })).toBe("restart");
  });
});

describe("pickPinnedLevel", () => {
  // The bug this closes: the rung pin lived in a useEffect depending on state
  // set INSIDE MANIFEST_PARSED. In between, hls.js starts loading on free ABR
  // and fetches level 0 (the lowest) - a SECOND ffmpeg on the same hash. On a
  // NAS calibrated for one concurrent encode the two 4K decodes starve each
  // other and the first segment never lands (prod 2026-07-25: 480p + 1080p
  // spawned 43 ms apart, segment 0 still missing 9.5 s later).
  it("pins the TOP rung in Auto", () => {
    expect(pickPinnedLevel({ levelHeights: [480, 720, 1080], desiredHeight: 0 })).toBe(2);
  });

  it("pins the chosen rung when the viewer picked one", () => {
    expect(pickPinnedLevel({ levelHeights: [480, 720, 1080], desiredHeight: 720 })).toBe(1);
  });

  it("falls back to the TOP rung when the chosen height is gone from the ladder", () => {
    // A shaped/trimmed ladder can drop the rung a viewer picked earlier. Landing
    // on the top rung keeps ONE encoder; leaving it unpinned re-enables free ABR.
    expect(pickPinnedLevel({ levelHeights: [480, 720], desiredHeight: 1080 })).toBe(1);
  });

  it("pins nothing before the manifest is parsed", () => {
    expect(pickPinnedLevel({ levelHeights: [], desiredHeight: 1080 })).toBe(-1);
  });

  it("pins the only rung of a single-rendition ladder", () => {
    expect(pickPinnedLevel({ levelHeights: [1080], desiredHeight: 0 })).toBe(0);
  });
});

// The mute that sticks after a track switch. `startPlayback` runs on EVERY
// MANIFEST_PARSED, so on every hls.js instance rebuild (audio track, subtitles,
// offset, token refresh). Its last-resort branch deliberately leaves the
// element muted - a fair trade on a cold start ("muted playback beats none"),
// a silent regression when the viewer was already listening. And since the
// video element survives the rebuild, the mute sticks until the viewer touches
// the volume control themselves.
describe("liftForcedMuteOnNextGesture", () => {
  function mutedVideo() {
    return { muted: true } as unknown as HTMLVideoElement;
  }

  it("lifts the forced mute on the next click", () => {
    const video = mutedVideo();
    liftForcedMuteOnNextGesture(video);
    document.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    expect(video.muted).toBe(false);
  });

  it("lifts it on the next keystroke too", () => {
    const video = mutedVideo();
    liftForcedMuteOnNextGesture(video);
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowRight", bubbles: true }));
    expect(video.muted).toBe(false);
  });

  it("disarms after one gesture: a later mute is the viewer's own choice", () => {
    const video = mutedVideo();
    liftForcedMuteOnNextGesture(video);
    document.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    video.muted = true; // the viewer mutes deliberately
    document.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    expect(video.muted, "one lift only, otherwise we fight the viewer").toBe(true);
  });

  it("does not fight the `m` shortcut, which toggles the mute itself", () => {
    // The `m` key toggles the mute. If the recovery lifted it just before, the
    // toggle would put it back - the viewer would press `m` to get the sound
    // back and get the opposite.
    const video = mutedVideo();
    liftForcedMuteOnNextGesture(video);
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "m", bubbles: true }));
    expect(video.muted).toBe(true);
  });

  it("leaves the sound alone when something already restored it", () => {
    // The volume button lifted the mute before the click reached the document.
    const video = { muted: false } as unknown as HTMLVideoElement;
    let touched = false;
    Object.defineProperty(video, "muted", {
      get: () => false,
      set: () => { touched = true; },
    });
    liftForcedMuteOnNextGesture(video);
    document.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    expect(touched, "force nothing when the sound is already back").toBe(false);
  });

  it("can be disarmed explicitly (teardown)", () => {
    const video = mutedVideo();
    const disarm = liftForcedMuteOnNextGesture(video);
    disarm();
    document.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    expect(video.muted).toBe(true);
  });
});
