import { afterEach, describe, expect, it } from "vitest";
import { mseHlsSupported } from "./hlsCodecParams";

// The probe that replaced importing hls.js for one static question: the
// library was half a megabyte on the first page of every client.
describe("mseHlsSupported", () => {
  const realMediaSource = (globalThis as Record<string, unknown>).MediaSource;
  afterEach(() => { (globalThis as Record<string, unknown>).MediaSource = realMediaSource; });

  it("is false where there is no MediaSource at all", () => {
    (globalThis as Record<string, unknown>).MediaSource = undefined;
    expect(mseHlsSupported()).toBe(false);
  });

  it("asks the baseline fMP4 the player would actually append", () => {
    const asked: string[] = [];
    (globalThis as Record<string, unknown>).MediaSource = { isTypeSupported: (t: string) => { asked.push(t); return true; } };
    expect(mseHlsSupported()).toBe(true);
    expect(asked[0]).toContain("avc1.42E01E");
    expect(asked[0]).toContain("mp4a.40.2");
  });

  it("a throwing implementation is a no, not a crash", () => {
    (globalThis as Record<string, unknown>).MediaSource = { isTypeSupported: () => { throw new Error("nope"); } };
    expect(mseHlsSupported()).toBe(false);
  });
});
