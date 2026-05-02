// Cloud system — two-pass: FBM cloud FBO (half-res) + overlay composite.
// The overlay is rendered AFTER the EffectComposer (priority 2) so the HDR
// bloom pass never touches it — preventing bloom blow-out on cloud pixels.
//
// Palette modes (interpolated smoothly each frame):
//   day       — standard azure/white
//   golden    — warm orange/pink when |sunAltitude| < 0.20 (sunrise/sunset)
//   night     — deep blue/dark when sunAltitude < -0.15
import { useRef, useMemo, useEffect } from "react";
import { useThree, useFrame } from "@react-three/fiber";
import * as THREE from "three";
import { useFBO } from "@react-three/drei";
import { CLOUDS_VERT, CLOUDS_FRAG, CLOUD_UPSCALE_VERT, CLOUD_UPSCALE_FRAG } from "../shaders/clouds";
import { useSceneParams } from "./WeatherScene";
import { useFrameThrottle } from "../runtime/useFrameThrottle";
import { cloudTexRef, moonStateRef } from "./cloudTextureRef";

type Tier = "high" | "med" | "low";

function detectTier(): Tier {
  try {
    const canvas = document.createElement("canvas");
    const gl = canvas.getContext("webgl2") || canvas.getContext("webgl");
    if (!gl) return "low";
    const maxTex = gl.getParameter(gl.MAX_TEXTURE_SIZE) as number;
    const cores  = navigator.hardwareConcurrency ?? 2;
    const isWebGL2 = "drawBuffers" in gl;
    if (isWebGL2 && maxTex >= 8192 && cores >= 4) return "high";
    if (maxTex >= 4096) return "med";
    return "low";
  } catch {
    return "low";
  }
}

// Respect prefers-reduced-motion
const prefersReduced = typeof window !== "undefined" &&
  window.matchMedia("(prefers-reduced-motion: reduce)").matches;

// ─── Palette targets ───────────────────────────────────────────────────────
type RGB = [number, number, number];

// Day palette: clear blue sky
const DAY_SKY_COLOR:    RGB = [0.78, 0.86, 0.95];
const DAY_CLOUD_SHADOW: RGB = [0.18, 0.24, 0.36];
const DAY_CLOUD_COLOR:  RGB = [0.84, 0.87, 0.93];
const DAY_SUNLIGHT:     RGB = [1.00, 0.95, 0.85];
const DAY_SUN_GLARE:    RGB = [1.00, 0.78, 0.55];

// Golden hour palette: warm orange/pink
const GH_SKY_COLOR:    RGB = [0.85, 0.55, 0.55];
const GH_CLOUD_SHADOW: RGB = [0.30, 0.15, 0.30];
const GH_CLOUD_COLOR:  RGB = [0.92, 0.78, 0.68]; // warm tinted clouds
const GH_SUNLIGHT:     RGB = [1.00, 0.65, 0.45];
const GH_SUN_GLARE:    RGB = [1.00, 0.55, 0.25];

// Night palette: deep blue/dark
const NIGHT_SKY_COLOR:    RGB = [0.04, 0.07, 0.14];
const NIGHT_CLOUD_SHADOW: RGB = [0.10, 0.14, 0.22];
const NIGHT_CLOUD_COLOR:  RGB = [0.55, 0.60, 0.72];
const NIGHT_SUNLIGHT:     RGB = [0.75, 0.80, 0.95]; // moonlight
const NIGHT_SUN_GLARE:    RGB = [0.00, 0.00, 0.00]; // off at night

function lerp3(a: RGB, b: RGB, t: number): RGB {
  return [
    a[0] + (b[0] - a[0]) * t,
    a[1] + (b[1] - a[1]) * t,
    a[2] + (b[2] - a[2]) * t,
  ];
}

function smoothstep(edge0: number, edge1: number, x: number): number {
  const t = Math.max(0, Math.min(1, (x - edge0) / (edge1 - edge0)));
  return t * t * (3 - 2 * t);
}

