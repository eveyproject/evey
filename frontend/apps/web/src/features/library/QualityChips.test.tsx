import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { QualityChips, qualityOf } from "./QualityChips";

const video = (o: Record<string, unknown>) => [{ type: "MEDIA_STREAM_TYPE_VIDEO" as const, ...o }];

describe("QualityChips", () => {
  it("names the resolution and the HDR flavour off the video stream", () => {
    expect(qualityOf(video({ width: 3840, height: 2160, colorTransfer: "smpte2084" }))).toEqual({ res: "4K", hdr: "HDR" });
    expect(qualityOf(video({ width: 1920, height: 1080, colorTransfer: "arib-std-b67" }))).toEqual({ res: "1080p", hdr: "HLG" });
    expect(qualityOf(video({ width: 1280, height: 720, colorTransfer: "bt709" }))).toEqual({ res: "720p", hdr: null });
    expect(qualityOf(undefined)).toEqual({ res: null, hdr: null });
  });

  it("sets 4K apart from the other resolutions", () => {
    const { rerender } = render(<QualityChips res="4K" hdr="HDR" />);
    expect(screen.getByTestId("resolution-badge").className).toContain("evey-quality-4k");
    rerender(<QualityChips res="1080p" hdr={null} />);
    expect(screen.getByTestId("resolution-badge").className).not.toContain("evey-quality-4k");
    expect(screen.queryByTestId("hdr-badge")).toBeNull();
  });

  it("draws nothing for a title without a video stream", () => {
    render(<QualityChips res={null} hdr={null} />);
    expect(screen.queryByTestId("quality-chips")).toBeNull();
  });
});
