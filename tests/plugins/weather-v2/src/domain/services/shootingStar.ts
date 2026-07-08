// Occasional shooting star on clear nights. Deterministic pseudo-random
// trigger keyed off elapsed animation time + a per-session seed - the
// renderer only owns the clock and the "is it dark and clear enough" gate.
//
// shootingStar MUTATES the ShootingStar `out` param in place and returns
// void: the render loop calls it every frame (~30/s), and the class comment
// on GlRenderer promises the loop allocates nothing. `out` is allocated
// exactly once, at mount (initShootingStar()), and reused for the widget's
// whole lifetime. The scheduling MATH itself stays a pure function of
// (timeSec, seed, active) - only the "return a fresh object" part changed -
// so it's still fully testable via a throwaway target object per assertion.

export interface ShootingStar {
  visible: boolean;
  x: number;    // 0..1 widget-space head position
  y: number;    // 0..1, y=0 top
  dirX: number; // unit travel direction (widget space, aspect-uncorrected)
  dirY: number;
  life: number; // 0..1 brightness, fades out over the streak
}

export function initShootingStar(): ShootingStar {
  return { visible: false, x: 0, y: 0, dirX: 0, dirY: 0, life: 0 };
}

// One candidate slot per this many seconds of elapsed animation time; only a
// fraction of slots (APPEAR_CHANCE) actually produce a star, and each star is
// only visible for a short slice (STREAK_SEC) within its slot - the result is
// a rare, occasional streak rather than a periodic tick.
const WINDOW_SEC = 25;
const APPEAR_CHANCE = 0.35;
const STREAK_SEC = 1.1;

// Cheap deterministic hash -> [0, 1), same family as the shader's hash21 but
// kept separate: this one runs on the CPU/TS side for the pure scheduler.
function hash1(n: number): number {
  const s = Math.sin(n * 12.9898) * 43758.5453123;
  return s - Math.floor(s);
}

function hide(out: ShootingStar): void {
  out.visible = false;
  out.x = 0;
  out.y = 0;
  out.dirX = 0;
  out.dirY = 0;
  out.life = 0;
}

export function shootingStar(out: ShootingStar, timeSec: number, seed: number, active: boolean): void {
  if (!active || !Number.isFinite(timeSec) || timeSec < 0) {
    hide(out);
    return;
  }

  const windowIndex = Math.floor(timeSec / WINDOW_SEC);
  const base = windowIndex * 7.13 + seed;
  if (hash1(base) > APPEAR_CHANCE) {
    hide(out);
    return;
  }

  const tInWindow = timeSec - windowIndex * WINDOW_SEC;
  const startAt = hash1(base + 1.31) * (WINDOW_SEC - STREAK_SEC);
  const progress = (tInWindow - startAt) / STREAK_SEC;
  if (progress < 0 || progress >= 1) {
    hide(out);
    return;
  }

  const dirSign = hash1(base + 2.47) < 0.5 ? -1 : 1;
  const descentDeg = 20 + hash1(base + 3.59) * 40; // 20..60 degrees below horizontal
  const rad = (descentDeg * Math.PI) / 180;
  const dirX = dirSign * Math.cos(rad);
  const dirY = Math.sin(rad);

  const startX = hash1(base + 4.71);
  const startY = hash1(base + 5.83) * 0.35; // upper third of the sky
  const travel = 0.5 + hash1(base + 6.97) * 0.3;

  out.visible = true;
  out.x = startX + progress * travel * dirX;
  out.y = startY + progress * travel * dirY;
  out.dirX = dirX;
  out.dirY = dirY;
  out.life = Math.pow(1 - progress, 1.5);
}
