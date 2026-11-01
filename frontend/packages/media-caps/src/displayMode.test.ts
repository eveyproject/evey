import { describe, it, expect } from "vitest";
import { pickDisplayMode, type DisplayMode } from "./displayMode";

// The modes a 1080p television reports through an Android TV box.
const modes: DisplayMode[] = [
  { id: 1, width: 1920, height: 1080, refreshRate: 59.94006 },
  { id: 2, width: 1920, height: 1080, refreshRate: 60.000004 },
  { id: 3, width: 1920, height: 1080, refreshRate: 50 },
  { id: 4, width: 1920, height: 1080, refreshRate: 29.97003 },
  { id: 5, width: 1920, height: 1080, refreshRate: 25 },
  { id: 6, width: 1920, height: 1080, refreshRate: 23.976025 },
  { id: 7, width: 1920, height: 1080, refreshRate: 24.000002 },
  { id: 8, width: 1280, height: 720, refreshRate: 50 },
];

describe("pickDisplayMode", () => {
  it("plays 25 fps at 50 Hz, not 25 Hz, and keeps the picture size", () => {
    expect(pickDisplayMode(modes, 1, 25)?.id).toBe(3);
  });

  it("plays film at its own rate when no higher multiple exists", () => {
    expect(pickDisplayMode(modes, 1, 23.976)?.id).toBe(6);
    expect(pickDisplayMode(modes, 1, 24)?.id).toBe(7);
  });

  it("does not switch when the current mode already holds every frame evenly", () => {
    expect(pickDisplayMode(modes, 1, 29.97)).toBeNull();
    expect(pickDisplayMode(modes, 1, 59.94)).toBeNull();
    expect(pickDisplayMode(modes, 3, 25)).toBeNull();
  });

  it("does nothing without a frame rate or without a fitting mode", () => {
    expect(pickDisplayMode(modes, 1, undefined)).toBeNull();
    expect(pickDisplayMode(modes, 1, 0)).toBeNull();
    expect(pickDisplayMode(modes, 1, 12.5 * 1.1)).toBeNull();
    expect(pickDisplayMode(modes, 99, 25)).toBeNull();
  });
});
