// Which display mode a video should play in, so each frame stays on screen
// the same number of refreshes. A 25 fps episode on a 59.94 Hz output is held
// two refreshes, then three, unevenly: motion stutters. The same episode at
// 50 Hz is held two refreshes, every time.

/** One mode of the attached display (Display.Mode on Android). */
export interface DisplayMode {
  id: number;
  width: number;
  height: number;
  refreshRate: number;
}

// How far a refresh rate may sit from an exact multiple of the frame rate,
// relative. Tight enough to tell 23.976 from 24 (0.1% apart: a 24 Hz output
// repeats a 23.976 fps frame every 42 s), loose enough for the rounding of a
// probed rate (24000/1001 read as 23.976).
const MULTIPLE_TOLERANCE = 0.0005;

/**
 * The mode to switch to for a video of fps frames per second: same picture
 * size as the current mode, a refresh rate that is a whole multiple of fps,
 * the highest such (50 Hz over 25 Hz for 25 fps: a 25 Hz panel flickers).
 * Null when the frame rate is unknown, when no mode fits, or when the current
 * mode already fits: no switch, no black screen.
 */
export function pickDisplayMode(modes: readonly DisplayMode[], currentId: number, fps: number | null | undefined): DisplayMode | null {
  if (!fps || !Number.isFinite(fps) || fps <= 0) return null;
  const current = modes.find((m) => m.id === currentId);
  if (!current) return null;
  const fits = (m: DisplayMode) => {
    const ratio = m.refreshRate / fps;
    const whole = Math.round(ratio);
    return whole >= 1 && Math.abs(ratio - whole) <= MULTIPLE_TOLERANCE * whole;
  };
  if (fits(current)) return null;
  const candidates = modes.filter((m) => m.width === current.width && m.height === current.height && fits(m));
  if (candidates.length === 0) return null;
  return candidates.reduce((best, m) => (m.refreshRate > best.refreshRate ? m : best));
}