// 3-tier cloud cover remap: piecewise-linear S-curve that pushes apart
// the visual signature of "a bit / a lot / really a lot" of clouds, while
// keeping smooth transitions at the boundaries (no popping).
//
//   raw 0.00..0.20  →  visual 0.00..0.10  (a few wisps)
//   raw 0.20..0.55  →  visual 0.10..0.55  (visibly cloudy, distinct puffs)
//   raw 0.55..1.00  →  visual 0.55..0.88  (heavy / overcast)
function tieredCover(raw: number): number {
  const c = Math.max(0, Math.min(1, raw));
  if (c < 0.20) return c * 0.50;
  if (c < 0.55) return 0.10 + (c - 0.20) * (0.45 / 0.35);
  return 0.55 + (c - 0.55) * (0.33 / 0.45);
}

// Density follows the same shape so heavy weather actually reads heavy
// (denser cores), not just "more clouds".
function tieredDensity(raw: number): number {
  const d = Math.max(0, Math.min(1, raw));
  if (d < 0.25) return d * 0.55;
  if (d < 0.60) return 0.14 + (d - 0.25) * (0.40 / 0.35);
  return 0.54 + (d - 0.60) * (0.30 / 0.40);
}

export function Clouds() {
  const params   = useSceneParams();
  const { size, gl } = useThree();
  const tierRef  = useRef<Tier>(detectTier());
  const tier     = tierRef.current;
  const timeRef  = useRef(0);

  // Half-resolution FBO for cloud rendering
  const fboW = Math.max(1, Math.floor(size.width  / 2));
  const fboH = Math.max(1, Math.floor(size.height / 2));
  const fbo  = useFBO(fboW, fboH, {
    minFilter: THREE.LinearFilter,
    magFilter: THREE.LinearFilter,
    format: THREE.RGBAFormat,
    type: THREE.UnsignedByteType,
    depthBuffer: false,
  });

  // Dedicated off-screen scene + camera for the cloud FBO pass
  const rmScene  = useMemo(() => new THREE.Scene(), []);
  const rmCamera = useMemo(() => new THREE.OrthographicCamera(-1, 1, 1, -1, 0, 1), []);

  // Apple-Weather-leaning palette; runtime params nudge sun warmth.
  const cloudUniforms = useMemo(() => ({
    iTime:        { value: 0 },
    iResolution:  { value: new THREE.Vector2(fboW, fboH) },
    uSkyColor:    { value: new THREE.Color(...DAY_SKY_COLOR) },
    uCloudColor:  { value: new THREE.Color(...DAY_CLOUD_COLOR) },
    uCloudShadow: { value: new THREE.Color(...DAY_CLOUD_SHADOW) },
    uSunColor:    { value: new THREE.Color(...params.palette.sun) },
    uSunGlare:    { value: new THREE.Color(...DAY_SUN_GLARE) },
    uSunlight:    { value: new THREE.Color(...DAY_SUNLIGHT) },
    uSunDir:      { value: new THREE.Vector3(-1, 0.05, -1).normalize() },
    uSpeed:       { value: params.cloudSpeed },
    uCover:       { value: params.cloudCover },
    uDensity:     { value: params.cloudDensity },
    uPrecip:      { value: 0 },
    uMoonNDC:     { value: new THREE.Vector2(0, 0) },
    uMoonI:       { value: 0 },
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }), []);
  void tier; // tier still gates whether the FBO pass runs

  // Upscale pass — separate scene + camera, rendered directly to canvas AFTER
  // EffectComposer so the HDR bloom never captures cloud pixels.
  const upscaleScene  = useMemo(() => new THREE.Scene(), []);
  const upscaleCamera = useMemo(() => new THREE.OrthographicCamera(-1, 1, 1, -1, 0, 1), []);

  const upscaleUniforms = useMemo(() => ({
    uCloud:      { value: fbo.texture },
    uTexel:      { value: new THREE.Vector2(1 / fboW, 1 / fboH) },
    uSkyZenith:  { value: new THREE.Color(...params.palette.zenith) },
    uSkyHorizon: { value: new THREE.Color(...params.palette.horizon) },
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }), [fbo, fboW, fboH]);

  // Build cloud FBO quad.
  // NoBlending: the shader outputs straight RGBA directly into the FBO without
  // GL alpha-compositing (which would premultiply RGB by alpha).
  useEffect(() => {
    const geo = new THREE.PlaneGeometry(2, 2);
    const mat = new THREE.ShaderMaterial({
      uniforms: cloudUniforms,
      vertexShader: CLOUDS_VERT,
      fragmentShader: CLOUDS_FRAG,
      depthWrite: false,
      blending: THREE.NoBlending,
    });
    const mesh = new THREE.Mesh(geo, mat);
    rmScene.add(mesh);
    // Expose the cloud FBO texture so CelestialBody can sample it for
    // per-pixel sun occlusion.
    cloudTexRef.current = fbo.texture;
    return () => {
      rmScene.remove(mesh); geo.dispose(); mat.dispose();
      cloudTexRef.current = null;
    };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [rmScene]);

  // Build upscale quad in its own scene. NormalBlending so the straight-alpha
  // cloud RGBA composites over whatever is already in the canvas (host page
  // background visible through transparent canvas).
  useEffect(() => {
    const geo = new THREE.PlaneGeometry(2, 2);
    const mat = new THREE.ShaderMaterial({
      uniforms: upscaleUniforms,
      vertexShader: CLOUD_UPSCALE_VERT,
      fragmentShader: CLOUD_UPSCALE_FRAG,
      depthWrite: false,
      transparent: true,
      blending: THREE.NormalBlending,
    });
    const mesh = new THREE.Mesh(geo, mat);
    upscaleScene.add(mesh);
    return () => { upscaleScene.remove(mesh); geo.dispose(); mat.dispose(); };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [upscaleScene]);

  // Pass 1 (priority 1): update uniforms + render FBM into cloud FBO
  useFrameThrottle((_, dt) => {
    // Always update sky uniforms so the upscale pass has current palette.
    upscaleUniforms.uSkyZenith.value.setRGB(...params.palette.zenith);
    upscaleUniforms.uSkyHorizon.value.setRGB(...params.palette.horizon);

    // ── Palette interpolation: day / golden / night ──────────────────────
    const alt = params.sunAltitude;

    // goldenHour factor: 1 at horizon (|alt|=0), 0 at |alt|=0.20
    const goldenHour = 1.0 - smoothstep(0.0, 0.20, Math.abs(alt));

    // nightFactor: 1 when alt < -0.15, 0 when alt > -0.05
    const nightFactor = smoothstep(-0.05, -0.15, alt);

    // Compute target for each uniform: day → mix with golden → mix with night
    const tSkyColor    = lerp3(lerp3(DAY_SKY_COLOR,    GH_SKY_COLOR,    goldenHour), NIGHT_SKY_COLOR,    nightFactor);
    const tCloudShadow = lerp3(lerp3(DAY_CLOUD_SHADOW, GH_CLOUD_SHADOW, goldenHour), NIGHT_CLOUD_SHADOW, nightFactor);
    const tCloudColor  = lerp3(lerp3(DAY_CLOUD_COLOR,  GH_CLOUD_COLOR,  goldenHour), NIGHT_CLOUD_COLOR,  nightFactor);
    const tSunlight    = lerp3(lerp3(DAY_SUNLIGHT,     GH_SUNLIGHT,     goldenHour), NIGHT_SUNLIGHT,     nightFactor);
    const tSunGlare    = lerp3(lerp3(DAY_SUN_GLARE,    GH_SUN_GLARE,    goldenHour), NIGHT_SUN_GLARE,    nightFactor);

    const lerpK = 0.04;
    const cu = cloudUniforms;
    const lerpColor = (u: THREE.Color, target: RGB) => {
      u.r += (target[0] - u.r) * lerpK;
      u.g += (target[1] - u.g) * lerpK;
      u.b += (target[2] - u.b) * lerpK;
    };
    lerpColor(cu.uSkyColor.value,    tSkyColor);
    lerpColor(cu.uCloudShadow.value, tCloudShadow);
    lerpColor(cu.uCloudColor.value,  tCloudColor);
    lerpColor(cu.uSunlight.value,    tSunlight);
    lerpColor(cu.uSunGlare.value,    tSunGlare);

    if (tier === "low") return;

    timeRef.current += dt * (prefersReduced ? 0.25 : 1);
    cloudUniforms.iTime.value     = timeRef.current;
    cloudUniforms.iResolution.value.set(fboW, fboH);
    // 4 visual tiers from the API value:
    //   0.00..0.05 → "pas du tout" → no clouds at all (FBO cleared, no render)
    //   0.05..0.20 → "un peu"      → light scattered wisps
    //   0.20..0.55 → "beaucoup"    → clearly populated cumulus sky
    //   0.55..1.00 → "vraiment beaucoup" → heavy / near-overcast
    const visualCover   = tieredCover(params.cloudCover);
    const visualDensity = tieredDensity(params.cloudDensity);
    cloudUniforms.uCover.value   += (visualCover   - cloudUniforms.uCover.value)   * 0.05;
    cloudUniforms.uDensity.value += (visualDensity - cloudUniforms.uDensity.value) * 0.05;

    // Rain darkens the dense cloud cores. Snow and other types leave it.
    const rainTarget = params.precipitation === "rain" ? params.precipIntensity : 0;
    cloudUniforms.uPrecip.value += (rainTarget - cloudUniforms.uPrecip.value) * 0.05;

    // Moonlight halo on cloud pixels — only when the moon is up.
    cloudUniforms.uMoonNDC.value.set(moonStateRef.screenX, moonStateRef.screenY);
    cloudUniforms.uMoonI.value += (moonStateRef.intensity - cloudUniforms.uMoonI.value) * 0.05;
    cloudUniforms.uSpeed.value    = params.cloudSpeed;
    cloudUniforms.uSunColor.value.setRGB(...params.palette.sun);
    cloudUniforms.uSunDir.value.set(
      Math.cos(params.sunAltitude) * Math.sin(params.sunAzimuth),
      Math.sin(params.sunAltitude),
      -Math.cos(params.sunAltitude) * Math.cos(params.sunAzimuth)
    ).normalize();

    const prevTarget    = gl.getRenderTarget();
    const prevClearClr  = gl.getClearColor(new THREE.Color());
    const prevClearAlph = gl.getClearAlpha();
    gl.setRenderTarget(fbo);
    gl.setClearColor(0x000000, 0);
    gl.clear();
    // "Pas du tout": near-zero smoothed cover → just clear the FBO so the
    // upscale composites pure transparency (host page shows through, no
    // residual ghost cloud from the previous frame). The cleared FBO is
    // also what we want during the lerp-down from cloudy → clear.
    if (cloudUniforms.uCover.value > 0.005) {
      gl.render(rmScene, rmCamera);
    }
    gl.setClearColor(prevClearClr, prevClearAlph);
    gl.setRenderTarget(prevTarget);
  }, 1);

  // Pass 2 (priority 2): blit sky+clouds onto the canvas AFTER bloom post-processing.
  // Always runs (even with no clouds) to apply the sky gradient that replaces the
  // EffectComposer's opaque output with our transparent-capable composite.
  // autoClear=false preserves the bloom-composited frame for CelestialBody overlays.
  useFrame((state) => {
    const renderer = state.gl;
    const prevAutoClear = renderer.autoClear;
    renderer.autoClear = false;
    renderer.setRenderTarget(null);
    renderer.render(upscaleScene, upscaleCamera);
    renderer.autoClear = prevAutoClear;
  }, 2);

  return null;
}
