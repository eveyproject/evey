import { describe, it, expect, vi } from "vitest";
import type { AudioProbe } from "@evey/media-caps";

const getAudioProbeMock = vi.fn();
vi.mock("@evey/media-caps", () => ({
  getAudioProbe: () => getAudioProbeMock(),
}));

import { resolveMaxOutputChannels, probeMaxOutputChannels } from "./outputChannels";

function probe(over: Partial<AudioProbe> = {}): AudioProbe {
  return {
    apiLevel: 34,
    isTv: false,
    deviceModel: "SM-S928B",
    decoders: [],
    directPlayback: {},
    directProfiles: [],
    hdmi: { available: false },
    surroundOutputRaw: -1,
    outputDeviceTypes: [],
    ...over,
  } as AudioProbe;
}

describe("resolveMaxOutputChannels", () => {
  it("declares stereo when every output this device has is a stereo-only type", () => {
    // Phone: built-in speaker + wired headphones + Bluetooth. Nothing here can
    // carry more than two channels, so 5.1 is folded - and clips.
    expect(resolveMaxOutputChannels(probe({ outputDeviceTypes: [2, 4, 8] }))).toBe(2);
    expect(resolveMaxOutputChannels(probe({ outputDeviceTypes: [1, 2] }))).toBe(2);
  });

  it("keeps a plugged multichannel HDMI sink multichannel", () => {
    // The regression this whole change must not cause: an amplifier stays 5.1.
    expect(
      resolveMaxOutputChannels(
        probe({ hdmi: { available: true, plugState: 1, maxChannelCount: 6 }, outputDeviceTypes: [9] }),
      ),
    ).toBe(6);
    expect(
      resolveMaxOutputChannels(
        probe({ hdmi: { available: true, plugState: 1, maxChannelCount: 8 }, outputDeviceTypes: [10] }),
      ),
    ).toBe(8);
  });

  it("declares stereo for a plugged 2.0 television", () => {
    expect(
      resolveMaxOutputChannels(
        probe({ hdmi: { available: true, plugState: 1, maxChannelCount: 2 }, outputDeviceTypes: [9] }),
      ),
    ).toBe(2);
  });

  it("caps an implausible reading at the 8 the server accepts", () => {
    expect(
      resolveMaxOutputChannels(
        probe({ hdmi: { available: true, plugState: 1, maxChannelCount: 32 } }),
      ),
    ).toBe(8);
  });

  it("answers nothing when the platform does not answer", () => {
    // No probe at all (native module absent from this build).
    expect(resolveMaxOutputChannels(null)).toBeUndefined();
    // HDMI present but nothing plugged in AND an output we cannot describe.
    expect(
      resolveMaxOutputChannels(
        probe({ hdmi: { available: true, plugState: 0 }, outputDeviceTypes: [11] }),
      ),
    ).toBeUndefined();
    // Plugged sink that refuses to state its channel count (-1 from the raw
    // broadcast extra): unknown, not stereo.
    expect(
      resolveMaxOutputChannels(
        probe({ hdmi: { available: true, plugState: 1, maxChannelCount: -1 } }),
      ),
    ).toBeUndefined();
    // Empty device list: nothing to conclude from.
    expect(resolveMaxOutputChannels(probe({ outputDeviceTypes: [] }))).toBeUndefined();
  });

  it("treats an output type that MAY be multichannel as unknown, never as stereo", () => {
    // USB device (11) and dock (13) can be multichannel; folding them to 2
    // would silently demote a real surround rig.
    expect(resolveMaxOutputChannels(probe({ outputDeviceTypes: [2, 11] }))).toBeUndefined();
    expect(resolveMaxOutputChannels(probe({ outputDeviceTypes: [13] }))).toBeUndefined();
  });
});

describe("probeMaxOutputChannels", () => {
  it("returns the resolved ceiling from the native probe", async () => {
    getAudioProbeMock.mockResolvedValue(probe({ outputDeviceTypes: [2, 4] }));
    expect(await probeMaxOutputChannels()).toBe(2);
  });

  it("returns undefined when the native module is not in this build", async () => {
    getAudioProbeMock.mockResolvedValue(null);
    expect(await probeMaxOutputChannels()).toBeUndefined();
  });
});
