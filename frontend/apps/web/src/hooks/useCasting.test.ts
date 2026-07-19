// loadChromecastMedia is the single funnel every cast load goes through.
// These tests pin the LoadRequest it hands the receiver: autoplay always on
// (casting while the local player is paused must still play on the TV), the
// fMP4 hints on HLS, and - the resume bug - currentTime carrying the local
// position for DIRECT PLAY, where the receiver owns the timeline and nobody
// else will ever seek it (a transcode bakes the offset server-side instead).
import { describe, it, expect, vi, beforeEach } from "vitest";

import { loadChromecastMedia, isDirectLanHost } from "./useCasting";

// The server-side Cast/DLNA fallback discovers devices on the SERVER's LAN.
// That only overlaps the user's LAN when Evey is reached directly by a bare
// LAN IP (the server is on the same network). Behind a domain (reverse proxy,
// Docker bridge) the server's LAN is not the user's, so the fallback can only
// return [] and the "no devices on the server's network" toast is misleading -
// it must be skipped entirely there.
describe("isDirectLanHost", () => {
  // Addresses below are RFC 1918 private ranges / RFC 5737 documentation
  // blocks / RFC 2606 reserved names - generic examples, never a real host.
  it("treats bare IPv4 and loopback as same-LAN (fallback allowed)", () => {
    for (const h of ["192.0.2.10", "198.51.100.5", "203.0.113.9", "127.0.0.1"]) {
      expect(isDirectLanHost(h)).toBe(true);
    }
  });
  it("treats localhost and IPv6 literals as same-LAN", () => {
    for (const h of ["localhost", "::1", "fe80::1", "2001:db8::1"]) {
      expect(isDirectLanHost(h)).toBe(true);
    }
  });
  it("treats any domain as NOT same-LAN (fallback suppressed)", () => {
    for (const h of ["example.com", "media.example.org", "server.example", "nas.local"]) {
      expect(isDirectLanHost(h)).toBe(false);
    }
  });
});

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
