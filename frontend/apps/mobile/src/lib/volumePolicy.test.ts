import { describe, it, expect } from "vitest";
import { playerTrim, resolveVolume } from "./volumePolicy";

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

// The 2026-08-31 report: adjusting the volume sounded LUMPY. STREAM_MUSIC has
// ~15 hardware steps of several dB each and a drag was riding them raw. With
// the step count known, the device holds the step at or above the level and
// the player element trims the remainder - the audible product tracks the
// finger continuously and the bridge moves at most once per hardware step.
describe("resolveVolume with the hardware step count (smooth split)", () => {
  const opts = { hasDeviceControl: true, deviceSteps: 15 };

  it("keeps the audible product EXACTLY at the requested level across the whole band", () => {
    for (let i = 1; i <= 99; i++) {
      const v = i / 100;
      const t = resolveVolume(v, opts);
      expect(t.deviceFraction).not.toBeNull();
      expect(t.deviceFraction! * t.playerVolume).toBeCloseTo(v, 9);
      expect(t.playerVolume).toBeLessThanOrEqual(1);
      expect(t.boostGain).toBe(1);
    }
  });

  it("only ever hands the device a real hardware step", () => {
    for (let i = 1; i <= 99; i++) {
      const t = resolveVolume(i / 100, opts);
      expect((t.deviceFraction! * 15) % 1).toBeCloseTo(0, 9);
    }
  });

  it("moves the device at most once per hardware step over a full sweep", () => {
    const written = new Set<number>();
    for (let i = 1; i <= 999; i++) {
      written.add(resolveVolume(i / 1000, opts).deviceFraction!);
    }
    expect(written.size).toBeLessThanOrEqual(15);
  });

  it("keeps the endpoints special: zero never touches the device, 100%+ pins it to max", () => {
    expect(resolveVolume(0, opts)).toEqual({ deviceFraction: null, playerVolume: 0, boostGain: 1 });
    expect(resolveVolume(1, opts)).toEqual({ deviceFraction: 1, playerVolume: 1, boostGain: 1 });
    expect(resolveVolume(1.5, opts)).toEqual({ deviceFraction: 1, playerVolume: 1, boostGain: 1.5 });
  });

  it("fades continuously to silence below the first step (the player carries it)", () => {
    const t = resolveVolume(0.01, opts);
    expect(t.deviceFraction).toBeCloseTo(1 / 15, 9);
    expect(t.playerVolume).toBeCloseTo(0.15, 9);
  });

  it("falls back to the raw fraction when the step count is unknown", () => {
    expect(resolveVolume(0.5, { hasDeviceControl: true })).toEqual({ deviceFraction: 0.5, playerVolume: 1, boostGain: 1 });
  });
});


// What reaches the ear is device x player. The trim is taken against the level
// the device is ON, so the product is exactly the bar's level once the device
// has landed, and at most one step under it while it is on its way.
describe("playerTrim", () => {
  const opts = { hasDeviceControl: true, deviceSteps: 15 };

  it("makes device x player equal the level once the device has landed", () => {
    const device = resolveVolume(0.5, opts).deviceFraction!;
    expect(device * playerTrim(0.5, device, opts)).toBeCloseTo(0.5, 6);
  });

  it("never overshoots while the device is still on a lower step", () => {
    expect(playerTrim(0.5, 7 / 15, opts)).toBe(1);
    expect(playerTrim(0.4, 8 / 15, opts)).toBeCloseTo(0.75, 6);
  });

  it("leaves 0, the boosted zone and a platform without device control to the policy", () => {
    expect(playerTrim(0, 0.6, opts)).toBe(0);
    expect(playerTrim(1.5, 1, opts)).toBe(1);
    expect(playerTrim(0.5, null, opts)).toBe(resolveVolume(0.5, opts).playerVolume);
    expect(playerTrim(0.5, 0.6, { hasDeviceControl: false })).toBe(0.5);
  });
});
