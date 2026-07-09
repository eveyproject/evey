// Raw-WebGL implementation of the SceneRenderer port.
//
// Memory/CPU discipline (same philosophy as the weather-v2/cosmos plugins
// this repo already ships): one WebGL context, two minimal programs (veil
// background + particle points), no framebuffers, no scene graph. The
// particle VBO is uploaded exactly ONCE at mount (positions are computed in
// the vertex shader from uTime/uPointer, not re-uploaded per frame - see
// shaders.ts). The render loop itself allocates nothing: the palette frame,
// animation phases and pointer state are all flat objects mutated in place
// every tick. Everything is torn down in dispose(): observers, listeners,
// GL objects, then the context itself via WEBGL_lose_context.
//
// CPU/GPU discipline: quality-tiered fps cap + DPR clamp + veil-layer/
// caustics gating, pauses entirely when the tab is hidden or the widget is
// scrolled out of view, prefers-reduced-motion slows the scene instead of
// stopping it, survives WebGL context loss.

import type { SceneRenderer } from "../../domain/ports/SceneRenderer";
import type { RenderTarget, PresetDef } from "../../domain/model/scene";
import { PRESETS, DEFAULT_PRESET } from "../../domain/services/presets";
import { computePaletteFrame } from "../../domain/services/paletteDrift";
import { createParticleField, particleBudgetForTier, type ParticleSeed, type QualityTier } from "../../domain/services/particleField";
import { createPointerState, stepPointer, type PointerState } from "../../domain/services/pointerEase";
import { createAnimationState, stepAnimation, type AnimationState } from "../../domain/services/animationState";
import { VERT_SRC, VEIL_FRAG_SRC, PARTICLE_VERT_SRC, PARTICLE_FRAG_SRC } from "./shaders";

interface Quality {
  scale: number; // backing-store scale on top of clamped DPR
  fps: number;
  maxDpr: number;
  extraLayer: boolean; // third veil curtain
  caustics: boolean; // liquid-light pass
  particleTier: QualityTier;
}

// Result is cached module-wide: the probe context is created at most once
// per document, and its result is reused by every widget mount / context
// restore.
let cachedQuality: Quality | null = null;
function detectQuality(): Quality {
  if (cachedQuality) return cachedQuality;
  try {
    const c = document.createElement("canvas");
    const gl = (c.getContext("webgl2") || c.getContext("webgl")) as WebGLRenderingContext | null;
    if (!gl) return (cachedQuality = { scale: 0.5, fps: 24, maxDpr: 1, extraLayer: false, caustics: false, particleTier: "low" });
    const maxTex = gl.getParameter(gl.MAX_TEXTURE_SIZE) as number;
    const cores = navigator.hardwareConcurrency ?? 2;
    const isWebGL2 = "drawBuffers" in gl;
    if (isWebGL2 && maxTex >= 8192 && cores >= 4) {
      return (cachedQuality = { scale: 1.0, fps: 30, maxDpr: 1.5, extraLayer: true, caustics: true, particleTier: "high" });
    }
    if (maxTex >= 4096) {
      return (cachedQuality = { scale: 0.75, fps: 30, maxDpr: 1.25, extraLayer: true, caustics: false, particleTier: "mid" });
    }
    return (cachedQuality = { scale: 0.5, fps: 24, maxDpr: 1, extraLayer: false, caustics: false, particleTier: "low" });
  } catch {
    return (cachedQuality = { scale: 0.5, fps: 24, maxDpr: 1, extraLayer: false, caustics: false, particleTier: "low" });
  }
}

const PARTICLE_STRIDE_FLOATS = 6; // baseX, baseY, depth, size, phase, speed

export class GlRenderer implements SceneRenderer {
  private host: HTMLElement | null = null;
  private canvas: HTMLCanvasElement | null = null;
  private gl: WebGLRenderingContext | null = null;

  private veilProgram: WebGLProgram | null = null;
  private veilVbo: WebGLBuffer | null = null;
  private veilLoc: Record<string, WebGLUniformLocation | null> = {};

  private particleProgram: WebGLProgram | null = null;
  private particleVbo: WebGLBuffer | null = null;
  private particleLoc: Record<string, WebGLUniformLocation | null> = {};
  private particleAttr: Record<string, number> = {};
  private particleSeeds: ParticleSeed[] = [];
  private particleCount = 0;

  private quality = detectQuality();
  private reducedMotion =
    typeof window !== "undefined" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  private presetDef: PresetDef = PRESETS[DEFAULT_PRESET];
  private intensity = 0.7;
  private speedMul = 1;
  private paletteElapsedMs = 0;

  private anim: AnimationState = createAnimationState();
  private pointer: PointerState = createPointerState();
  private pointerTargetX = 0;
  private pointerTargetY = 0;

