import { describe, expect, it } from "vitest";

describe("the labels every client prints", () => {
  it("names a sub-720 file by its height, as every surface does (the phone's library card said SD)", async () => {
    const { resolutionLabel } = await import("./streamBadges");
    expect(resolutionLabel({ height: 576 } as never)).toBe("576p");
    expect(resolutionLabel({ height: 2160 } as never)).toBe("4K");
  });

  it("names a cropped scope or flat frame by its width's tier, not its height", async () => {
    const { resolutionLabel, resolutionBand } = await import("./streamBadges");
    expect(resolutionLabel({ width: 1920, height: 818 } as never)).toBe("1080p");
    expect(resolutionLabel({ width: 1916, height: 798 } as never)).toBe("1080p");
    expect(resolutionLabel({ width: 3840, height: 1600 } as never)).toBe("4K");
    expect(resolutionLabel({ width: 1280, height: 534 } as never)).toBe("720p");
    expect(resolutionLabel({ width: 720, height: 480 } as never)).toBe("480p");
    expect(resolutionBand(1920, 818)).toBe("1080p");
    expect(resolutionBand(2560, 1080)).toBe("1440p");
    expect(resolutionBand(720, 576)).toBe("SD");
  });

  it("tells HDR10 from HLG, and reads a legacy BT.2020 10-bit row as HDR10", async () => {
    const { hdrLabel, isHdrStream } = await import("./streamBadges");
    expect(hdrLabel({ colorTransfer: "arib-std-b67" } as never)).toBe("HLG");
    expect(hdrLabel({ colorTransfer: "smpte2084" } as never)).toBe("HDR10");
    expect(hdrLabel({ colorSpace: "bt2020nc", bitDepth: 10 } as never)).toBe("HDR10");
    expect(isHdrStream({ colorSpace: "bt709", bitDepth: 8 } as never)).toBe(false);
  });
});
