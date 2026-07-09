// Raw-WebGL implementation of the SceneRenderer port: a perspective wall of
// textured poster quads, a warm cinema-room backdrop (vignette + grain +
// sweeping/locked spotlight beam) and an optional soft floor reflection.
//
// Memory/CPU discipline (same philosophy as weather-v2/cosmos):
//   - one WebGL context, two tiny programs (background full-screen triangle,
//     poster quad), no framebuffers, no scene graph.
//   - the wall layout is built once per setWall() call, not per frame; the
//     render loop itself allocates nothing (scalar math + gl.* calls over a
//     fixed-size array of slots).
//   - poster textures are capacity-bounded and progressively streamed by
//     TextureManager (see that file), never all fetched at once.
//   - ~30/24 fps cap by tier, skips entirely when hidden/scrolled out of view.
//   - everything is torn down in dispose(): observers, listeners, GL
//     objects/textures, then the context itself via WEBGL_lose_context.

import type { SceneRenderer, WallTarget, WallCapacity } from "../../domain/ports/SceneRenderer";
import type { WallSlot } from "../../domain/model/wall";
import type { RouletteSpin } from "../../domain/services/roulette";
import { advanceScroll } from "../../domain/services/scrollState";
import { spotlightPhase, spotlightX } from "../../domain/services/spotlightSweep";
import { hitTestWall } from "../../domain/services/hitTest";
import { placeholderColorForId } from "../../domain/services/placeholderPalette";
import { WALL_GEOMETRY } from "../../domain/services/wallGeometry";
import { BG_VERT_SRC, BG_FRAG_SRC, POSTER_VERT_SRC, POSTER_FRAG_SRC } from "./shaders";
import { TextureManager } from "./TextureManager";

// ---- wall geometry constants (kept in sync with the layout the application
// builds via WALL_GEOMETRY - see that module's docstring) ------------------

const BASE_HALF_WIDTH = WALL_GEOMETRY.colSpacing * 0.47;
const BASE_HALF_HEIGHT = BASE_HALF_WIDTH / WALL_GEOMETRY.posterAspect;
const BASELINE_Y = -0.25;
const HOVER_LIFT = 0.09;
const HOVER_SCALE_BOOST = 1.16;
// The roulette winner pops harder than a hover and stays put (its own lift,
// not gated on the pointer) so it reads as "this is your pick".
const WINNER_LIFT = 0.15;
const WINNER_SCALE_BOOST = 1.3;

const SCROLL_SPEED_UNITS_PER_SEC = 0.032; // very slow, ambient drift
const SWEEP_PERIOD_MS = 14_000;
const SPOT_WIDTH_CLIP = 0.22;

interface Quality {
  scale: number;
  fps: number;
  maxDpr: number;
  posterCount: number;
  rows: number;
  reflections: boolean;
  textureCapacity: number;
}

let cachedQuality: Quality | null = null;
function detectQuality(): Quality {
  if (cachedQuality) return cachedQuality;
  try {
    const c = document.createElement("canvas");
    const gl = (c.getContext("webgl2") || c.getContext("webgl")) as WebGLRenderingContext | null;
    if (!gl) {
      return (cachedQuality = { scale: 0.5, fps: 24, maxDpr: 1, posterCount: 14, rows: 2, reflections: false, textureCapacity: 16 });
    }
    const maxTex = gl.getParameter(gl.MAX_TEXTURE_SIZE) as number;
    const cores = navigator.hardwareConcurrency ?? 2;
    const isWebGL2 = "drawBuffers" in gl;
    if (isWebGL2 && maxTex >= 8192 && cores >= 4) {
      return (cachedQuality = { scale: 1.0, fps: 30, maxDpr: 1.5, posterCount: 30, rows: 3, reflections: true, textureCapacity: 32 });
    }
    if (maxTex >= 4096) {
      return (cachedQuality = { scale: 0.75, fps: 30, maxDpr: 1.25, posterCount: 24, rows: 3, reflections: true, textureCapacity: 28 });
    }
    return (cachedQuality = { scale: 0.5, fps: 24, maxDpr: 1, posterCount: 14, rows: 2, reflections: false, textureCapacity: 16 });
  } catch {
    return (cachedQuality = { scale: 0.5, fps: 24, maxDpr: 1, posterCount: 14, rows: 2, reflections: false, textureCapacity: 16 });
  }
}

