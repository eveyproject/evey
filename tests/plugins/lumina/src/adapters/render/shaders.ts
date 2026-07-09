// Two minimal WebGL1 programs, one context, no framebuffers:
//
//   VEIL_* - a single fullscreen triangle painting the whole scene: a deep
//   background wash, three layers of domain-warped FBM "god-ray" veils that
//   drift and cross each other, an optional liquid-caustic pattern (quality
//   gated), a lazy pointer-reactive glow, a warm breathing vignette and a
//   time-varying film-grain dither. Every color comes from the palette the
//   CPU computes once per frame (domain/services/paletteDrift.ts) and hands
//   in as five vec3 uniforms - the shader itself never invents color, only
//   shapes and motion.
//
//   PARTICLE_* - the floating dust/bokeh specks. Positions are computed
//   entirely in the vertex shader from a per-particle seed (base position,
//   depth, phase, speed - see domain/services/particleField.ts) plus
//   uTime/uPointer, so the CPU never re-uploads particle data after the
//   initial (one-time) buffer upload at mount.

export const VERT_SRC = `
attribute vec2 aPos;
varying vec2 vUv;
void main() {
  vUv = aPos * 0.5 + 0.5;
  gl_Position = vec4(aPos, 0.0, 1.0);
}
`;

export const VEIL_FRAG_SRC = `
#ifdef GL_FRAGMENT_PRECISION_HIGH
precision highp float;
#else
precision mediump float;
#endif

uniform vec2  uRes;
uniform float uTime;
uniform float uDrift;
uniform float uCausticPhase;
uniform float uVignettePulse;
uniform vec2  uPointer;      // eased, -1..1
uniform float uIntensity;    // 0..1 user setting

uniform vec3 uVeilA;
uniform vec3 uVeilB;
uniform vec3 uVeilC;
uniform vec3 uCaustic;
uniform vec3 uVignette;

uniform float uExtraLayer; // 0/1, quality-gated third veil
uniform float uCaustics;   // 0/1, quality-gated liquid-light pass

varying vec2 vUv;

float hash21(vec2 p) {
  p = fract(p * vec2(123.34, 345.45));
  p += dot(p, p + 34.345);
  return fract(p.x * p.y);
}

float vnoise(vec2 p) {
  vec2 i = floor(p);
  vec2 f = fract(p);
  f = f * f * (3.0 - 2.0 * f);
  float a = hash21(i);
  float b = hash21(i + vec2(1.0, 0.0));
  float c = hash21(i + vec2(0.0, 1.0));
  float d = hash21(i + vec2(1.0, 1.0));
  return mix(mix(a, b, f.x), mix(c, d, f.x), f.y);
}

float fbm(vec2 p) {
  float v = 0.0;
  float a = 0.5;
  mat2 m = mat2(1.6, 1.2, -1.2, 1.6);
  for (int i = 0; i < 4; i++) {
    v += a * vnoise(p);
    p = m * p;
    a *= 0.5;
  }
  return v;
}

// One drifting, domain-warped "curtain" of light: a soft FBM band stretched
// mostly vertically, warped by a second noise field so it ripples instead of
// scrolling as a rigid stripe.
float curtain(vec2 uv, float freqX, float freqY, float driftMul, float warpAmt, float seed) {
  vec2 warp = vec2(fbm(uv * 1.3 + seed), fbm(uv * 1.7 - seed)) * warpAmt;
  vec2 p = vec2(uv.x * freqX + uDrift * driftMul, uv.y * freqY - uTime * 0.02 + seed) + warp;
  float n = fbm(p);
  return smoothstep(0.30, 0.95, n);
}

void main() {
  float aspect = uRes.x / max(uRes.y, 1.0);
  vec2 uv = vUv;
  vec2 p = (uv - 0.5) * vec2(aspect, 1.0) * 2.0;

  // 1. Deep background wash: the darkest palette anchor, softly brighter
  // toward the center so the veils have somewhere to bloom from.
  vec3 col = uVeilC * (0.85 + 0.25 * exp(-dot(p, p) * 0.5));

  // 2. Veil A: the primary curtain.
  float rayA = curtain(uv, 2.2, 1.0, 0.045, 0.6, 1.7);
  col += uVeilA * rayA * 0.55 * uIntensity;

  // 3. Veil B: a second curtain at a different frequency/direction, phase-
  // shifted in the palette (see paletteDrift.ts) so it never carries the
  // same color as A - this is what reads as "crossing" rather than moving
  // together.
  vec2 uvB = vec2(uv.x * 0.82 + uv.y * 0.35, uv.y * 0.9 - uv.x * 0.20);
  float rayB = curtain(uvB, 1.6, 1.3, -0.07, 0.5, 9.3);
  col += uVeilB * rayB * 0.48 * uIntensity;

  // 4. Veil C (extra layer, quality-gated): a finer, slower third curtain
  // adding parallax depth on the higher tiers only.
  if (uExtraLayer > 0.5) {
    vec2 uvC = vec2(uv.x * 1.1 - uv.y * 0.5, uv.y * 2.4 + uv.x * 0.15);
    float rayC = curtain(uvC, 3.4, 1.8, 0.02, 0.35, 27.1);
    col += mix(uVeilA, uVeilB, 0.5) * rayC * 0.24 * uIntensity;
  }

  // 5. Caustics (quality-gated): a classic interference-of-sines liquid-
  // light pattern, additive, subtle - like the bottom of a lit pool.
  if (uCaustics > 0.5) {
    vec2 cp = uv * 5.0 + vec2(sin(uCausticPhase * 0.30), cos(uCausticPhase * 0.23));
    float c1 = abs(sin(cp.x * 3.0 + cos(cp.y * 2.0 + uCausticPhase * 0.5)));
    float c2 = abs(sin(cp.y * 3.3 - sin(cp.x * 2.4 - uCausticPhase * 0.4)));
    float caustic = pow(c1 * c2, 3.0);
    col += uCaustic * caustic * 0.35 * uIntensity;
  }

  // 6. Lazy pointer-reactive glow: a soft highlight that drifts toward the
  // (already-eased) pointer position, never snapping.
  vec2 ptr = uPointer * vec2(aspect, 1.0);
  float glow = exp(-dot(p - ptr, p - ptr) * 2.2) * 0.16 * uIntensity;
  col += mix(uVeilA, uVeilB, 0.5) * glow;

  // 7. Warm vignette, breathing slowly.
  float edge = length(p);
  float vigMask = smoothstep(0.55, 1.55, edge) * (0.78 + 0.14 * sin(uVignettePulse * 0.6));
  col = mix(col, col * 0.35 + uVignette * 0.30, vigMask);

  // 8. Film grain: cheap per-pixel dither, time-varying so it never reads as
  // a static texture (and keeps the deep gradients above from banding).
  float grain = hash21(gl_FragCoord.xy + uTime * 71.0) - 0.5;
  col += grain * 0.030;

  gl_FragColor = vec4(clamp(col, 0.0, 1.0), 1.0);
}
`;

