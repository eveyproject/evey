// Driven port: something able to display a fully-computed SkyScene.
// The application pushes new scenes on a timer; the renderer owns smoothing,
// twinkle/shooting-star animation and frame timing.

import type { SkyScene } from "../model/scene";

export interface SceneRenderer {
  mount(host: HTMLElement): void;
  setTarget(scene: SkyScene): void;
  dispose(): void;
}
