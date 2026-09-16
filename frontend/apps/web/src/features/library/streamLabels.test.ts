import { describe, it, expect } from "vitest";
import type { V1MediaStream } from "@evey/api";
import { videoStream, hdrLabel, resolutionLabel, channelLabel } from "./streamLabels";

const video = (extra: Partial<V1MediaStream> = {}): V1MediaStream =>
  ({ type: "MEDIA_STREAM_TYPE_VIDEO", ...extra }) as V1MediaStream;

describe("streamLabels", () => {
  it("picks the video stream, none when absent", () => {
    const audio = { type: "MEDIA_STREAM_TYPE_AUDIO" } as V1MediaStream;
    expect(videoStream([audio, video({ height: 1080 })])?.height).toBe(1080);
    expect(videoStream([audio])).toBeUndefined();
    expect(videoStream(undefined)).toBeUndefined();
  });

  // Mirrors the server's DetectHDR: the transfer decides; a legacy row with
  // only BT.2020 + 10-bit reads as the conservative HDR10.
  it("labels HDR from the color transfer, then from BT.2020 + 10-bit", () => {
    expect(hdrLabel(video({ colorTransfer: " SMPTE2084 " }))).toBe("HDR10");
    expect(hdrLabel(video({ colorTransfer: "arib-std-b67" }))).toBe("HLG");
    expect(hdrLabel(video({ colorSpace: "bt2020nc", bitDepth: 10 }))).toBe("HDR10");
    expect(hdrLabel(video({ colorSpace: "bt2020nc", bitDepth: 8 }))).toBeNull();
    expect(hdrLabel(video({ colorSpace: "bt709", bitDepth: 10 }))).toBeNull();
    expect(hdrLabel(video({ colorTransfer: "bt709" }))).toBeNull();
    expect(hdrLabel(undefined)).toBeNull();
  });

  it("names the resolution tier by height, exact height below 720p", () => {
    expect(resolutionLabel(video({ height: 2160 }))).toBe("4K");
    expect(resolutionLabel(video({ height: 1600 }))).toBe("1440p");
    expect(resolutionLabel(video({ height: 1080 }))).toBe("1080p");
    expect(resolutionLabel(video({ height: 800 }))).toBe("720p");
    expect(resolutionLabel(video({ height: 576 }))).toBe("576p");
    expect(resolutionLabel(video({ height: 0 }))).toBeNull();
    expect(resolutionLabel(undefined)).toBeNull();
  });

  it("returns null, not an empty chip, when the channel count is unknown", () => {
    expect(channelLabel(undefined)).toBeNull();
    expect(channelLabel(0)).toBeNull();
    expect(channelLabel(6)).toBeTruthy();
  });
});
