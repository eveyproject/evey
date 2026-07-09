// Driven port: something able to display the poster wall.
// The application pushes the wall + drives the roulette; the renderer owns
// per-frame timing, texture loading and pointer picking.

import type { WallLayout } from "../model/wall";
import type { RouletteSpin } from "../services/roulette";

export interface RenderPoster {
  id: string;
  title: string;
  posterUrl: string;
}

export interface WallTarget {
  layout: WallLayout;
  posters: RenderPoster[]; // same length/order as layout.slots
}

// How many posters/rows the current device tier can comfortably afford - the
// application asks for this before building the layout/selecting posters, so
// a weak device gets a smaller, texture-cheaper wall from the start rather
// than building a big one and thrashing the texture LRU.
export interface WallCapacity {
  posterCount: number;
  rows: number;
}

export interface SceneRenderer {
  getCapacity(): WallCapacity;
  mount(host: HTMLElement): void;
  setWall(target: WallTarget): void;
  // Plays the "ce soir, on regarde ?" roulette: sweeps the spotlight through
  // `spin.steps`, then locks/zooms on `spin.winnerIndex` and calls `onLanded`
  // once settled. A no-op when the wall is empty or already spinning.
  runRoulette(spin: RouletteSpin, onLanded: (index: number) => void): void;
  // Fired when a poster is clicked/tapped (index into the last setWall posters).
  onActivate(cb: (index: number) => void): void;
  // Fired on hover/tap-move; null when no poster is under the pointer.
  onHover(cb: (index: number | null) => void): void;
  dispose(): void;
}