function wrapCentered(x: number, period: number): number {
  if (!(period > 0)) return x;
  let v = ((x % period) + period) % period;
  if (v > period / 2) v -= period;
  return v;
}

function clamp(v: number, lo: number, hi: number): number {
  return v < lo ? lo : v > hi ? hi : v;
}

interface PosterEntry {
  id: string;
  posterUrl: string;
  placeholder: readonly [number, number, number];
  tex: WebGLTexture | null;
}

export class GlRenderer implements SceneRenderer {
  private host: HTMLElement | null = null;
  private canvas: HTMLCanvasElement | null = null;
  private gl: WebGLRenderingContext | null = null;

  private bgProgram: WebGLProgram | null = null;
  private bgVbo: WebGLBuffer | null = null;
  private bgLoc: Record<string, WebGLUniformLocation | null> = {};

  private posterProgram: WebGLProgram | null = null;
  private quadVbo: WebGLBuffer | null = null;
  private posterLoc: Record<string, WebGLUniformLocation | null> = {};
  private posterAttr = -1;

  private textures: TextureManager | null = null;
  private quality = detectQuality();
  private reducedMotion =
    typeof window !== "undefined" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  private slots: WallSlot[] = [];
  private posters: PosterEntry[] = [];
  private period = 1;

  private scrollOffset = 0;
  private hoverIndex: number | null = null;
  private hoverLift = 0; // smoothed 0..1
  private lockLift = 0;  // smoothed 0..1 emphasis for the elected winner

  private spin: RouletteSpin | null = null;
  private spinStartMs = 0;
  private lockedIndex: number | null = null;
  private onLandedCb: ((index: number) => void) | null = null;

  private raf = 0;
  private running = false;
  private inView = true;
  private lastTs = 0;
  private frameAcc = 0;
  private elapsedMs = 0;
  private lost = false;

  private activateCb: ((index: number) => void) | null = null;
  private hoverCb: ((index: number | null) => void) | null = null;

  // Reused every frame for the seamless-loop tile offsets (-period, 0,
  // +period) instead of allocating a fresh array per poster per frame.
  private readonly tileOffsets: [number, number, number] = [0, 0, 0];

  // Spotlight target for the current frame (clip-space x, intensity, lock
  // amount) - written in place by updateSpotlight() rather than returned as a
  // fresh object, to keep the render loop allocation-free.
  private spotX = 0;
  private spotIntensity = 0;
  private spotLock = 0;

  private resizeObs: ResizeObserver | null = null;
  private viewObs: IntersectionObserver | null = null;
  private onLost = (e: Event) => { e.preventDefault(); this.lost = true; };
  private onRestored = () => { this.lost = false; this.initGL(); };
  private onPointerMove = (e: PointerEvent) => this.handlePointer(e, false);
  private onPointerUp = (e: PointerEvent) => this.handlePointer(e, true);
  private onPointerLeave = () => this.setHover(null);

  getCapacity(): WallCapacity {
    return { posterCount: this.quality.posterCount, rows: this.quality.rows };
  }

