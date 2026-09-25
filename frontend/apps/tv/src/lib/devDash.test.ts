import { afterEach, describe, expect, it } from "vitest";
import { dashUrlFor, devDashEnabled } from "./devDash";

describe("devDash", () => {
  const saved = process.env.EXPO_PUBLIC_EVEY_DEV_DASH;
  afterEach(() => {
    process.env.EXPO_PUBLIC_EVEY_DEV_DASH = saved;
  });

  it("is off unless the dev bundle says 1", () => {
    delete process.env.EXPO_PUBLIC_EVEY_DEV_DASH;
    expect(devDashEnabled()).toBe(false);
    process.env.EXPO_PUBLIC_EVEY_DEV_DASH = "1";
    expect(devDashEnabled()).toBe(true);
  });

  it("asks for the manifest.mpd of the very master, and of nothing else", () => {
    expect(dashUrlFor("http://h/api/v1/media/m1/hls/master.m3u8?token=t&audioTrack=1"))
      .toBe("http://h/api/v1/media/m1/hls/manifest.mpd?token=t&audioTrack=1");
    expect(dashUrlFor("http://h/api/v1/media/m1/stream?token=t")).toBeNull();
  });
});
