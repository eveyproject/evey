import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";

const eveyFetchMock = vi.fn();
const reportOnce = vi.fn();
vi.mock("@evey/api", () => ({ eveyFetch: (...args: unknown[]) => eveyFetchMock(...args) }));
vi.mock("@evey/media-caps", () => ({
  getAudioProbe: vi.fn(),
  reportAudioProbeOnce: (...args: unknown[]) => reportOnce(...args),
}));

import { useAudioCapsBeacon } from "./useAudioCapsBeacon";

beforeEach(() => {
  eveyFetchMock.mockReset().mockResolvedValue(undefined);
  reportOnce.mockReset().mockResolvedValue("sent");
});

// The beacon is a diagnostic player event on the media being played: it
// runs through the once-per-launch reporter, posts to that media's
// player-event route with a non-fatal frame, and never fires without a media.
describe("useAudioCapsBeacon", () => {
  it("posts the probe as a non-fatal player event for the media", async () => {
    renderHook(() => useAudioCapsBeacon("m 1"));
    expect(reportOnce).toHaveBeenCalledTimes(1);
    const post = reportOnce.mock.calls[0][1] as (body: Record<string, unknown>) => Promise<unknown>;
    await post({ probe: { directPlayback: { eac3: true } } });
    expect(eveyFetchMock).toHaveBeenCalledWith({
      url: "/api/v1/media/m%201/player-event",
      method: "POST",
      data: { probe: { directPlayback: { eac3: true } }, fatal: false, code: 0, variant: 0, segment: 0, posSec: 0 },
    });
  });

  it("does nothing without a media id", () => {
    renderHook(() => useAudioCapsBeacon(""));
    expect(reportOnce).not.toHaveBeenCalled();
  });
});
