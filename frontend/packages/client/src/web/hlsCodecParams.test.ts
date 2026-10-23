import { afterEach, describe, expect, it } from "vitest";
import { useDataSaverStore } from "@evey/store";
import { appendCodecParams, dataSaverStreamInfoParams, mseHlsSupported } from "./hlsCodecParams";

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

// The browser's data saver rides the master URL in kbit/s and the verdict in
// bit/s - GetStreamInfo's own unit - and nothing is sent when it is off.
describe("data saver", () => {
  afterEach(() => useDataSaverStore.setState({ level: "off" }));

  it("puts its ceilings on the master URL", () => {
    useDataSaverStore.setState({ level: "travel" });
    const p = new URLSearchParams();
    appendCodecParams(p);
    expect(p.get("maxBitrate")).toBe("1000");
    expect(p.get("maxHeight")).toBe("480");
    expect(p.get("maxAudioChannels")).toBe("2");
    expect(dataSaverStreamInfoParams()).toEqual({ maxBitrate: "1000000", maxHeight: 480 });
  });

  it("adds nothing when off", () => {
    const p = new URLSearchParams();
    appendCodecParams(p);
    expect(p.has("maxBitrate")).toBe(false);
    expect(p.has("maxHeight")).toBe(false);
    expect(dataSaverStreamInfoParams()).toEqual({});
  });
});
