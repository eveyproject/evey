// Pure gesture-intent tracker behind the auto-hiding nav rail: accumulates
// vertical finger travel and emits "hide" when the user dives into content
// (finger up = scrolling down) or "show" when they head back up. Working on
// touch deltas instead of scroll offsets keeps it screen-agnostic (no wiring
// into every list) and functional even on pages too short to scroll.
export type RailIntent = "show" | "hide";

export function createRailIntentTracker(threshold = 24) {
  let lastY = 0;
  let lastX = 0;
  let acc = 0;

  return {
    start(y: number, x = 0) {
      lastY = y;
      lastX = x;
      acc = 0;
    },
    move(y: number, x = 0): RailIntent | null {
      const dy = y - lastY;
      const dx = x - lastX;
      lastY = y;
      lastX = x;
      // Mostly-horizontal moves (carousels, swipes) must not toggle the rail.
      if (Math.abs(dx) > Math.abs(dy)) return null;
      acc += dy;
      if (acc <= -threshold) {
        acc = 0;
        return "hide";
      }
      if (acc >= threshold) {
        acc = 0;
        return "show";
      }
      return null;
    },
  };
}
