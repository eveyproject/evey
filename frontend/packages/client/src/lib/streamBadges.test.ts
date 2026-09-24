import { describe, expect, it } from "vitest";

describe("the labels every client prints", () => {
  it("names a sub-720 file by its height, as every surface does (the phone's library card said SD)", async () => {
    const { resolutionLabel } = await import("./streamBadges");
    expect(resolutionLabel({ height: 576 } as never)).toBe("576p");
    expect(resolutionLabel({ height: 2160 } as never)).toBe("4K");
  });

  it("tells HDR10 from HLG, and reads a legacy BT.2020 10-bit row as HDR10", async () => {
    const { hdrLabel, isHdrStream } = await import("./streamBadges");
    expect(hdrLabel({ colorTransfer: "arib-std-b67" } as never)).toBe("HLG");
    expect(hdrLabel({ colorTransfer: "smpte2084" } as never)).toBe("HDR10");
    expect(hdrLabel({ colorSpace: "bt2020nc", bitDepth: 10 } as never)).toBe("HDR10");
    expect(isHdrStream({ colorSpace: "bt709", bitDepth: 8 } as never)).toBe(false);
  });
});
