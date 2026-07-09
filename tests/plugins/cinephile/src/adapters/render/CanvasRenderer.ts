// Canvas2D implementation of the SceneRenderer port.
//
// This plugin is data-viz, not an ambient scene: a handful of glowing nodes
// and simple gradients read better (and cost far less) through Canvas2D's
// built-in radial gradients/shadowBlur than through a hand-rolled WebGL
// program, so unlike marquee/cosmos this renderer is raw Canvas2D + vanilla
// DOM (still zero runtime dependencies).
//
// Memory/CPU discipline mirrors the WebGL renderers in this repo: node state
// is allocated once per setScene() call (a data reload, not a frame) and
// mutated in place every tick by the render loop - draw() never pushes into
// a JS array or allocates a typed buffer. Paused when hidden/out of view.
// Everything is torn down in dispose(): observers, listeners, the canvas.

import type { SceneRenderer, GalaxyTarget, RendererCapacity } from "../../domain/ports/SceneRenderer";
import type { GalaxyNode } from "../../domain/services/galaxyLayout";
import { hitTestGalaxy } from "../../domain/services/hitTest";
import { maxNodesForTier, glowEnabledForTier, type QualityTier } from "../../domain/services/qualityTier";

let cachedTier: QualityTier | null = null;
function detectQualityTier(): QualityTier {
  if (cachedTier) return cachedTier;
  try {
    const cores = typeof navigator !== "undefined" ? navigator.hardwareConcurrency ?? 2 : 2;
    const dpr = typeof window !== "undefined" ? window.devicePixelRatio || 1 : 1;
    if (cores >= 4 && dpr <= 2) return (cachedTier = "high");
    if (cores >= 2) return (cachedTier = "mid");
    return (cachedTier = "low");
  } catch {
    return (cachedTier = "low");
  }
}

interface NodeState {
  x: number;
  y: number;
  radius: number;
  targetX: number;
  targetY: number;
  targetRadius: number;
  opacity: number;
  phase: number;
  hovered: boolean;
}

// Fraction of min(canvas width, height) used as the orbit/radius scale,
// leaving margin for the tooltip/profile card and canvas edges.
const SPACE_SCALE = 0.42;
const ENTER_EASE_PER_60FPS = 0.1;
const HOVER_SCALE = 1.08;

export class CanvasRenderer implements SceneRenderer {
  private host: HTMLElement | null = null;
  private canvas: HTMLCanvasElement | null = null;
  private ctx: CanvasRenderingContext2D | null = null;

  private tier = detectQualityTier();
  private glow = glowEnabledForTier(this.tier);
  private reducedMotion =
    typeof window !== "undefined" && window.matchMedia?.("(prefers-reduced-motion: reduce)").matches === true;

  private nodes: GalaxyNode[] = [];
  private state: NodeState[] = [];

  private raf = 0;
  private running = false;
  private inView = true;
  private lastTs = 0;
  private time = 0;

  private resizeObs: ResizeObserver | null = null;
  private viewObs: IntersectionObserver | null = null;

  private activateCb: ((index: number) => void) | null = null;
  private hoverCb: ((index: number | null) => void) | null = null;
  private hoveredIndex: number | null = null;

  private onPointerMove = (e: PointerEvent) => this.handlePointer(e.clientX, e.clientY, false);
  private onPointerLeave = () => this.setHovered(null);
  private onClick = (e: MouseEvent) => this.handlePointer(e.clientX, e.clientY, true);

  getCapacity(): RendererCapacity {
    return { maxNodes: maxNodesForTier(this.tier) };
  }

  mount(host: HTMLElement): void {
    this.host = host;
    const canvas = document.createElement("canvas");
    canvas.style.cssText = "position:absolute;inset:0;width:100%;height:100%;display:block;cursor:pointer;";
    host.appendChild(canvas);
    this.canvas = canvas;
    this.ctx = canvas.getContext("2d") as CanvasRenderingContext2D | null;

    canvas.addEventListener("pointermove", this.onPointerMove);
    canvas.addEventListener("pointerleave", this.onPointerLeave);
    canvas.addEventListener("click", this.onClick);

    this.resizeObs = new ResizeObserver(() => this.resize());
    this.resizeObs.observe(host);
    this.viewObs = new IntersectionObserver((entries) => {
      this.inView = entries[0]?.isIntersecting ?? true;
    });
    this.viewObs.observe(host);

    this.resize();
    this.running = true;
    this.raf = requestAnimationFrame(this.loop);
  }

  setScene(target: GalaxyTarget): void {
    this.nodes = target.nodes;
    const prev = this.state;
    const next: NodeState[] = new Array(target.nodes.length);
    for (let i = 0; i < target.nodes.length; i++) {
      const node = target.nodes[i];
      const existing = prev[i];
      next[i] = {
        x: existing ? existing.x : 0,
        y: existing ? existing.y : 0,
        radius: existing ? existing.radius : 0,
        targetX: node.x,
        targetY: node.y,
        targetRadius: node.radius,
        opacity: existing ? existing.opacity : 0,
        phase: (i * 0.61803398875) % 1,
        hovered: false,
      };
    }
    this.state = next;
    this.hoveredIndex = null;
  }

  onActivate(cb: (index: number) => void): void {
    this.activateCb = cb;
  }

  onHover(cb: (index: number | null) => void): void {
    this.hoverCb = cb;
  }

