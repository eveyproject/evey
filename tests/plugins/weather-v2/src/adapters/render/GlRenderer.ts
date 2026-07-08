// Raw-WebGL implementation of the SceneRenderer port.
//
// Memory discipline (the v1 OOM class is designed out):
//   - one WebGL context, one program, one VBO (fullscreen triangle), one
//     320px texture. No scene graph, no FBOs, no post-processing chain.
//   - the render loop allocates NOTHING: uniform state lives in two flat
//     Float64 property objects lerped in place.
//   - the drawing buffer is resized in place on container resize (no
//     recreate), capped by DPR clamp and a quality tier scale.
//   - everything is torn down in dispose(): observers, listeners, GL
//     objects, then the context itself via WEBGL_lose_context.
//
// CPU/GPU discipline:
//   - ~30 fps cap (24 on the low tier) via accumulator, skips entirely when
//     the tab is hidden or the widget is scrolled out of view.
//   - prefers-reduced-motion slows the scene instead of animating fully.
//   - time is wrapped at 4096 s so mediump GPUs never hit float precision
//     drift (one pattern reshuffle per ~68 min, imperceptible).

import type { SceneRenderer, RenderTarget } from "../../domain/ports/SceneRenderer";
import { VERT_SRC, FRAG_SRC } from "./shaders";
import { initThunderState, stepThunder, type ThunderState } from "../../domain/services/thunder";
import { initShootingStar, shootingStar, type ShootingStar } from "../../domain/services/shootingStar";
// Real NASA imagery (public domain). Moon: NASA/GSFC (FullMoon2010). Sun:
// NASA/SDO and the AIA science team - one full-disc AIA 171 image, cropped
// tight so the disc fills the frame. Keyed by the ids in
// domain/services/sunModel.ts.
import moonUrl from "../../assets/moon.jpg";
import sunAia171 from "../../assets/sun_aia171.jpg";

const SUN_TEXTURES: Record<string, string> = {
  aia171: sunAia171,
};
const DEFAULT_SUN_ID = "aia171";

const TIME_WRAP_S = 4096;

// Gate to keep the shooting-star check meaningfully rare from a night-sky
// standpoint: clear-enough, dark-enough, and not on the lowest quality tier
// (it's a cheap extra branch in the shader, but skip it on weak devices).
const STAR_NIGHT_MIN = 0.85;
const STAR_CLEARNESS_MIN = 0.6;

interface Quality {
  scale: number; // backing-store scale on top of clamped DPR
  fps: number;
  maxDpr: number;
}

// Result is cached module-wide: the probe context is created at most once per
// document, and its result is reused by every widget mount / context restore.
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
    // Intentionally NOT calling WEBGL_lose_context.loseContext() here: Firefox
    // logs "WebGL context was lost." to the console on every such call, once
    // per widget mount. The probe is a single tiny context the GC reclaims once
    // this canvas goes out of scope, and caching guarantees at most one ever.
    if (isWebGL2 && maxTex >= 8192 && cores >= 4) return (cachedQuality = { scale: 1.0, fps: 30, maxDpr: 1.5 });
    if (maxTex >= 4096) return (cachedQuality = { scale: 0.75, fps: 30, maxDpr: 1.25 });
    return (cachedQuality = { scale: 0.5, fps: 24, maxDpr: 1 });
  } catch {
    return (cachedQuality = { scale: 0.5, fps: 24, maxDpr: 1 });
  }
}

// Flat uniform state: every animatable scalar, plus palette channels.
interface UState {
  cover: number; density: number; wind: number;
  rain: number; snow: number; fog: number;
  sunX: number; sunY: number; sunI: number; golden: number; night: number;
  moonX: number; moonY: number; moonI: number; moonPhase: number;
  aurora: number;
  cloudR: number; cloudG: number; cloudB: number;
  shadR: number; shadG: number; shadB: number;
  sunlR: number; sunlG: number; sunlB: number;
  fogR: number; fogG: number; fogB: number;
  glowR: number; glowG: number; glowB: number;
}

