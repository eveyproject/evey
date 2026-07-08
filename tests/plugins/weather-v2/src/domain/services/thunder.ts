// Lightning scheduler: decides WHEN a flash fires and WHERE it lands.
// Pure state-transition function (the renderer owns the clock and the RNG),
// so the scheduling logic is testable without a WebGL context - unlike the
// v2.8.0 build where this lived inline in GlRenderer's render loop next to
// non-deterministic Math.random() calls, untestable by construction.

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
): ThunderState {
  let { flash, countdown, flashX, flashY } = state;
  if (thunderRate > 0) {
    countdown -= stepSec;
    if (countdown <= 0) {
      flash = 0.7 + rand() * 0.5;
      // Flash origin: upper band where the storm cloud layer actually reads
      // (matches the shader's cloud vertical mask), so the burst looks like
      // it comes from inside the cloud rather than floating in open sky.
      flashX = 0.15 + rand() * 0.70;
      flashY = 0.05 + rand() * 0.35;
      countdown = Math.min(0.3 - Math.log(rand() + 1e-6) / thunderRate, 3 / thunderRate);
    }
  }
  flash = Math.max(0, flash - stepSec * 5);
  return { flash, countdown, flashX, flashY };
}
