import { describe, it, expect } from "vitest";
import { displaySupportsHdr } from "./displayHdr";

describe("displaySupportsHdr", () => {
  it("declares HDR for an HDR10 / HLG / HDR10+ panel", () => {
    expect(displaySupportsHdr(["hdr10"])).toBe(true);
    expect(displaySupportsHdr(["hlg"])).toBe(true);
    expect(displaySupportsHdr(["hdr10plus"])).toBe(true);
    expect(displaySupportsHdr(["hdr10", "hlg", "dovi"])).toBe(true);
  });

  // The server carries one bool and reads it as "HDR10-capable"; a DoVi source
  // tone-maps server-side regardless. Declaring from DoVi alone would claim a
  // capability nothing evidenced.
  it("refuses to declare HDR from Dolby Vision alone", () => {
    expect(displaySupportsHdr(["dovi"])).toBe(false);
  });

  // The Android TV box of the living room, driving a old SDR panel: supportedHdrTypes=[]
  // on every mode. The tone-map must stay.
  it("treats an SDR panel and a missing probe the same way", () => {
    expect(displaySupportsHdr([])).toBe(false);
    expect(displaySupportsHdr(null)).toBe(false);
    expect(displaySupportsHdr(undefined)).toBe(false);
  });

  it("is case-insensitive on the platform's spelling", () => {
    expect(displaySupportsHdr(["HDR10"])).toBe(true);
  });

  it("ignores a type it does not know", () => {
    expect(displaySupportsHdr(["hdr-vivid"])).toBe(false);
  });
});
