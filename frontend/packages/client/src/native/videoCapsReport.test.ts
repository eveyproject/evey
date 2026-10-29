import { describe, it, expect } from "vitest";
import { formatVideoCaps } from "./videoCapsReport";

describe("formatVideoCaps", () => {
  it("puts the level, the frames and the size-rate answers on one line", () => {
    expect(
      formatVideoCaps({
        maxH264Level: 51,
        limits: {
          maxWidth: 4096,
          maxHeight: 2304,
          tenBitCodecs: ["hevc"],
          codecs: {
            h264: { maxWidth: 4096, maxHeight: 2304, sizeRate: { "2160p60": false, "2160p30": true, "1080p60": true } },
            hevc: { maxWidth: 4096, maxHeight: 2304 },
          },
        },
      }),
    ).toBe("h264_level=51 max=4096x2304 10bit=hevc | h264 4096x2304 2160p60=no 2160p30=yes 1080p60=yes | hevc 4096x2304");
  });

  it("says what is unknown instead of inventing it", () => {
    expect(formatVideoCaps({ maxH264Level: undefined, limits: undefined })).toBe("h264_level=unknown limits=unknown");
  });
});
