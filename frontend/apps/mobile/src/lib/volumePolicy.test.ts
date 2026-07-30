import { describe, it, expect } from "vitest";
import { resolveVolume } from "./volumePolicy";

// The 0-2 bar has to land somewhere real. Before this, 0-1 only scaled the
// PLAYER's own volume, which multiplies with the device volume: on a tablet
// left at 20%, dragging to 200% was barely louder than 100% - the boost had
// almost nothing to amplify (user report, 2026-07-30). Where a device volume
// control exists, the bar now IS the device volume, and the >100% zone is the
// only part that amplifies.

describe("resolveVolume with a device volume control (Android)", () => {
  const opts = { hasDeviceControl: true };

  it("maps the 0-100% half onto the device volume, player untouched", () => {
    expect(resolveVolume(0.5, opts)).toEqual({ deviceFraction: 0.5, playerVolume: 1, boostGain: 1 });
  });

  it("keeps the player at full so the two attenuations never stack", () => {
    // player.volume * deviceVolume would square the fade: 50% would sound like 25%.
    expect(resolveVolume(0.25, opts).playerVolume).toBe(1);
  });

  it("silences EVEY at zero and leaves the device level alone", () => {
    // Zero means "no sound from this app", not "mute the tablet". Writing 0 to
    // the system stream would silence notifications and every other app, and
    // leave the device muted after leaving the player.
    expect(resolveVolume(0, opts)).toEqual({ deviceFraction: null, playerVolume: 0, boostGain: 1 });
  });

  it("keeps driving the device just above zero", () => {
    // The silence rule is the endpoint only - the rest of the band still moves
    // the system volume, which is the whole point of this policy.
    expect(resolveVolume(0.05, opts).deviceFraction).toBeCloseTo(0.05, 3);
    expect(resolveVolume(0.05, opts).playerVolume).toBe(1);
  });

  it("pins the device to max before amplifying above 100%", () => {
    // Amplifying a signal the device is already attenuating is exactly the
    // complaint this policy answers.
    expect(resolveVolume(1.6, opts)).toEqual({ deviceFraction: 1, playerVolume: 1, boostGain: 1.6 });
  });

  it("clamps to the 0-2 range", () => {
    expect(resolveVolume(3, opts).boostGain).toBe(2);
    // Clamped to zero, so it takes the silence rule: Evey goes quiet, the
    // device keeps its level.
    expect(resolveVolume(-1, opts)).toEqual({ deviceFraction: null, playerVolume: 0, boostGain: 1 });
  });
});

describe("resolveVolume without a device volume control (iOS, or module missing)", () => {
  const opts = { hasDeviceControl: false };

  it("falls back to the historical behaviour: the player carries 0-100%", () => {
    expect(resolveVolume(0.5, opts)).toEqual({ deviceFraction: null, playerVolume: 0.5, boostGain: 1 });
  });

  it("still amplifies above 100% where the boost exists", () => {
    expect(resolveVolume(1.6, opts)).toEqual({ deviceFraction: null, playerVolume: 1, boostGain: 1.6 });
  });

  it("never asks for a device change it cannot make", () => {
    for (const v of [0, 0.3, 1, 2]) {
      expect(resolveVolume(v, opts).deviceFraction).toBeNull();
    }
  });
});
