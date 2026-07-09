// Lazy pointer-follow easing for the particle parallax ("réagit
// paresseusement au mouvement du curseur" in the plugin spec). A single
// mutable 2-float state, eased exponentially toward the latest pointer
// target - framerate independent (dt-scaled), zero allocation per step, so
// it is safe to call from the render loop every frame.

export interface PointerState {
  x: number;
  y: number;
}

// Higher = catches up to the pointer faster. Deliberately slow: the point is
// a lazy drift, not a responsive cursor-follow.
const EASE_RATE = 1.6;

export function createPointerState(): PointerState {
  return { x: 0, y: 0 };
}

export function stepPointer(state: PointerState, targetX: number, targetY: number, dt: number): void {
  const k = 1 - Math.exp(-EASE_RATE * Math.max(0, dt));
  state.x += (targetX - state.x) * k;
  state.y += (targetY - state.y) * k;
}