function emptyState(): UState {
  return {
    cover: 0, density: 0, wind: 0.3,
    rain: 0, snow: 0, fog: 0,
    sunX: 0.5, sunY: 0.75, sunI: 0, golden: 0, night: 0,
    moonX: 0.5, moonY: 0.75, moonI: 0, moonPhase: 0.5,
    aurora: 0,
    cloudR: 0.84, cloudG: 0.87, cloudB: 0.93,
    shadR: 0.18, shadG: 0.24, shadB: 0.36,
    sunlR: 1, sunlG: 0.95, sunlB: 0.85,
    fogR: 0.9, fogG: 0.9, fogB: 0.9,
    glowR: 1, glowG: 0.78, glowB: 0.55,
  };
}

const U_KEYS = Object.keys(emptyState()) as Array<keyof UState>;

export class GlRenderer implements SceneRenderer {
  private host: HTMLElement | null = null;
  private canvas: HTMLCanvasElement | null = null;
  private gl: WebGLRenderingContext | null = null;
  private program: WebGLProgram | null = null;
  private vbo: WebGLBuffer | null = null;
  private moonTex: WebGLTexture | null = null;
  private sunTex: WebGLTexture | null = null;
  private loc: Record<string, WebGLUniformLocation | null> = {};

  private quality = detectQuality();
  private reducedMotion =
    typeof window !== "undefined" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  private current = emptyState();
  private target = emptyState();
  private hasTarget = false;

  private cloudSpeed = 0.15;
  private thunderRate = 0;
  private windDrift = 0;
  private thunder: ThunderState = initThunderState();

  // Per-mount seed so the shooting-star schedule differs between widget
  // instances/reloads while staying a pure function of (time, seed) within
  // one mount - see domain/services/shootingStar.ts.
  private starSeed = Math.random() * 1000;
  private star: ShootingStar = initShootingStar();

  private sunModelId = DEFAULT_SUN_ID;

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

  setTarget(t: RenderTarget): void {
    const { params, cycle, palette } = t;

    // Daily sun skin: swap the texture in place when the model id changes
    // (initial paint + UTC-midnight rollover while the widget stays open).
    if (t.sunModelId && t.sunModelId !== this.sunModelId && SUN_TEXTURES[t.sunModelId]) {
      this.sunModelId = t.sunModelId;
      if (this.sunTex) this.loadImageInto(this.sunTex, SUN_TEXTURES[t.sunModelId]);
    }

    const tg = this.target;
    tg.cover = params.cloudCover;
    tg.density = params.cloudDensity;
    tg.rain = params.precipitation === "rain" ? params.precipIntensity : 0;
    tg.snow = params.precipitation === "snow" ? params.precipIntensity : 0;
    tg.fog = params.fog;
    // Global celestial dimming: heavy weather mutes the discs; the residual
    // per-pixel occlusion happens in the shader against the actual clouds.
    const precipCut =
      params.precipIntensity *
      (params.mood === "storm" ? 0.75 : params.mood === "rain" ? 0.45 : 0.25);
    const veil = (1 - params.fog * 0.85) * (1 - precipCut);
    tg.sunX = cycle.sunX;
    tg.sunY = 1 - cycle.sunY; // domain y=0 top -> shader y up
    tg.sunI = cycle.sunIntensity * (1 - params.cloudCover * 0.12) * veil;
    tg.golden = cycle.golden;
    tg.moonX = cycle.moonX;
    tg.moonY = 1 - cycle.moonY;
    tg.moonI = cycle.moonIntensity * (1 - params.cloudCover * 0.25) * veil;
    tg.moonPhase = t.moonPhase;
    tg.night = cycle.night;
    tg.aurora = t.auroraIntensity;
    [tg.cloudR, tg.cloudG, tg.cloudB] = palette.cloudColor;
    [tg.shadR, tg.shadG, tg.shadB] = palette.cloudShadow;
    [tg.sunlR, tg.sunlG, tg.sunlB] = palette.sunlight;
    [tg.fogR, tg.fogG, tg.fogB] = palette.fogColor;
    [tg.glowR, tg.glowG, tg.glowB] = palette.glowColor;

    this.cloudSpeed = params.cloudSpeed;
    this.thunderRate = params.thunderRate;

    if (!this.hasTarget) {
      // First data: snap instead of lerping from the void.
      Object.assign(this.current, this.target);
      this.hasTarget = true;
    }
  }

