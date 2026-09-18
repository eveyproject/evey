// loadChromecastMedia is the single funnel every cast load goes through.
// These tests pin the LoadRequest it hands the receiver: autoplay always on
// (casting while the local player is paused must still play on the TV), the
// fMP4 hints on HLS, and - the resume bug - currentTime carrying the local
// position for DIRECT PLAY, where the receiver owns the timeline and nobody
// else will ever seek it (a transcode bakes the offset server-side instead).
import { describe, it, expect, vi, beforeEach } from "vitest";

import { renderHook, act } from "@testing-library/react";

import { askReceiverCaps, CAPS_NAMESPACE, loadChromecastMedia, useCasting, type CastTarget } from "./useCasting";

class FakeMediaInfo {
  contentId: string;
  contentType: string;
  streamType?: string;
  metadata?: unknown;
  hlsVideoSegmentFormat?: string;
  hlsSegmentFormat?: string;
  constructor(contentId: string, contentType: string) {
    this.contentId = contentId;
    this.contentType = contentType;
  }
}

class FakeLoadRequest {
  media: FakeMediaInfo;
  autoplay?: boolean;
  currentTime?: number;
  constructor(media: FakeMediaInfo) {
    this.media = media;
  }
}

function installCastGlobals() {
  (window as any).chrome = {
    cast: {
      AutoJoinPolicy: { ORIGIN_SCOPED: "origin_scoped" },
      media: {
        MediaInfo: FakeMediaInfo,
        LoadRequest: FakeLoadRequest,
        GenericMediaMetadata: class {},
        StreamType: { BUFFERED: "BUFFERED" },
        HlsVideoSegmentFormat: { FMP4: "FMP4" },
        HlsSegmentFormat: { FMP4: "FMP4" },
      },
    },
  };
}

function fakeSession() {
  let captured: FakeLoadRequest | null = null;
  const session = {
    loadMedia: vi.fn(async (req: FakeLoadRequest) => {
      captured = req;
    }),
  };
  return { session: session as any, get captured() { return captured; } };
}

describe("loadChromecastMedia", () => {
  beforeEach(installCastGlobals);

  it("resumes a direct-play cast at the local position via LoadRequest.currentTime", async () => {
    const s = fakeSession();
    await loadChromecastMedia(s.session, "http://host/media/m1/stream.mp4?token=t", "Film", 542);
    expect(s.captured).not.toBeNull();
    expect(s.captured!.currentTime).toBe(542);
    expect(s.captured!.autoplay).toBe(true);
    expect(s.captured!.media.contentType).toBe("video/mp4");
  });

  it("leaves currentTime unset at position 0 and keeps the fMP4 hints on HLS", async () => {
    const s = fakeSession();
    await loadChromecastMedia(s.session, "http://host/api/v1/media/m1/hls/master.m3u8?token=t", "Film", 0);
    expect(s.captured!.currentTime).toBeUndefined();
    expect(s.captured!.autoplay).toBe(true);
    expect(s.captured!.media.contentType).toBe("application/x-mpegurl");
    expect(s.captured!.media.hlsVideoSegmentFormat).toBe("FMP4");
    expect(s.captured!.media.hlsSegmentFormat).toBe("FMP4");
  });
});

// askReceiverCaps is the only capability signal the SDK path has: the Web
// Sender SDK exposes no model name, so the sender asks OUR receiver what it
// observed. A receiver that does not answer (older build, Default Media
// Receiver) must resolve to null within the timeout - the caller then
// negotiates on the floor - and the listener must never leak.
describe("askReceiverCaps", () => {
  function fakeChannel(reply?: (ns: string, msg: object) => string | null) {
    const listeners = new Set<(ns: string, message: string) => void>();
    const session = {
      addMessageListener: vi.fn((_ns: string, l: (ns: string, message: string) => void) => { listeners.add(l); }),
      removeMessageListener: vi.fn((_ns: string, l: (ns: string, message: string) => void) => { listeners.delete(l); }),
      sendMessage: vi.fn(async (ns: string, msg: object) => {
        const answer = reply?.(ns, msg);
        if (answer !== null && answer !== undefined) {
          queueMicrotask(() => { for (const l of listeners) l(ns, answer); });
        }
      }),
    };
    return { session, listeners };
  }
  const observed = { videoCodecs: ["h264", "hevc"], maxHeight: 2160, hdr: true, nativeDolby: false, maxAudioChannels: 2, maxH264Level: 51, h264MaxHeight: 1080 };

  it("asks on the caps namespace and returns the observed caps verbatim, user agent as diagnostic", async () => {
    const { session, listeners } = fakeChannel(() => JSON.stringify({ type: "caps", caps: observed, userAgent: "CrKey/1.56" }));
    const got = await askReceiverCaps(session, 1000);
    expect(session.sendMessage).toHaveBeenCalledWith(CAPS_NAMESPACE, { type: "caps" });
    expect(got).toEqual({ caps: observed, userAgent: "CrKey/1.56" });
    expect(listeners.size, "listener removed once answered").toBe(0);
  });

  it("times out to null when the receiver never answers (older build, DMR)", async () => {
    vi.useFakeTimers();
    try {
      const { session, listeners } = fakeChannel(() => null);
      const p = askReceiverCaps(session, 1500);
      await vi.advanceTimersByTimeAsync(1500);
      expect(await p).toBeNull();
      expect(listeners.size).toBe(0);
    } finally {
      vi.useRealTimers();
    }
  });

  it("ignores a malformed answer and keeps waiting; a send failure resolves null", async () => {
    vi.useFakeTimers();
    try {
      const { session } = fakeChannel(() => JSON.stringify({ type: "caps", caps: { maxHeight: "big" } }));
      const p = askReceiverCaps(session, 500);
      await vi.advanceTimersByTimeAsync(500);
      expect(await p).toBeNull();
    } finally {
      vi.useRealTimers();
    }
    const failing = fakeChannel();
    failing.session.sendMessage = vi.fn(async () => { throw new Error("channel closed"); });
    expect(await askReceiverCaps(failing.session, 1000)).toBeNull();
  });

  it("fills the optional fields a receiver may omit with the conservative values", async () => {
    const { session } = fakeChannel(() => JSON.stringify({ type: "caps", caps: { videoCodecs: ["h264"], maxHeight: 1080 } }));
    const got = await askReceiverCaps(session, 1000);
    expect(got?.caps).toEqual({ videoCodecs: ["h264"], maxHeight: 1080, hdr: false, nativeDolby: false, maxAudioChannels: 2, maxH264Level: 40, h264MaxHeight: undefined });
    expect(got?.userAgent).toBeNull();
  });
});

