/** The playback speeds every player offers, slowest first. */
export const SPEED_OPTIONS: readonly number[] = [0.5, 0.75, 1, 1.25, 1.5, 2];

/** The speed after `current` in the list, wrapping round (a remote's one "speed" button). */
export function nextPlaybackSpeed(current: number): number {
  const i = SPEED_OPTIONS.indexOf(current);
  return SPEED_OPTIONS[(i + 1) % SPEED_OPTIONS.length];
}
