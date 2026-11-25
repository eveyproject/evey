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

  // One chip for every resolution: 4K and 1080p differ by their text only.
  it("dresses every resolution the same", () => {
    const { rerender } = render(<QualityChips res="4K" hdr="HDR" />);
    const uhd = screen.getByTestId("resolution-badge");
    const look = [uhd.className, uhd.getAttribute("style")];
    rerender(<QualityChips res="1080p" hdr={null} />);
    const fhd = screen.getByTestId("resolution-badge");
    expect([fhd.className, fhd.getAttribute("style")]).toEqual(look);
    expect(screen.queryByTestId("hdr-badge")).toBeNull();
  });

  it("draws nothing for a title without a video stream", () => {
    render(<QualityChips res={null} hdr={null} />);
    expect(screen.queryByTestId("quality-chips")).toBeNull();
  });
});