  // Per-mount seed so the particle layout differs between widget
  // instances/reloads while staying a pure function of (count, seed) - see
  // domain/services/particleField.ts.
  private fieldSeed = Math.random() * 1000;

  private raf = 0;
  private running = false;
  private inView = true;
  private lastTs = 0;
  private frameAcc = 0;
  private lost = false;

  private resizeObs: ResizeObserver | null = null;
  private viewObs: IntersectionObserver | null = null;
  private onLost = (e: Event) => { e.preventDefault(); this.lost = true; };
  private onRestored = () => { this.lost = false; this.initGL(); };

  mount(host: HTMLElement): void {
    this.host = host;
    const canvas = document.createElement("canvas");
    canvas.style.cssText = "position:absolute;inset:0;width:100%;height:100%;display:block;";
    host.appendChild(canvas);
    this.canvas = canvas;

    canvas.addEventListener("webglcontextlost", this.onLost, false);
    canvas.addEventListener("webglcontextrestored", this.onRestored, false);

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

  setTarget(target: RenderTarget): void {
    this.presetDef = PRESETS[target.paletteId] ?? PRESETS[DEFAULT_PRESET];
    this.intensity = target.intensity;
    this.speedMul = target.speedMul;
  }

  setPointer(nx: number, ny: number): void {
    this.pointerTargetX = nx;
    this.pointerTargetY = ny;
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
    }
    if (gl) {
      if (this.veilProgram) gl.deleteProgram(this.veilProgram);
      if (this.particleProgram) gl.deleteProgram(this.particleProgram);
      if (this.veilVbo) gl.deleteBuffer(this.veilVbo);
      if (this.particleVbo) gl.deleteBuffer(this.particleVbo);
      (gl.getExtension("WEBGL_lose_context") as { loseContext(): void } | null)?.loseContext();
    }
    canvas?.remove();
    this.gl = null;
    this.veilProgram = null;
    this.particleProgram = null;
    this.veilVbo = null;
    this.particleVbo = null;
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
      console.error("lumina shader:", gl.getShaderInfoLog(sh));
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
      console.error("lumina link:", gl.getProgramInfoLog(program));
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

    this.veilProgram = this.link(VERT_SRC, VEIL_FRAG_SRC);
    this.particleProgram = this.link(PARTICLE_VERT_SRC, PARTICLE_FRAG_SRC);
    if (!this.veilProgram || !this.particleProgram) return;

    this.veilVbo = gl.createBuffer();
    gl.bindBuffer(gl.ARRAY_BUFFER, this.veilVbo);
    gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1, -1, 3, -1, -1, 3]), gl.STATIC_DRAW);
    for (const name of [
      "uRes", "uTime", "uDrift", "uCausticPhase", "uVignettePulse", "uPointer", "uIntensity",
      "uVeilA", "uVeilB", "uVeilC", "uCaustic", "uVignette", "uExtraLayer", "uCaustics",
    ]) {
      this.veilLoc[name] = gl.getUniformLocation(this.veilProgram, name);
    }

    this.particleVbo = gl.createBuffer();
    for (const name of ["uTime", "uPointer", "uIntensity", "uColor"]) {
      this.particleLoc[name] = gl.getUniformLocation(this.particleProgram, name);
    }
    for (const name of ["aBase", "aDepth", "aSize", "aPhase", "aSpeed"]) {
      this.particleAttr[name] = gl.getAttribLocation(this.particleProgram, name);
    }
    this.uploadParticles();

    gl.disable(gl.DEPTH_TEST);
    gl.clearColor(0, 0, 0, 1);
  }

  // Seeds and uploads the particle field ONCE (quality/particle count is
  // detected once at mount and never changes for the life of the widget -
  // same assumption as weather-v2/cosmos's cached quality tier).
  private uploadParticles(): void {
    const gl = this.gl;
    if (!gl || !this.particleVbo) return;
    const count = particleBudgetForTier(this.quality.particleTier);
    this.particleSeeds = createParticleField(count, this.fieldSeed);
    this.particleCount = this.particleSeeds.length;

    const rows = new Float32Array(this.particleCount * PARTICLE_STRIDE_FLOATS);
    for (let i = 0; i < this.particleCount; i++) {
      const s = this.particleSeeds[i];
      const o = i * PARTICLE_STRIDE_FLOATS;
      rows[o] = s.baseX;
      rows[o + 1] = s.baseY;
      rows[o + 2] = s.depth;
      rows[o + 3] = s.size;
      rows[o + 4] = s.phase;
      rows[o + 5] = s.speed;
    }
    gl.bindBuffer(gl.ARRAY_BUFFER, this.particleVbo);
    gl.bufferData(gl.ARRAY_BUFFER, rows, gl.STATIC_DRAW);
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

  private loop = (ts: number): void => {
    if (!this.running) return;
    this.raf = requestAnimationFrame(this.loop);

    if (document.hidden || !this.inView || this.lost || !this.gl) {
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
    const effSpeed = this.speedMul * motion;

    // In-place mutation of flat state objects - zero allocation in this
    // per-frame path (see the domain services for the step functions).
    stepAnimation(this.anim, step, effSpeed);
    stepPointer(this.pointer, this.pointerTargetX, this.pointerTargetY, step);
    this.paletteElapsedMs += step * 1000 * effSpeed;

    this.draw();
  };

  private draw(): void {
    const gl = this.gl;
    const canvas = this.canvas;
    if (!gl || !canvas) return;

    const frame = computePaletteFrame(this.presetDef, this.paletteElapsedMs);

    gl.disable(gl.BLEND);
    gl.clear(gl.COLOR_BUFFER_BIT);

    // --- veil background (opaque, must NOT blend or the clear color bleeds
    // through) ---
    if (this.veilProgram && this.veilVbo) {
      gl.useProgram(this.veilProgram);
      gl.bindBuffer(gl.ARRAY_BUFFER, this.veilVbo);
      const aPos = gl.getAttribLocation(this.veilProgram, "aPos");
      gl.enableVertexAttribArray(aPos);
      gl.vertexAttribPointer(aPos, 2, gl.FLOAT, false, 0, 0);

      const L = this.veilLoc;
      gl.uniform2f(L.uRes, canvas.width, canvas.height);
      gl.uniform1f(L.uTime, this.anim.time);
      gl.uniform1f(L.uDrift, this.anim.drift);
      gl.uniform1f(L.uCausticPhase, this.anim.causticPhase);
      gl.uniform1f(L.uVignettePulse, this.anim.vignettePulse);
      gl.uniform2f(L.uPointer, this.pointer.x, this.pointer.y);
      gl.uniform1f(L.uIntensity, this.intensity);
      gl.uniform3f(L.uVeilA, frame.veilA[0], frame.veilA[1], frame.veilA[2]);
      gl.uniform3f(L.uVeilB, frame.veilB[0], frame.veilB[1], frame.veilB[2]);
      gl.uniform3f(L.uVeilC, frame.veilC[0], frame.veilC[1], frame.veilC[2]);
      gl.uniform3f(L.uCaustic, frame.caustic[0], frame.caustic[1], frame.caustic[2]);
      gl.uniform3f(L.uVignette, frame.vignette[0], frame.vignette[1], frame.vignette[2]);
      gl.uniform1f(L.uExtraLayer, this.quality.extraLayer ? 1 : 0);
      gl.uniform1f(L.uCaustics, this.quality.caustics ? 1 : 0);
      gl.drawArrays(gl.TRIANGLES, 0, 3);
      gl.disableVertexAttribArray(aPos);
    }

    // --- particles (additive, on top) ---
    if (this.particleProgram && this.particleVbo && this.particleCount > 0) {
      gl.enable(gl.BLEND);
      gl.blendFunc(gl.SRC_ALPHA, gl.ONE);
      gl.useProgram(this.particleProgram);
      gl.bindBuffer(gl.ARRAY_BUFFER, this.particleVbo);
      const stride = PARTICLE_STRIDE_FLOATS * 4;
      const A = this.particleAttr;
      gl.enableVertexAttribArray(A.aBase);
      gl.vertexAttribPointer(A.aBase, 2, gl.FLOAT, false, stride, 0);
      gl.enableVertexAttribArray(A.aDepth);
      gl.vertexAttribPointer(A.aDepth, 1, gl.FLOAT, false, stride, 8);
      gl.enableVertexAttribArray(A.aSize);
      gl.vertexAttribPointer(A.aSize, 1, gl.FLOAT, false, stride, 12);
      gl.enableVertexAttribArray(A.aPhase);
      gl.vertexAttribPointer(A.aPhase, 1, gl.FLOAT, false, stride, 16);
      gl.enableVertexAttribArray(A.aSpeed);
      gl.vertexAttribPointer(A.aSpeed, 1, gl.FLOAT, false, stride, 20);

      const L = this.particleLoc;
      gl.uniform1f(L.uTime, this.anim.time);
      gl.uniform2f(L.uPointer, this.pointer.x, this.pointer.y);
      gl.uniform1f(L.uIntensity, this.intensity);
      gl.uniform3f(L.uColor, frame.caustic[0], frame.caustic[1], frame.caustic[2]);
      gl.drawArrays(gl.POINTS, 0, this.particleCount);

      gl.disableVertexAttribArray(A.aBase);
      gl.disableVertexAttribArray(A.aDepth);
      gl.disableVertexAttribArray(A.aSize);
      gl.disableVertexAttribArray(A.aPhase);
      gl.disableVertexAttribArray(A.aSpeed);
    }
  }
}
