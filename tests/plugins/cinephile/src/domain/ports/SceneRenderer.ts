// Driven port: something able to display the taste galaxy.
// The application pushes the scene once per (re)load; the renderer owns
// per-frame timing, entrance easing and pointer picking.

import type { GalaxyNode } from "../services/galaxyLayout";

export interface GalaxyTarget {
  nodes: GalaxyNode[];
}

// How many nodes the current device tier can comfortably afford (glow cost
// scales with node count) - the application asks for this before building
// the layout, so a weak device gets a smaller, cheaper galaxy from the start.
export interface RendererCapacity {
  maxNodes: number;
}

export interface SceneRenderer {
  getCapacity(): RendererCapacity;
  mount(host: HTMLElement): void;
  setScene(target: GalaxyTarget): void;
  // Fired when a node is clicked/tapped (index into the last setScene nodes).
  onActivate(cb: (index: number) => void): void;
  // Fired on hover/tap-move; null when no node is under the pointer.
  onHover(cb: (index: number | null) => void): void;
  dispose(): void;
}
