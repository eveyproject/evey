import { describe, it, expect, vi, beforeEach } from "vitest";
import { reportAudioProbeOnce, resetAudioProbeLatch } from "./reportProbe";
import type { AudioProbe } from "./audioProbe";

const aProbe: AudioProbe = {
  apiLevel: 34,
  isTv: true,
  deviceModel: "TV Model A",
  decoders: ["aac"],
  directPlayback: { ac3: true, eac3: true },
  directProfiles: [],
  hdmi: { available: true, plugState: 1, encodings: [5, 6], maxChannelCount: 8 },
  surroundOutputRaw: 1,
  outputDeviceTypes: [9],
};

describe("reportAudioProbeOnce", () => {
  beforeEach(() => resetAudioProbeLatch());

  it("sends one beacon carrying the formatted evidence", async () => {
    const post = vi.fn().mockResolvedValue(undefined);
    expect(await reportAudioProbeOnce(async () => aProbe, post)).toBe("sent");
    expect(post).toHaveBeenCalledTimes(1);
    expect(post.mock.calls[0][0].type).toBe("audio_caps_probe");
    expect(post.mock.calls[0][0].details).toContain("dp=ac3+eac3");
  });

  it("sends only once per app launch", async () => {
    const post = vi.fn().mockResolvedValue(undefined);
    await reportAudioProbeOnce(async () => aProbe, post);
    expect(await reportAudioProbeOnce(async () => aProbe, post)).toBe("skipped-already-sent");
    expect(post).toHaveBeenCalledTimes(1);
  });

  // A build without the native module must stay eligible: latching here would
  // mean an app that installs the module later never reports at all.
  it("does not latch when there is no native module", async () => {
    const post = vi.fn().mockResolvedValue(undefined);
    expect(await reportAudioProbeOnce(async () => null, post)).toBe("skipped-no-probe");
    expect(post).not.toHaveBeenCalled();
    expect(await reportAudioProbeOnce(async () => aProbe, post)).toBe("sent");
  });

  it("swallows a throwing probe instead of breaking playback", async () => {
    const post = vi.fn().mockResolvedValue(undefined);
    const outcome = await reportAudioProbeOnce(async () => { throw new Error("boom"); }, post);
    expect(outcome).toBe("skipped-no-probe");
    expect(post).not.toHaveBeenCalled();
  });

  it("swallows a failing post, and does not retry it on the next play", async () => {
    const post = vi.fn().mockRejectedValue(new Error("offline"));
    expect(await reportAudioProbeOnce(async () => aProbe, post)).toBe("post-failed");
    expect(await reportAudioProbeOnce(async () => aProbe, post)).toBe("skipped-already-sent");
    expect(post).toHaveBeenCalledTimes(1);
  });

  it("posts once even when two callers race the probe", async () => {
    // The latch can only be set after the probe resolves, so both callers used
    // to clear the check and both post - a duplicate beacon in the admin log.
    let releaseProbe: (v: AudioProbe) => void = () => {};
    const probeFn = vi.fn(() => new Promise<AudioProbe | null>((res) => { releaseProbe = res; }));
    const post = vi.fn(async () => ({}));

    const first = reportAudioProbeOnce(probeFn, post);
    const second = reportAudioProbeOnce(probeFn, post);
    releaseProbe(aProbe);

    const [a, b] = await Promise.all([first, second]);
    expect(post).toHaveBeenCalledTimes(1);
    expect(probeFn).toHaveBeenCalledTimes(1);
    expect([a, b]).toEqual(["sent", "sent"]);
  });
});
