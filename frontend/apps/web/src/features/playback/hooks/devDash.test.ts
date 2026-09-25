import { afterEach, describe, expect, it } from "vitest";
import { DEV_DASH_KEY, dashUrlFor, devDashEnabled } from "./devDash";

describe("devDash", () => {
  afterEach(() => localStorage.removeItem(DEV_DASH_KEY));

  it("is off unless the dev flag says 1", () => {
    expect(devDashEnabled()).toBe(false);
    localStorage.setItem(DEV_DASH_KEY, "true");
    expect(devDashEnabled()).toBe(false);
    localStorage.setItem(DEV_DASH_KEY, "1");
    expect(devDashEnabled()).toBe(true);
  });

  it("asks for the manifest.mpd of the very master the session would load", () => {
    expect(dashUrlFor("/api/v1/media/m1/hls/master.m3u8?token=t&audioTrack=1&vd=transcode"))
      .toBe("/api/v1/media/m1/hls/manifest.mpd?token=t&audioTrack=1&vd=transcode");
  });
});
