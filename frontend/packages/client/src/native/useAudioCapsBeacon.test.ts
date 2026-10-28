import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";

const eveyFetchMock = vi.fn();
const reportOnce = vi.fn();
vi.mock("@evey/api", () => ({ eveyFetch: (...args: unknown[]) => eveyFetchMock(...args) }));
vi.mock("@evey/media-caps", async () => {
  const guard = await vi.importActual<typeof import("../../../media-caps/src/decoderGuard")>(
    "../../../media-caps/src/decoderGuard",
  );
  return {
    formatAbnormalExit: guard.formatAbnormalExit,
    formatDecoderCheck: guard.formatDecoderCheck,
    getAudioProbe: vi.fn(),
    reportAudioProbeOnce: (...args: unknown[]) => reportOnce(...args),
  };
});
vi.mock("./deviceCodecs", () => ({ getDecoderCheck: () => Promise.resolve(null), getVideoCapsReport: () => Promise.resolve(null) }));

import { reportDecoderCheckOnce, reportVideoCapsOnce, resetDecoderCheckLatch, useAudioCapsBeacon } from "./useAudioCapsBeacon";

beforeEach(() => {
  resetDecoderCheckLatch();
  eveyFetchMock.mockReset().mockResolvedValue(undefined);
  reportOnce.mockReset().mockResolvedValue("sent");
});

// The beacon is a diagnostic player event on the media being played: it
// runs through the once-per-launch reporter, posts to that media's
// player-event route with a non-fatal frame, and never fires without a media.
describe("useAudioCapsBeacon", () => {
  it("posts the probe as a non-fatal player event for the media", async () => {
    renderHook(() => useAudioCapsBeacon("m 1"));
    expect(reportOnce).toHaveBeenCalledTimes(1);
    const post = reportOnce.mock.calls[0][1] as (body: Record<string, unknown>) => Promise<unknown>;
    await post({ probe: { directPlayback: { eac3: true } } });
    expect(eveyFetchMock).toHaveBeenCalledWith({
      url: "/api/v1/media/m%201/player-event",
      method: "POST",
      data: { probe: { directPlayback: { eac3: true } }, fatal: false, code: 0, variant: 0, segment: 0, posSec: 0 },
    });
  });

  it("does nothing without a media id", () => {
    renderHook(() => useAudioCapsBeacon(""));
    expect(reportOnce).not.toHaveBeenCalled();
  });
});

// The one trace a crash on a television out of reach leaves: the next launch
// reports it, once, ahead of the decoder verdict.
describe("reportDecoderCheckOnce", () => {
  it("reports a death during decoding, then the check, once per launch", async () => {
    const post = vi.fn().mockResolvedValue(undefined);
    const check = Promise.resolve({
      decoders: [],
      blockedBy: "suspect:exit:crash_native",
      bench: { rtf: 0.21, rtfCold: 0.3, rtfMax: 0.25, maxChannels: 2, wallMs: 900, slow: false },
      abnormalExit: { kind: "playback", reason: "crash_native", status: 6, suspect: true, since: 0, version: "0.1.12" },
    });
    await reportDecoderCheckOnce(check, post);
    await reportDecoderCheckOnce(check, post);
    expect(post.mock.calls.map((c) => c[0])).toEqual([
      {
        type: "ac3go_abnormal_exit",
        details: "kind=playback reason=crash_native status=6 suspect=true since=1970-01-01T00:00:00.000Z version=0.1.12",
        fatal: true,
      },
      {
        type: "ac3go_check",
        details: "declared=none blocked=suspect:exit:crash_native rtf=0.210 cold=0.300 max=0.250 ch=2 wall=900ms",
        fatal: false,
      },
    ]);
  });

  it("sends nothing on a build without the check", async () => {
    const post = vi.fn();
    await reportDecoderCheckOnce(Promise.resolve(null), post);
    expect(post).not.toHaveBeenCalled();
  });
});

// What a decoder error is read against: the decoders' own declaration, sent
// once per launch, off Android nothing.
describe("reportVideoCapsOnce", () => {
  it("reports the decoders' declaration once per launch", async () => {
    resetDecoderCheckLatch();
    const post = vi.fn().mockResolvedValue(undefined);
    const report = Promise.resolve({
      maxH264Level: 51,
      limits: { maxWidth: 3840, maxHeight: 2160, tenBitCodecs: [], codecs: { h264: { maxWidth: 3840, maxHeight: 2160, sizeRate: { "2160p60": false } } } },
    });
    await reportVideoCapsOnce(report, post);
    await reportVideoCapsOnce(report, post);
    expect(post.mock.calls.map((c) => c[0])).toEqual([
      { type: "video_caps", details: "h264_level=51 max=3840x2160 10bit=none | h264 3840x2160 2160p60=no", fatal: false },
    ]);
  });

  it("sends nothing where there is nothing to report", async () => {
    resetDecoderCheckLatch();
    const post = vi.fn();
    await reportVideoCapsOnce(Promise.resolve(null), post);
    expect(post).not.toHaveBeenCalled();
  });
});