  setSunImageData(dataUrl: string): void {
    // Upgrade the current sun texture to the live image. The bundled snapshot
    // stays as the placeholder until this resolves, so there's never a blank
    // sun. A later daily model change reloads the bundled default, and the app
    // fetches that day's live image again.
    if (this.sunTex) this.loadImageInto(this.sunTex, dataUrl);
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
      if (this.program) gl.deleteProgram(this.program);
      if (this.vbo) gl.deleteBuffer(this.vbo);
      if (this.moonTex) gl.deleteTexture(this.moonTex);
      if (this.sunTex) gl.deleteTexture(this.sunTex);
      (gl.getExtension("WEBGL_lose_context") as { loseContext(): void } | null)?.loseContext();
    }
    canvas?.remove();
    this.gl = null;
    this.program = null;
    this.vbo = null;
    this.moonTex = null;
    this.sunTex = null;
    this.canvas = null;
    this.host = null;
  }

  // ---- internals ----------------------------------------------------------

  // Decode a bundled data-url image and upload it into an existing texture.
  // No CORS concern: the images are inlined as data: urls at build time.
  private loadImageInto(tex: WebGLTexture | null, url: string): void {
    if (!tex) return;
    const img = new Image();
    img.onload = () => {
      const g = this.gl;
      if (!g || !tex) return;
      g.bindTexture(g.TEXTURE_2D, tex);
      g.pixelStorei(g.UNPACK_FLIP_Y_WEBGL, 1);
      g.texImage2D(g.TEXTURE_2D, 0, g.RGB, g.RGB, g.UNSIGNED_BYTE, img);
      g.pixelStorei(g.UNPACK_FLIP_Y_WEBGL, 0);
    };
    img.src = url;
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
    if (!gl) return; // overlay still shows the data; scene stays empty
    this.gl = gl;

    const compile = (type: number, src: string): WebGLShader | null => {
      const sh = gl.createShader(type);
      if (!sh) return null;
      gl.shaderSource(sh, src);
      gl.compileShader(sh);
      if (!gl.getShaderParameter(sh, gl.COMPILE_STATUS)) {
        console.error("weather-v2 shader:", gl.getShaderInfoLog(sh));
        gl.deleteShader(sh);
        return null;
      }
      return sh;
    };
    const vs = compile(gl.VERTEX_SHADER, VERT_SRC);
    const fs = compile(gl.FRAGMENT_SHADER, FRAG_SRC);
    if (!vs || !fs) return;
    const program = gl.createProgram();
    if (!program) return;
    gl.attachShader(program, vs);
    gl.attachShader(program, fs);
    gl.linkProgram(program);
    gl.deleteShader(vs);
    gl.deleteShader(fs);
    if (!gl.getProgramParameter(program, gl.LINK_STATUS)) {
      console.error("weather-v2 link:", gl.getProgramInfoLog(program));
      gl.deleteProgram(program);
      return;
    }
    this.program = program;
    gl.useProgram(program);

    // Fullscreen triangle.
    this.vbo = gl.createBuffer();
    gl.bindBuffer(gl.ARRAY_BUFFER, this.vbo);
    gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1, -1, 3, -1, -1, 3]), gl.STATIC_DRAW);
    const aPos = gl.getAttribLocation(program, "aPos");
    gl.enableVertexAttribArray(aPos);
    gl.vertexAttribPointer(aPos, 2, gl.FLOAT, false, 0, 0);

    for (const name of [
      "uRes", "uTime", "uCover", "uDensity", "uWindDrift", "uWind", "uRain", "uSnow",
      "uFog", "uFlash", "uFlashPos", "uSunPos", "uSunI", "uGolden", "uMoonPos", "uMoonI",
      "uMoonPhase", "uMoonTex", "uSunTex", "uCloudCol", "uCloudShadow", "uSunlight",
      "uFogCol", "uGlowCol", "uAurora", "uStarPos", "uStarDir", "uStarLife",
    ]) {
      this.loc[name] = gl.getUniformLocation(program, name);
    }

    // Photo textures: 1x1 placeholder each, swapped when the image decodes.
    const newTex = (placeholder: [number, number, number]): WebGLTexture | null => {
      const tex = gl.createTexture();
      gl.bindTexture(gl.TEXTURE_2D, tex);
      gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGB, 1, 1, 0, gl.RGB, gl.UNSIGNED_BYTE, new Uint8Array(placeholder));
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
      return tex;
    };
    this.moonTex = newTex([90, 90, 100]);
    this.sunTex = newTex([255, 160, 40]);
    this.loadImageInto(this.moonTex, moonUrl);
    this.loadImageInto(this.sunTex, SUN_TEXTURES[this.sunModelId] ?? SUN_TEXTURES[DEFAULT_SUN_ID]);

    gl.disable(gl.BLEND);
    gl.disable(gl.DEPTH_TEST);
    gl.clearColor(0, 0, 0, 0);
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

    if (document.hidden || !this.inView || this.lost || !this.gl || !this.program) {
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
    this.time = (this.time + step * motion) % TIME_WRAP_S;
    this.windDrift = (this.windDrift + step * motion * this.cloudSpeed * 0.15) % 512;

    // Thunder envelope: mutates this.thunder in place, see
    // domain/services/thunder.ts - zero allocation in this per-frame path.
    stepThunder(this.thunder, step, this.thunderRate, Math.random);

    // In-place exponential smoothing toward the target.
    const k = 1 - Math.exp(-3 * step);
    const cur = this.current;
    const tg = this.target;
    for (let i = 0; i < U_KEYS.length; i++) {
      const key = U_KEYS[i];
      cur[key] += (tg[key] - cur[key]) * k;
    }

    // Occasional shooting star: only rolled on a clear-enough, dark-enough
    // sky, and skipped on the lowest quality tier.
    const clearness = (1 - cur.cover) * (1 - cur.rain) * (1 - cur.snow) * (1 - cur.fog);
    const starGate =
      this.quality.scale >= 0.75 && cur.night >= STAR_NIGHT_MIN && clearness >= STAR_CLEARNESS_MIN;
    // Mutates this.star in place, see domain/services/shootingStar.ts - zero
    // allocation in this per-frame path.
    shootingStar(this.star, this.time, this.starSeed, starGate);

    this.draw();
  };

  private draw(): void {
    const gl = this.gl;
    const canvas = this.canvas;
    if (!gl || !canvas) return;
    const c = this.current;
    const L = this.loc;

    gl.clear(gl.COLOR_BUFFER_BIT);
    gl.uniform2f(L.uRes, canvas.width, canvas.height);
    gl.uniform1f(L.uTime, this.time);
    gl.uniform1f(L.uCover, c.cover);
    gl.uniform1f(L.uDensity, c.density);
    gl.uniform1f(L.uWindDrift, this.windDrift);
    gl.uniform1f(L.uWind, c.wind);
    gl.uniform1f(L.uRain, c.rain);
    gl.uniform1f(L.uSnow, c.snow);
    gl.uniform1f(L.uFog, c.fog);
    gl.uniform1f(L.uFlash, this.thunder.flash);
    gl.uniform2f(L.uFlashPos, this.thunder.flashX, 1 - this.thunder.flashY);
    gl.uniform2f(L.uSunPos, c.sunX, c.sunY);
    gl.uniform1f(L.uSunI, c.sunI);
    gl.uniform1f(L.uGolden, c.golden);
    gl.uniform2f(L.uMoonPos, c.moonX, c.moonY);
    gl.uniform1f(L.uMoonI, c.moonI);
    gl.uniform1f(L.uMoonPhase, c.moonPhase);
    gl.uniform1i(L.uMoonTex, 0);
    gl.uniform1i(L.uSunTex, 1);
    gl.uniform3f(L.uCloudCol, c.cloudR, c.cloudG, c.cloudB);
    gl.uniform3f(L.uCloudShadow, c.shadR, c.shadG, c.shadB);
    gl.uniform3f(L.uSunlight, c.sunlR, c.sunlG, c.sunlB);
    gl.uniform3f(L.uFogCol, c.fogR, c.fogG, c.fogB);
    gl.uniform3f(L.uGlowCol, c.glowR, c.glowG, c.glowB);
    gl.uniform1f(L.uAurora, c.aurora);
    gl.uniform2f(L.uStarPos, this.star.x, 1 - this.star.y);
    gl.uniform2f(L.uStarDir, this.star.dirX, -this.star.dirY);
    gl.uniform1f(L.uStarLife, this.star.visible ? this.star.life : 0);
    gl.activeTexture(gl.TEXTURE0);
    gl.bindTexture(gl.TEXTURE_2D, this.moonTex);
    gl.activeTexture(gl.TEXTURE1);
    gl.bindTexture(gl.TEXTURE_2D, this.sunTex);
    gl.drawArrays(gl.TRIANGLES, 0, 3);
  }
}