  mount(host: HTMLElement): void {
    this.host = host;
    const canvas = document.createElement("canvas");
    canvas.style.cssText = "position:absolute;inset:0;width:100%;height:100%;display:block;cursor:pointer;";
    host.appendChild(canvas);
    this.canvas = canvas;

    canvas.addEventListener("webglcontextlost", this.onLost, false);
    canvas.addEventListener("webglcontextrestored", this.onRestored, false);
    canvas.addEventListener("pointermove", this.onPointerMove);
    canvas.addEventListener("pointerup", this.onPointerUp);
    canvas.addEventListener("pointerleave", this.onPointerLeave);

    this.initGL();

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

  setWall(target: WallTarget): void {
    this.slots = target.layout.slots;
    this.period = target.layout.period;
    // Textures are reserved once here (scene-change boundary), not per
    // frame: reserve() touches the LRU and may evict, which is bookkeeping
    // we don't want to redo 30 times every animation frame once the wall is
    // stable. draw() only ever reads poster.tex + isLoaded() afterwards.
    this.posters = target.posters.map((p) => {
      const placeholder = placeholderColorForId(p.id);
      const tex = this.textures ? this.textures.reserve(p.id, p.posterUrl, placeholder) : null;
      return { id: p.id, posterUrl: p.posterUrl, placeholder, tex };
    });
    this.scrollOffset = 0;
    this.hoverIndex = null;
    this.lockedIndex = null;
    this.spin = null;
  }

  runRoulette(spin: RouletteSpin, onLanded: (index: number) => void): void {
    if (this.slots.length === 0 || spin.steps.length === 0 || this.spin) return;
    this.spin = spin;
    this.spinStartMs = this.elapsedMs;
    this.lockedIndex = null;
    this.onLandedCb = onLanded;
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
    const { gl, canvas } = this;
    if (canvas) {
      canvas.removeEventListener("webglcontextlost", this.onLost);
      canvas.removeEventListener("webglcontextrestored", this.onRestored);
      canvas.removeEventListener("pointermove", this.onPointerMove);
      canvas.removeEventListener("pointerup", this.onPointerUp);
      canvas.removeEventListener("pointerleave", this.onPointerLeave);
    }
    this.textures?.dispose();
    this.textures = null;
    if (gl) {
      if (this.bgProgram) gl.deleteProgram(this.bgProgram);
      if (this.posterProgram) gl.deleteProgram(this.posterProgram);
      if (this.bgVbo) gl.deleteBuffer(this.bgVbo);
      if (this.quadVbo) gl.deleteBuffer(this.quadVbo);
      (gl.getExtension("WEBGL_lose_context") as { loseContext(): void } | null)?.loseContext();
    }
    canvas?.remove();
    this.gl = null;
    this.bgProgram = null;
    this.posterProgram = null;
    this.bgVbo = null;
    this.quadVbo = null;
    this.canvas = null;
    this.host = null;
  }

  // ---- internals ------------------------------------------------------

  private compile(type: number, src: string): WebGLShader | null {
    const gl = this.gl;
    if (!gl) return null;
    const sh = gl.createShader(type);
    if (!sh) return null;
    gl.shaderSource(sh, src);
    gl.compileShader(sh);
    if (!gl.getShaderParameter(sh, gl.COMPILE_STATUS)) {
      console.error("marquee shader:", gl.getShaderInfoLog(sh));
      gl.deleteShader(sh);
      return null;
    }
    return sh;
  }

  private link(vsSrc: string, fsSrc: string): WebGLProgram | null {
    const gl = this.gl;
    if (!gl) return null;
    const vs = this.compile(gl.VERTEX_SHADER, vsSrc);
    const fs = this.compile(gl.FRAGMENT_SHADER, fsSrc);
    if (!vs || !fs) return null;
    const program = gl.createProgram();
    if (!program) return null;
    gl.attachShader(program, vs);
    gl.attachShader(program, fs);
    gl.linkProgram(program);
    gl.deleteShader(vs);
    gl.deleteShader(fs);
    if (!gl.getProgramParameter(program, gl.LINK_STATUS)) {
      console.error("marquee link:", gl.getProgramInfoLog(program));
      gl.deleteProgram(program);
      return null;
    }
    return program;
  }

  private initGL(): void {
    const canvas = this.canvas;
    if (!canvas) return;
    const gl = canvas.getContext("webgl", {
      alpha: true,
      premultipliedAlpha: false,
      antialias: false,
      depth: false,
      stencil: false,
      preserveDrawingBuffer: false,
      powerPreference: "low-power",
    }) as WebGLRenderingContext | null;
    if (!gl) return;
    this.gl = gl;

    this.bgProgram = this.link(BG_VERT_SRC, BG_FRAG_SRC);
    this.posterProgram = this.link(POSTER_VERT_SRC, POSTER_FRAG_SRC);
    if (!this.bgProgram || !this.posterProgram) return;

    this.bgVbo = gl.createBuffer();
    gl.bindBuffer(gl.ARRAY_BUFFER, this.bgVbo);
    gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1, -1, 3, -1, -1, 3]), gl.STATIC_DRAW);
    for (const name of ["uRes", "uTime", "uSpotX", "uSpotIntensity", "uSpotWidth", "uLock"]) {
      this.bgLoc[name] = gl.getUniformLocation(this.bgProgram, name);
    }

    this.quadVbo = gl.createBuffer();
    gl.bindBuffer(gl.ARRAY_BUFFER, this.quadVbo);
    gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-0.5, -0.5, 0.5, -0.5, -0.5, 0.5, 0.5, 0.5]), gl.STATIC_DRAW);
    this.posterAttr = gl.getAttribLocation(this.posterProgram, "aCorner");
    for (const name of ["uCenter", "uSize", "uAspect", "uTex", "uPlaceholder", "uLoaded", "uDepthFade", "uHighlight", "uSpotLight", "uAlpha"]) {
      this.posterLoc[name] = gl.getUniformLocation(this.posterProgram, name);
    }

    this.textures = new TextureManager(gl, this.quality.textureCapacity);

    gl.disable(gl.DEPTH_TEST);
    gl.clearColor(0.02, 0.015, 0.02, 1);
  }

  private resize(): void {
    const { host, canvas, gl } = this;
    if (!host || !canvas || !gl) return;
    const dpr = Math.min(window.devicePixelRatio || 1, this.quality.maxDpr) * this.quality.scale;
    const w = Math.max(1, Math.round(host.clientWidth * dpr));
    const h = Math.max(1, Math.round(host.clientHeight * dpr));
    if (canvas.width !== w || canvas.height !== h) {
      canvas.width = w;
      canvas.height = h;
      gl.viewport(0, 0, w, h);
    }
  }

  private aspect(): number {
    const c = this.canvas;
    return c && c.height > 0 ? c.width / c.height : 1;
  }

  private setHover(index: number | null): void {
    if (this.hoverIndex === index) return;
    this.hoverIndex = index;
    this.hoverCb?.(index);
  }

  private handlePointer(e: PointerEvent, isActivate: boolean): void {
    const canvas = this.canvas;
    if (!canvas || this.slots.length === 0) return;
    const rect = canvas.getBoundingClientRect();
    if (rect.width <= 0 || rect.height <= 0) return;
    const ndcX = ((e.clientX - rect.left) / rect.width) * 2 - 1;
    const ndcY = -(((e.clientY - rect.top) / rect.height) * 2 - 1);
    const aspect = this.aspect();
    const wallX = ndcX * aspect;
    const wallY = ndcY - BASELINE_Y;

    const index = hitTestWall(this.slots, this.scrollOffset, this.period, { x: wallX, y: wallY }, {
      halfWidth: BASE_HALF_WIDTH,
      halfHeight: BASE_HALF_HEIGHT,
    });

    if (isActivate) {
      if (index !== null && !this.spin) this.activateCb?.(index);
    } else {
      this.setHover(index);
    }
  }

  private loop = (ts: number): void => {
    if (!this.running) return;
    this.raf = requestAnimationFrame(this.loop);

    if (document.hidden || !this.inView || this.lost || !this.gl || !this.bgProgram || !this.posterProgram) {
      this.lastTs = ts;
      return;
    }

    const dt = Math.min((ts - this.lastTs) / 1000, 0.25);
    this.lastTs = ts;
    this.frameAcc += dt;
    const interval = 1 / this.quality.fps;
    if (this.frameAcc < interval) return;
    const step = Math.min(this.frameAcc, interval * 3);
    this.frameAcc = 0;

    const motion = this.reducedMotion ? 0.25 : 1;
    this.elapsedMs += step * motion * 1000;

    const frozen = this.spin !== null || this.lockedIndex !== null;
    if (!frozen) {
      this.scrollOffset = advanceScroll(this.scrollOffset, step * motion, SCROLL_SPEED_UNITS_PER_SEC, this.period);
    }

    this.hoverLift += ((this.hoverIndex !== null ? 1 : 0) - this.hoverLift) * (1 - Math.exp(-8 * step));
    this.lockLift += ((this.lockedIndex !== null ? 1 : 0) - this.lockLift) * (1 - Math.exp(-6 * step));

    this.textures?.pump();
    this.draw();
  };

  // Resolves the spotlight's target clip-space x and intensity for this
  // frame - sweeping ambiently, chasing the roulette schedule, or locked on
  // the winner - into this.spotX/spotIntensity/spotLock (no per-frame
  // allocation), and finalizes the spin once its schedule has fully played.
  private updateSpotlight(): void {
    const aspect = this.aspect();

    if (this.spin) {
      const spin = this.spin;
      const elapsed = this.elapsedMs - this.spinStartMs;
      if (elapsed >= spin.totalDurationMs) {
        this.lockedIndex = spin.winnerIndex;
        this.spin = null;
        const cb = this.onLandedCb;
        this.onLandedCb = null;
        cb?.(this.lockedIndex);
      } else {
        let stepIdx = 0;
        for (let i = 0; i < spin.steps.length; i++) {
          if (spin.steps[i].atMs <= elapsed) stepIdx = i;
          else break;
        }
        const slot = this.slots[spin.steps[stepIdx].index];
        this.spotX = slot ? clamp(wrapCentered(slot.x - this.scrollOffset, this.period) / aspect, -1, 1) : 0;
        this.spotIntensity = 1;
        this.spotLock = 0.4;
        return;
      }
    }

    if (this.lockedIndex !== null) {
      const slot = this.slots[this.lockedIndex];
      this.spotX = slot ? clamp(wrapCentered(slot.x - this.scrollOffset, this.period) / aspect, -1, 1) : 0;
      this.spotIntensity = 1;
      this.spotLock = 1;
      return;
    }

    const phase = spotlightPhase(this.elapsedMs, SWEEP_PERIOD_MS);
    this.spotX = spotlightX(phase, 1);
    this.spotIntensity = 0.55;
    this.spotLock = 0;
  }

  private draw(): void {
    const gl = this.gl;
    const canvas = this.canvas;
    if (!gl || !canvas || !this.bgProgram || !this.posterProgram) return;

    this.updateSpotlight();
    const aspect = this.aspect();

    gl.clear(gl.COLOR_BUFFER_BIT);

    gl.useProgram(this.bgProgram);
    gl.bindBuffer(gl.ARRAY_BUFFER, this.bgVbo);
    const aPos = gl.getAttribLocation(this.bgProgram, "aPos");
    gl.enableVertexAttribArray(aPos);
    gl.vertexAttribPointer(aPos, 2, gl.FLOAT, false, 0, 0);
    gl.uniform2f(this.bgLoc.uRes, canvas.width, canvas.height);
    gl.uniform1f(this.bgLoc.uTime, this.elapsedMs / 1000);
    gl.uniform1f(this.bgLoc.uSpotX, this.spotX);
    gl.uniform1f(this.bgLoc.uSpotIntensity, this.spotIntensity);
    gl.uniform1f(this.bgLoc.uSpotWidth, SPOT_WIDTH_CLIP);
    gl.uniform1f(this.bgLoc.uLock, this.spotLock);
    gl.drawArrays(gl.TRIANGLES, 0, 3);
    gl.disableVertexAttribArray(aPos);

    if (this.posters.length === 0) return;

    gl.useProgram(this.posterProgram);
    gl.bindBuffer(gl.ARRAY_BUFFER, this.quadVbo);
    gl.enableVertexAttribArray(this.posterAttr);
    gl.vertexAttribPointer(this.posterAttr, 2, gl.FLOAT, false, 0, 0);
    gl.uniform1f(this.posterLoc.uAspect, aspect);
    gl.uniform1i(this.posterLoc.uTex, 0);
    gl.activeTexture(gl.TEXTURE0);
    gl.uniform1f(this.posterLoc.uAlpha, 1);

    const visibleHalfWidth = aspect + BASE_HALF_WIDTH;
    this.tileOffsets[0] = -this.period;
    this.tileOffsets[1] = 0;
    this.tileOffsets[2] = this.period;
    for (let i = 0; i < this.posters.length; i++) {
      const slot = this.slots[i];
      const poster = this.posters[i];
      if (!slot || !poster) continue;
      const tex = poster.tex;
      const loaded = this.textures?.isLoaded(poster.id) ?? false;
      // A poster is emphasized when hovered OR when it is the elected winner.
      // The winner uses its own (stronger, pointer-independent) lift/scale so it
      // stands out even when the mouse is elsewhere.
      const isHover = i === this.hoverIndex;
      const isLock = i === this.lockedIndex;
      const hoverEmph = isHover ? this.hoverLift : 0;
      const lockEmph = isLock ? this.lockLift : 0;
      const emph = Math.max(hoverEmph, lockEmph);
      const scaleBoost = lockEmph >= hoverEmph ? WINNER_SCALE_BOOST : HOVER_SCALE_BOOST;
      const liftAmt = lockEmph >= hoverEmph ? WINNER_LIFT : HOVER_LIFT;
      const boost = 1 + (scaleBoost - 1) * emph;

      for (const tile of this.tileOffsets) {
        const cx = slot.x - this.scrollOffset + tile;
        if (Math.abs(cx) > visibleHalfWidth) continue;
        // +y is up in clip space, so a positive offset here visually lifts
        // the poster.
        const cy = slot.y + BASELINE_Y + liftAmt * emph;
        const hw = BASE_HALF_WIDTH * slot.scale * boost;
        const hh = BASE_HALF_HEIGHT * slot.scale * boost;
        const clipCx = clamp(cx / aspect, -1, 1);
        const spotLight = clamp(1 - Math.abs(clipCx - this.spotX) / SPOT_WIDTH_CLIP, 0, 1);

        if (tex) gl.bindTexture(gl.TEXTURE_2D, tex);
        gl.uniform2f(this.posterLoc.uCenter, cx, cy);
        gl.uniform2f(this.posterLoc.uSize, hw, hh);
        gl.uniform3f(this.posterLoc.uPlaceholder, poster.placeholder[0], poster.placeholder[1], poster.placeholder[2]);
        gl.uniform1f(this.posterLoc.uLoaded, loaded ? 1 : 0);
        gl.uniform1f(this.posterLoc.uDepthFade, slot.z);
        gl.uniform1f(this.posterLoc.uHighlight, emph);
        gl.uniform1f(this.posterLoc.uSpotLight, spotLight);
        gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4);
      }
    }

    if (this.quality.reflections) this.drawReflections(visibleHalfWidth);

    gl.disableVertexAttribArray(this.posterAttr);
  }

  // A soft, cheap "floor" reflection: the front row only, mirrored below the
  // wall's lowest edge, faded via alpha blending. Skipped on the low tier.
  private drawReflections(visibleHalfWidth: number): void {
    const gl = this.gl;
    if (!gl) return;
    const floorY = BASELINE_Y - BASE_HALF_HEIGHT;

    gl.enable(gl.BLEND);
    gl.blendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA);
    gl.uniform1f(this.posterLoc.uAlpha, 0.14);
    gl.uniform1f(this.posterLoc.uHighlight, 0);
    gl.uniform1f(this.posterLoc.uSpotLight, 0.6);

    for (let i = 0; i < this.posters.length; i++) {
      const slot = this.slots[i];
      const poster = this.posters[i];
      if (!slot || slot.row !== 0 || !poster) continue;
      const tex = poster.tex;
      const loaded = this.textures?.isLoaded(poster.id) ?? false;
      const hw = BASE_HALF_WIDTH * slot.scale;
      const hh = BASE_HALF_HEIGHT * slot.scale;
      const cy = 2 * floorY - (slot.y + BASELINE_Y);

      for (const tile of this.tileOffsets) {
        const cx = slot.x - this.scrollOffset + tile;
        if (Math.abs(cx) > visibleHalfWidth) continue;
        if (tex) gl.bindTexture(gl.TEXTURE_2D, tex);
        gl.uniform2f(this.posterLoc.uCenter, cx, cy);
        gl.uniform2f(this.posterLoc.uSize, hw, hh);
        gl.uniform3f(this.posterLoc.uPlaceholder, poster.placeholder[0], poster.placeholder[1], poster.placeholder[2]);
        gl.uniform1f(this.posterLoc.uLoaded, loaded ? 1 : 0);
        gl.uniform1f(this.posterLoc.uDepthFade, slot.z);
        gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4);
      }
    }

    gl.disable(gl.BLEND);
  }
}