export const PARTICLE_VERT_SRC = `
attribute vec2  aBase;
attribute float aDepth;
attribute float aSize;
attribute float aPhase;
attribute float aSpeed;

uniform float uTime;
uniform vec2  uPointer;
uniform float uIntensity;

varying float vDepth;
varying float vTwinkle;

void main() {
  float t = uTime * aSpeed + aPhase;
  // Gentle floating drift, larger for the far/small particles so the field
  // reads as a soft haze rather than a grid.
  vec2 drift = vec2(sin(t * 0.7), cos(t * 0.5)) * (0.05 + 0.05 * (1.0 - aDepth));
  // Parallax toward the (already lazily-eased) pointer, stronger for near
  // particles - a shallow depth-of-field cue with zero extra CPU work.
  vec2 parallax = uPointer * aDepth * 0.10;
  vec2 pos = aBase + drift + parallax;

  gl_Position = vec4(pos, 0.0, 1.0);
  gl_PointSize = aSize * (10.0 + 14.0 * aDepth) * (0.6 + 0.4 * uIntensity);

  vDepth = aDepth;
  vTwinkle = 0.5 + 0.5 * sin(t * 1.3 + aPhase * 2.0);
}
`;

export const PARTICLE_FRAG_SRC = `
#ifdef GL_FRAGMENT_PRECISION_HIGH
precision highp float;
#else
precision mediump float;
#endif

uniform vec3  uColor;
uniform float uIntensity;

varying float vDepth;
varying float vTwinkle;

void main() {
  vec2 c = gl_PointCoord * 2.0 - 1.0;
  float r = length(c);
  float bokeh = exp(-r * r * 3.2);
  float alpha = bokeh * (0.25 + 0.55 * vDepth) * (0.5 + 0.5 * vTwinkle) * uIntensity;
  gl_FragColor = vec4(uColor * bokeh, alpha);
}
`;
