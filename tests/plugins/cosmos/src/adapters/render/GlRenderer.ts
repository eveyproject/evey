// Raw-WebGL implementation of the SceneRenderer port.
//
// Memory/CPU discipline (same philosophy as the weather-v2 plugin this repo
// already ships): one WebGL context, two minimal programs (sky background +
// bodies/lines), no framebuffers, no scene graph. The render loop allocates
// nothing per frame - buffers are only rebuilt when a new SkyScene arrives
// (every tick from the application layer, not every frame: real star/planet
// positions barely move between ticks, so there is nothing to interpolate).
// Everything is torn down in dispose(): observers, GL objects, then the
// context itself via WEBGL_lose_context.

import type { SceneRenderer } from "../../domain/ports/SceneRenderer";
import type { SkyScene } from "../../domain/model/scene";
import { SKY_VERT_SRC, SKY_FRAG_SRC, BODY_VERT_SRC, BODY_FRAG_SRC } from "./shaders";

interface Quality {
  scale: number;
  fps: number;
  maxDpr: number;
}

let cachedQuality: Quality | null = null;
function detectQuality(): Quality {
  if (cachedQuality) return cachedQuality;
  try {
    const c = document.createElement("canvas");
    const gl = (c.getContext("webgl2") || c.getContext("webgl")) as WebGLRenderingContext | null;
    if (!gl) return (cachedQuality = { scale: 0.5, fps: 24, maxDpr: 1 });
    const maxTex = gl.getParameter(gl.MAX_TEXTURE_SIZE) as number;
    const cores = navigator.hardwareConcurrency ?? 2;
    const isWebGL2 = "drawBuffers" in gl;
    if (isWebGL2 && maxTex >= 8192 && cores >= 4) return (cachedQuality = { scale: 1.0, fps: 30, maxDpr: 1.5 });
    if (maxTex >= 4096) return (cachedQuality = { scale: 0.75, fps: 30, maxDpr: 1.25 });
    return (cachedQuality = { scale: 0.5, fps: 24, maxDpr: 1 });
  } catch {
    return (cachedQuality = { scale: 0.5, fps: 24, maxDpr: 1 });
  }
}

const KIND_LINE = 0, KIND_STAR = 1, KIND_PLANET = 2, KIND_MOON = 3, KIND_SUN = 4;
const STRIDE_FLOATS = 8; // x,y,r,g,b,size,kind,seed

const SHOOT_MIN_GAP_S = 18;
const SHOOT_MAX_GAP_S = 75;
const SHOOT_DURATION_S = 0.9;

export class GlRenderer implements SceneRenderer {
  private host: HTMLElement | null = null;
  private canvas: HTMLCanvasElement | null = null;
  private gl: WebGLRenderingContext | null = null;

  private skyProgram: WebGLProgram | null = null;
  private skyVbo: WebGLBuffer | null = null;
  private skyLoc: Record<string, WebGLUniformLocation | null> = {};

  private bodyProgram: WebGLProgram | null = null;
  private pointsVbo: WebGLBuffer | null = null;
  private linesVbo: WebGLBuffer | null = null;
  private bodyLoc: Record<string, WebGLUniformLocation | null> = {};
  private bodyAttr: Record<string, number> = {};

  private pointCount = 0;
  private lineVertCount = 0;

  private quality = detectQuality();
  private reducedMotion =
    typeof window !== "undefined" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  private starVisibility = 0;
  private twilightWarmth = 0;
  private sunX = 0;
  private sunY = 0;
  private moonPhase = 0.5;
  private moonWaxing = 1;

  private shootTimerS = 6 + Math.random() * SHOOT_MIN_GAP_S;
  private shootActive = false;
  private shootProgress = -1;
  private shootFrom: [number, number] = [0, 0];
  private shootTo: [number, number] = [0, 0];

  private raf = 0;
  private running = false;
  private inView = true;
  private lastTs = 0;
  private frameAcc = 0;
  private time = 0;
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

  setTarget(scene: SkyScene): void {
    this.starVisibility = scene.starVisibility;
    this.twilightWarmth = scene.twilightWarmth;
    this.sunX = scene.sun.x;
    this.sunY = scene.sun.y;
    this.moonPhase = scene.moon.phaseFraction;
    this.moonWaxing = scene.moon.waxing ? 1 : 0;

    this.uploadPoints(scene);
    this.uploadLines(scene);

    // Only ever schedule a shooting star when the sky is dark enough to see it.
    if (this.starVisibility < 0.4) {
      this.shootActive = false;
      this.shootProgress = -1;
    }
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
      if (this.skyProgram) gl.deleteProgram(this.skyProgram);
      if (this.bodyProgram) gl.deleteProgram(this.bodyProgram);
      if (this.skyVbo) gl.deleteBuffer(this.skyVbo);
      if (this.pointsVbo) gl.deleteBuffer(this.pointsVbo);
      if (this.linesVbo) gl.deleteBuffer(this.linesVbo);
      (gl.getExtension("WEBGL_lose_context") as { loseContext(): void } | null)?.loseContext();
    }
    canvas?.remove();
    this.gl = null;
    this.skyProgram = null;
    this.bodyProgram = null;
    this.skyVbo = null;
    this.pointsVbo = null;
    this.linesVbo = null;
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
      console.error("cosmos shader:", gl.getShaderInfoLog(sh));
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
      console.error("cosmos link:", gl.getProgramInfoLog(program));
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

