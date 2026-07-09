// Driven port: something able to display the lumina scene. The application
// pushes the (rare) settings target; the renderer owns every continuous
// animation - palette drift, veil motion, caustics, particle parallax - and
// its own frame timing.

import type { RenderTarget } from "../model/scene";

export interface SceneRenderer {
  mount(host: HTMLElement): void;
  setTarget(target: RenderTarget): void;
  // Normalized pointer offset from the widget center (-1..1, y down like
  // CSS), forwarded on every pointermove. The renderer eases it internally
  // (see domain/services/pointerEase.ts), so this is safe to call every
  // frame without any smoothing on the caller's side.
  setPointer(nx: number, ny: number): void;
  dispose(): void;
}
