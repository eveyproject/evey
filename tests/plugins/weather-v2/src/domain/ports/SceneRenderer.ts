// Driven port: something able to display a weather scene.
// The application pushes targets; the renderer owns smoothing and timing.

import type { SceneParams, ScenePalette } from "../model/scene";
import type { DayCycle } from "../services/dayCycle";

export interface RenderTarget {
  params: SceneParams;
  cycle: DayCycle;
  palette: ScenePalette;
  moonPhase: number;   // 0..1
  sunModelId: string;  // which daily sun skin to display
}

export interface SceneRenderer {
  mount(host: HTMLElement): void;
  setTarget(target: RenderTarget): void;
  // Upload a live sun image (data: URL) over the bundled snapshot. No-op-safe
  // if the renderer isn't mounted yet.
  setSunImageData(dataUrl: string): void;
  dispose(): void;
}