    this.skyProgram = this.link(SKY_VERT_SRC, SKY_FRAG_SRC);
    this.bodyProgram = this.link(BODY_VERT_SRC, BODY_FRAG_SRC);
    if (!this.skyProgram || !this.bodyProgram) return;

    this.skyVbo = gl.createBuffer();
    gl.bindBuffer(gl.ARRAY_BUFFER, this.skyVbo);
    gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1, -1, 3, -1, -1, 3]), gl.STATIC_DRAW);
    for (const name of ["uRes", "uTime", "uStarVis", "uTwilight", "uSunPos", "uShootStar", "uShootProgress"]) {
      this.skyLoc[name] = gl.getUniformLocation(this.skyProgram, name);
    }

    this.pointsVbo = gl.createBuffer();
    this.linesVbo = gl.createBuffer();
    for (const name of ["uTime", "uStarVis", "uMoonPhase", "uMoonWaxing"]) {
      this.bodyLoc[name] = gl.getUniformLocation(this.bodyProgram, name);
    }
    for (const name of ["aPos", "aColor", "aSize", "aKind", "aSeed"]) {
      this.bodyAttr[name] = gl.getAttribLocation(this.bodyProgram, name);
    }

    gl.enable(gl.BLEND);
    gl.blendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA);
    gl.disable(gl.DEPTH_TEST);
    gl.clearColor(0, 0, 0, 1);
  }

  private uploadPoints(scene: SkyScene): void {
    const gl = this.gl;
    if (!gl || !this.pointsVbo) return;

    const rows: number[] = [];
    const pushBody = (x: number, y: number, visible: boolean, color: [number, number, number], size: number, kind: number, seed: number) => {
      if (!visible) return;
      rows.push(x, y, color[0], color[1], color[2], size, kind, seed);
    };

    for (let i = 0; i < scene.stars.length; i++) {
      const s = scene.stars[i];
      // Brighter (lower magnitude) stars get bigger points.
      const size = Math.max(1.5, 6.5 - s.mag * 1.15) * (this.canvas ? this.canvas.height / 600 : 1);
      pushBody(s.x, s.y, s.visible, s.color, size, KIND_STAR, (i * 0.61803398875) % 1);
    }
    for (const p of scene.planets) {
      const color = PLANET_COLORS[p.name];
      const size = 9 * (this.canvas ? this.canvas.height / 600 : 1);
      pushBody(p.x, p.y, p.visible, color, size, KIND_PLANET, 0);
    }
    const moonSize = 26 * (this.canvas ? this.canvas.height / 600 : 1);
    pushBody(scene.moon.x, scene.moon.y, scene.moon.visible, [1, 1, 1], moonSize, KIND_MOON, 0);
    const sunSize = 34 * (this.canvas ? this.canvas.height / 600 : 1);
    pushBody(scene.sun.x, scene.sun.y, scene.sun.visible, [1, 0.8, 0.4], sunSize, KIND_SUN, 0);

    this.pointCount = rows.length / STRIDE_FLOATS;
    gl.bindBuffer(gl.ARRAY_BUFFER, this.pointsVbo);
    gl.bufferData(gl.ARRAY_BUFFER, new Float32Array(rows), gl.DYNAMIC_DRAW);
  }

  private uploadLines(scene: SkyScene): void {
    const gl = this.gl;
    if (!gl || !this.linesVbo) return;
    const rows: number[] = [];
    for (const l of scene.lines) {
      if (!l.visible) continue;
      rows.push(l.ax, l.ay, 0.55, 0.6, 0.75, 0, KIND_LINE, 0);
      rows.push(l.bx, l.by, 0.55, 0.6, 0.75, 0, KIND_LINE, 0);
    }
    this.lineVertCount = rows.length / STRIDE_FLOATS;
    gl.bindBuffer(gl.ARRAY_BUFFER, this.linesVbo);
    gl.bufferData(gl.ARRAY_BUFFER, new Float32Array(rows), gl.DYNAMIC_DRAW);
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
    this.time += step * motion;
    this.advanceShootingStar(step * motion);

    this.draw();
  };

  private advanceShootingStar(step: number): void {
    if (this.shootActive) {
      this.shootProgress += step / SHOOT_DURATION_S;
      if (this.shootProgress >= 1) {
        this.shootActive = false;
        this.shootProgress = -1;
        this.shootTimerS = SHOOT_MIN_GAP_S + Math.random() * (SHOOT_MAX_GAP_S - SHOOT_MIN_GAP_S);
      }
      return;
    }
    if (this.starVisibility < 0.4) return; // only at night
    this.shootTimerS -= step;
    if (this.shootTimerS <= 0) {
      const angle = Math.random() * Math.PI * 2;
      const dist = 0.5 + Math.random() * 0.4;
      const fromX = (Math.random() - 0.5) * 1.6;
      const fromY = (Math.random() - 0.5) * 1.6;
      this.shootFrom = [fromX, fromY];
      this.shootTo = [fromX + Math.cos(angle) * dist, fromY + Math.sin(angle) * dist];
      this.shootActive = true;
      this.shootProgress = 0;
    }
  }

  private draw(): void {
    const gl = this.gl;
    const canvas = this.canvas;
    if (!gl || !canvas) return;

    gl.clear(gl.COLOR_BUFFER_BIT);

    // --- sky background ---
    if (this.skyProgram) {
      gl.useProgram(this.skyProgram);
      gl.bindBuffer(gl.ARRAY_BUFFER, this.skyVbo);
      const aPos = gl.getAttribLocation(this.skyProgram, "aPos");
      gl.enableVertexAttribArray(aPos);
      gl.vertexAttribPointer(aPos, 2, gl.FLOAT, false, 0, 0);

      const L = this.skyLoc;
      gl.uniform2f(L.uRes, canvas.width, canvas.height);
      gl.uniform1f(L.uTime, this.time);
      gl.uniform1f(L.uStarVis, this.starVisibility);
      gl.uniform1f(L.uTwilight, this.twilightWarmth);
      gl.uniform2f(L.uSunPos, this.sunX, this.sunY);
      gl.uniform4f(L.uShootStar, this.shootFrom[0], this.shootFrom[1], this.shootTo[0], this.shootTo[1]);
      gl.uniform1f(L.uShootProgress, this.shootActive ? this.shootProgress : -1);
      gl.drawArrays(gl.TRIANGLES, 0, 3);
      gl.disableVertexAttribArray(aPos);
    }

    // --- lines + points ---
    if (this.bodyProgram) {
      gl.useProgram(this.bodyProgram);
      gl.uniform1f(this.bodyLoc.uTime, this.time);
      gl.uniform1f(this.bodyLoc.uStarVis, this.starVisibility);
      gl.uniform1f(this.bodyLoc.uMoonPhase, this.moonPhase);
      gl.uniform1f(this.bodyLoc.uMoonWaxing, this.moonWaxing);

      const bindAttribs = (buf: WebGLBuffer | null) => {
        gl.bindBuffer(gl.ARRAY_BUFFER, buf);
        const stride = STRIDE_FLOATS * 4;
        gl.enableVertexAttribArray(this.bodyAttr.aPos);
        gl.vertexAttribPointer(this.bodyAttr.aPos, 2, gl.FLOAT, false, stride, 0);
        gl.enableVertexAttribArray(this.bodyAttr.aColor);
        gl.vertexAttribPointer(this.bodyAttr.aColor, 3, gl.FLOAT, false, stride, 8);
        gl.enableVertexAttribArray(this.bodyAttr.aSize);
        gl.vertexAttribPointer(this.bodyAttr.aSize, 1, gl.FLOAT, false, stride, 20);
        gl.enableVertexAttribArray(this.bodyAttr.aKind);
        gl.vertexAttribPointer(this.bodyAttr.aKind, 1, gl.FLOAT, false, stride, 24);
        gl.enableVertexAttribArray(this.bodyAttr.aSeed);
        gl.vertexAttribPointer(this.bodyAttr.aSeed, 1, gl.FLOAT, false, stride, 28);
      };

      if (this.lineVertCount > 0) {
        bindAttribs(this.linesVbo);
        gl.drawArrays(gl.LINES, 0, this.lineVertCount);
      }
      if (this.pointCount > 0) {
        bindAttribs(this.pointsVbo);
        gl.drawArrays(gl.POINTS, 0, this.pointCount);
      }
    }
  }
}

const PLANET_COLORS: Record<string, [number, number, number]> = {
  venus: [1.0, 0.96, 0.85],
  mars: [1.0, 0.55, 0.4],
  jupiter: [0.95, 0.87, 0.72],
  saturn: [0.95, 0.9, 0.7],
};
