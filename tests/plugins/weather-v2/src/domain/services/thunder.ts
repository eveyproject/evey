// Lightning scheduler: decides WHEN a flash fires and WHERE it lands.
// State-transition logic is testable without a WebGL context - unlike the
// v2.8.0 build where this lived inline in GlRenderer's render loop next to
// non-deterministic Math.random() calls, untestable by construction.
//
// stepThunder MUTATES the ThunderState in place and returns void: the render
// loop calls it every frame (~30/s), and the class comment on GlRenderer
// promises the loop allocates nothing. `state` is allocated exactly once, at
// mount (initThunderState()), and reused for the widget's whole lifetime.

export interface ThunderState {
  flash: number;   // 0..1 current flash brightness (decays every step)
  countdown: number; // seconds until the next flash is scheduled
  flashX: number;   // 0..1 widget-space position of the flash origin
  flashY: number;   // 0..1, upper cloud band
}

export function initThunderState(): ThunderState {
  return { flash: 0, countdown: 0, flashX: 0.5, flashY: 0.2 };
}

// stepSec: elapsed seconds since the last step.
// thunderRate: average flashes/second from the current SceneParams (0 = calm
// sky, scheduling is skipped and any residual flash just decays away).
// rand: injected source of randomness in [0, 1); production passes
// Math.random, tests pass a deterministic stub.
export function stepThunder(
  state: ThunderState,
  stepSec: number,
  thunderRate: number,
  rand: () => number,
): void {
  if (thunderRate > 0) {
    state.countdown -= stepSec;
    if (state.countdown <= 0) {
      state.flash = 0.7 + rand() * 0.5;
      // Flash origin: upper band where the storm cloud layer actually reads
      // (matches the shader's cloud vertical mask), so the burst looks like
      // it comes from inside the cloud rather than floating in open sky.
      state.flashX = 0.15 + rand() * 0.70;
      state.flashY = 0.05 + rand() * 0.35;
      state.countdown = Math.min(0.3 - Math.log(rand() + 1e-6) / thunderRate, 3 / thunderRate);
    }
  }
  state.flash = Math.max(0, state.flash - stepSec * 5);
}
