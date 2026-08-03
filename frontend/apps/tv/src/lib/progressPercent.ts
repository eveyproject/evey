// The share of a title already watched, as a number safe to put in a width.
//
// percentComplete comes from the server, and a position reported past the
// declared duration (a track shorter than its header claims is a known shape in
// this library) yields more than 100 - which paints a progress bar wider than
// its own container and reads "104%" next to it.
export function progressPercent(value: number | null | undefined): number {
  if (value == null || Number.isNaN(value)) return 0;
  if (value < 0) return 0;
  if (value > 100) return 100;
  return value;
}