// startCast negotiates the stream AFTER it knows the device: the resolver is
// called with what the session learned (the receiver's answer, or none) and
// only then is media loaded - never with a URL computed before the tap.
describe("startCast resolves the URL for the connected target", () => {
  beforeEach(() => {
    installCastGlobals();
    localStorage.clear();
  });

  function installSdk(answer: object | null) {
    const listeners = new Set<(ns: string, message: string) => void>();
    const session = {
      getCastDevice: () => ({ friendlyName: "Salon" }),
      addMessageListener: (_ns: string, l: (ns: string, message: string) => void) => { listeners.add(l); },
      removeMessageListener: (_ns: string, l: (ns: string, message: string) => void) => { listeners.delete(l); },
      sendMessage: vi.fn(async (ns: string) => {
        if (answer) queueMicrotask(() => { for (const l of listeners) l(ns, JSON.stringify(answer)); });
      }),
      loadMedia: vi.fn(async () => {}),
    };
    (window as any).cast = {
      framework: {
        CastContext: { getInstance: () => ({
          getCurrentSession: () => session,
          requestSession: vi.fn(async () => {}),
          setOptions: () => {},
          addEventListener: () => {},
          removeEventListener: () => {},
        }) },
        CastContextEventType: { CAST_STATE_CHANGED: "cs", SESSION_STATE_CHANGED: "ss" },
        RemotePlayer: class {},
        RemotePlayerController: class { addEventListener() {} removeEventListener() {} },
        RemotePlayerEventType: {},
      },
    };
    (window as any).__onGCastApiAvailable?.(true);
    return session;
  }

  it("SDK path: the receiver's observed caps reach the resolver, then loadMedia gets ITS url", async () => {
    const observed = { videoCodecs: ["h264", "hevc"], maxHeight: 2160, hdr: false, nativeDolby: false, maxAudioChannels: 2, maxH264Level: 51, h264MaxHeight: 1080 };
    const session = installSdk({ type: "caps", caps: observed, userAgent: "CrKey" });
    const { result } = renderHook(() => useCasting());
    await act(async () => { (window as any).__onGCastApiAvailable?.(true); });
    const targets: CastTarget[] = [];
    const resolver = vi.fn(async (t: CastTarget) => { targets.push(t); return { url: "/api/v1/media/m1/hls/master.m3u8?token=c", castResumeSec: 42 }; });

    let outcome = "";
    await act(async () => { outcome = await result.current.startCast(resolver, "Film"); });

    expect(outcome).toBe("connected");
    expect(targets).toEqual([{ kind: "sdk", caps: observed, userAgent: "CrKey" }]);
    const req = session.loadMedia.mock.calls[0][0] as FakeLoadRequest;
    expect(req.media.contentId).toBe(`${window.location.origin}/api/v1/media/m1/hls/master.m3u8?token=c`);
    expect(req.currentTime, "resume rides the load request").toBe(42);
    expect(req.media.contentType).toBe("application/x-mpegurl");
  });

  it("SDK path: no answer from the receiver hands the resolver caps=null (the floor), still loads", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      const session = installSdk(null);
      const { result } = renderHook(() => useCasting());
      await act(async () => { (window as any).__onGCastApiAvailable?.(true); });
      const resolver = vi.fn(async () => ({ url: "/api/v1/media/m1/hls/master.m3u8?token=c", castResumeSec: 0 }));
      let outcome = "";
      await act(async () => {
        const p = result.current.startCast(resolver, "Film");
        await vi.advanceTimersByTimeAsync(1600);
        outcome = await p;
      });
      expect(outcome).toBe("connected");
      expect(resolver).toHaveBeenCalledWith({ kind: "sdk", caps: null, userAgent: null });
      expect(session.loadMedia).toHaveBeenCalledTimes(1);
    } finally {
      vi.useRealTimers();
    }
  });
});