  dispose(): void {
    this.running = false;
    cancelAnimationFrame(this.raf);
    this.resizeObs?.disconnect();
    this.viewObs?.disconnect();
    this.resizeObs = null;
    this.viewObs = null;
    const canvas = this.canvas;
    if (canvas) {
      canvas.removeEventListener("pointermove", this.onPointerMove);
      canvas.removeEventListener("pointerleave", this.onPointerLeave);
      canvas.removeEventListener("click", this.onClick);
      canvas.remove();
    }
    this.canvas = null;
    this.ctx = null;
    this.host = null;
    this.nodes = [];
    this.state = [];
    this.activateCb = null;
    this.hoverCb = null;
    this.hoveredIndex = null;
  }

  // ---- internals ----------------------------------------------------------

  private resize(): void {
    const { host, canvas } = this;
    if (!host || !canvas) return;
    const dpr = Math.min(typeof window !== "undefined" ? window.devicePixelRatio || 1 : 1, 2);
    const w = Math.max(1, Math.round(host.clientWidth * dpr));
    const h = Math.max(1, Math.round(host.clientHeight * dpr));
    if (canvas.width !== w || canvas.height !== h) {
      canvas.width = w;
      canvas.height = h;
    }
  }

  private handlePointer(clientX: number, clientY: number, isClick: boolean): void {
    const canvas = this.canvas;
    if (!canvas || this.state.length === 0) {
      if (isClick) return;
      this.setHovered(null);
      return;
    }
    const rect = canvas.getBoundingClientRect();
    const w = rect.width || 1;
    const h = rect.height || 1;
    const scale = Math.min(w, h) * SPACE_SCALE;
    if (scale <= 0) return;
    const nx = (clientX - rect.left - w / 2) / scale;
    const ny = (clientY - rect.top - h / 2) / scale;
    const index = hitTestGalaxy(this.state, { x: nx, y: ny });
    if (isClick) {
      if (index !== null) this.activateCb?.(index);
      return;
    }
    this.setHovered(index);
  }

  private setHovered(index: number | null): void {
    if (index === this.hoveredIndex) return;
    if (this.hoveredIndex !== null && this.state[this.hoveredIndex]) this.state[this.hoveredIndex].hovered = false;
    this.hoveredIndex = index;
    if (index !== null && this.state[index]) this.state[index].hovered = true;
    this.hoverCb?.(index);
  }

  private loop = (ts: number): void => {
    if (!this.running) return;
    this.raf = requestAnimationFrame(this.loop);

    if (document.hidden || !this.inView || !this.ctx) {
      this.lastTs = ts;
      return;
    }

    const dt = Math.min((ts - this.lastTs) / 1000, 0.25);
    this.lastTs = ts;
    this.time += this.reducedMotion ? dt * 0.25 : dt;
    this.advance(dt);
    this.draw();
  };

  private advance(dt: number): void {
    const ease = Math.min(1, ENTER_EASE_PER_60FPS * (dt * 60));
    for (let i = 0; i < this.state.length; i++) {
      const s = this.state[i];
      s.x += (s.targetX - s.x) * ease;
      s.y += (s.targetY - s.y) * ease;
      s.radius += (s.targetRadius - s.radius) * ease;
      s.opacity += (1 - s.opacity) * ease;
    }
  }

  private draw(): void {
    const ctx = this.ctx;
    const canvas = this.canvas;
    if (!ctx || !canvas) return;
    const w = canvas.width;
    const h = canvas.height;
    ctx.clearRect(0, 0, w, h);

    const scale = Math.min(w, h) * SPACE_SCALE;
    const cx = w / 2;
    const cy = h / 2;

    for (let i = 0; i < this.state.length; i++) {
      const s = this.state[i];
      const node = this.nodes[i];
      if (!node || s.opacity <= 0.001) continue;

      const px = cx + s.x * scale;
      const py = cy + s.y * scale;
      const baseR = Math.max(1, s.radius * scale);
      const pulse = this.reducedMotion ? 0 : Math.sin(this.time * 1.4 + s.phase * Math.PI * 2) * 0.06;
      const drawR = baseR * (1 + pulse) * (s.hovered ? HOVER_SCALE : 1);
      const hue = hueForDecade(node.decade);

      ctx.globalAlpha = s.opacity;
      if (this.glow) {
        ctx.shadowBlur = drawR * 0.9;
        ctx.shadowColor = `hsla(${hue}, 85%, 70%, 0.55)`;
      } else {
        ctx.shadowBlur = 0;
      }
      const gradient = ctx.createRadialGradient(px, py, 0, px, py, drawR);
      gradient.addColorStop(0, `hsla(${hue}, 90%, 78%, 0.95)`);
      gradient.addColorStop(1, `hsla(${hue}, 78%, 42%, 0.88)`);
      ctx.fillStyle = gradient;
      ctx.beginPath();
      ctx.arc(px, py, drawR, 0, Math.PI * 2);
      ctx.fill();
    }
    ctx.globalAlpha = 1;
    ctx.shadowBlur = 0;
  }
}

// Deterministic, purely cosmetic hue per decade: sweeps from deep violet
// (older decades) towards warm gold (recent ones); the merged "other" node
// gets a distinct steel-blue so it never reads as "just another decade".
function hueForDecade(decade: number | null): number {
  if (decade === null) return 205;
  const index = Math.round((decade - 1900) / 10);
  return (((270 - index * 10) % 360) + 360) % 360;
}
