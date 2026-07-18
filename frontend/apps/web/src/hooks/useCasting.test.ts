// loadChromecastMedia is the single funnel every cast load goes through.
// These tests pin the LoadRequest it hands the receiver: autoplay always on
// (casting while the local player is paused must still play on the TV), the
// fMP4 hints on HLS, and - the resume bug - currentTime carrying the local
// position for DIRECT PLAY, where the receiver owns the timeline and nobody
// else will ever seek it (a transcode bakes the offset server-side instead).
import { describe, it, expect, vi, beforeEach } from "vitest";

import { loadChromecastMedia } from "./useCasting";

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
