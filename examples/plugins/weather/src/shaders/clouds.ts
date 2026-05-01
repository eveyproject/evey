// Layered 2.5D cloud shader — Apple Weather style.
// Four FBM layers composited front-to-back; vertical gradient lighting
// (screen-space Y) gives lit tops and shadowed bases without noisy derivatives.
// Rendered off-screen into a half-res FBO (NoBlending → straight RGBA).
// Upscaled to canvas at priority=2, after EffectComposer — bloom never touches clouds.
//
// References:
//   - Inigo Quilez "fbm" — https://iquilezles.org/articles/fbm/
//   - Shadertoy "Drifting Clouds 2D" guil (4tdSWr)

export const CLOUDS_VERT = /* glsl */ `
precision highp float;
varying vec2 vUv;
void main() {
  vUv = uv;
  gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
}
`;

export const CLOUDS_FRAG = /* glsl */ `
precision highp float;

uniform float uTime;
uniform float uCover;
uniform float uDensity;
uniform float uSpeed;
uniform vec2  uWind;
uniform vec3  uSunDir;
uniform vec3  uSunColor;
uniform vec3  uAmbient;
uniform int   uSteps;

varying vec2 vUv;

// ---------------------------------------------------------------------------
// Value noise + FBM
// ---------------------------------------------------------------------------
float hash21(vec2 p) {
  p = fract(p * vec2(127.1, 311.7));
  return fract(sin(dot(p, vec2(12.9898, 78.233))) * 43758.5453);
}

float vnoise(vec2 p) {
  vec2 i = floor(p);
  vec2 f = fract(p);
  vec2 u = f * f * (3.0 - 2.0 * f);
  return mix(
    mix(hash21(i),                 hash21(i + vec2(1.0, 0.0)), u.x),
    mix(hash21(i + vec2(0.0, 1.0)), hash21(i + vec2(1.0, 1.0)), u.x),
    u.y
  );
}

float fbm(vec2 p) {
  float v = 0.0, a = 0.5;
  float c = 0.766, s = 0.643; // 40° rotation
  mat2 r = mat2(c, s, -s, c);
  for (int i = 0; i < 5; i++) {
    v += a * vnoise(p);
    p = r * p * 2.01;
    a *= 0.5;
  }
  return clamp((v - 0.03) / 0.94, 0.0, 1.0);
}

// ---------------------------------------------------------------------------
// Cloud sample: returns alpha only.
// Colour is determined OUTSIDE by the vertical gradient.
// ---------------------------------------------------------------------------
float cloudAlpha(vec2 p, float thr, float soft) {
  vec2 q = vec2(fbm(p),
                fbm(p + vec2(3.7, 1.9)));
  float n = fbm(p + q * 0.55);
  return smoothstep(thr - soft, thr + soft, n);
}

void main() {
  vec2 uv = vUv;
  uv.x *= 3.20; // slightly less stretch = larger, rounder clouds

  vec2 wd = normalize(uWind + vec2(0.0001)) * uSpeed * 0.018;
  vec2 dt = wd * uTime;

  // Coverage threshold — lower = more clouds
  float thr  = mix(0.68, 0.28, uCover);
  // Edge softness — 0.12 gives clean but not toon-like edges
  float soft = 0.12;

  // Layer 0: cirrus (thin, high, sparse — softer edges)
  float A0 = cloudAlpha(uv * 0.90 + dt * 0.35 + vec2( 0.0,  0.0), thr + 0.14, soft * 1.40);
  // Layer 1: large cumulus (main mass, most visible)
  float A1 = cloudAlpha(uv * 1.60 + dt * 0.85 + vec2(11.2,  4.8), thr,        soft);
  // Layer 2: mid cumulus
  float A2 = cloudAlpha(uv * 2.60 + dt * 1.30 + vec2(23.4, 10.1), thr - 0.06, soft * 0.90);
  // Layer 3: small puffs (detail/texture)
  float A3 = cloudAlpha(uv * 4.20 + dt * 1.85 + vec2(37.1, 17.8), thr - 0.10, soft * 0.80);

  // Per-layer max alpha — density=1 gives near-opaque, density=0.4 gives ~55% cloud opacity
  float d  = uDensity;
  float a0 = A0 * 0.40 * d;
  float a1 = A1 * 0.88 * d;  // main layer: drives most of the opacity
  float a2 = A2 * 0.75 * d;
  float a3 = A3 * 0.60 * d;

  // Vertical fade: clouds in upper ~60% of frame.
  // vUv.y=0 = bottom of screen, y=1 = top.
  // Soft ramp from y=0.30..0.55 so cloud bases have a natural curved underbelly.
  float vert = smoothstep(0.28, 0.55, vUv.y) * smoothstep(1.00, 0.97, vUv.y);
  a0 *= vert; a1 *= vert; a2 *= vert; a3 *= vert;

  // Vertical gradient: cloud base (y≈0.35) is shadowed, top (y≈1.0) is lit.
  // litGrad=0 → shadow underside; litGrad=1 → lit top.
  float litGrad = smoothstep(0.38, 0.80, vUv.y);

  // Apple Weather colour palette:
  //   litTop ≈ (0.95, 0.95, 0.96) — near-white sun-lit top
  //   shadow ≈ (0.72, 0.74, 0.80) — cool blue-grey underside
  // Both are significantly lighter than any sky blue → clouds pop clearly.
  vec3 litTop = vec3(0.95, 0.95, 0.96);
  vec3 shadow = vec3(0.72, 0.74, 0.80);

  // Warm lit top slightly with sun, cool shadow slightly with zenith
  litTop = clamp(mix(litTop, uSunColor * 0.72 + vec3(0.20), 0.14), 0.80, 0.97);
  shadow = mix(shadow, uAmbient * 0.28 + vec3(0.50), 0.18);

  // Per-layer colours — litGrad blends shadow underside → lit top
  vec3 c0 = mix(shadow * 0.97, litTop * 0.92, litGrad * 0.78 + 0.18); // cirrus: mostly lit
  vec3 c1 = mix(shadow,        litTop,         litGrad * 0.85 + 0.10); // main: full range
  vec3 c2 = mix(shadow,        litTop * 0.98,  litGrad * 0.82 + 0.12);
  vec3 c3 = mix(shadow * 0.94, litTop * 0.90,  litGrad * 0.80 + 0.14); // small puffs

  // Front-to-back composite (L3 nearest viewer)
  vec3  acc   = vec3(0.0);
  float trans = 1.0;
  acc += c3 * a3;           trans *= (1.0 - a3);
  acc += c2 * a2 * trans;   trans *= (1.0 - a2);
  acc += c1 * a1 * trans;   trans *= (1.0 - a1);
  acc += c0 * a0 * trans;   trans *= (1.0 - a0);

  float totalAlpha = clamp(1.0 - trans, 0.0, 1.0);

  // Soft underbelly fade — natural curved cloud base, not a straight horizontal cut
  float edge = smoothstep(0.26, 0.45, vUv.y);
  totalAlpha *= edge;

  // Straight-alpha output (NoBlending on this material — FBO stores gl_FragColor directly)
  vec3 finalColor = totalAlpha > 0.001 ? (acc / totalAlpha) : shadow;
  gl_FragColor = vec4(finalColor, clamp(totalAlpha, 0.0, 1.0));
}
`;

export const CLOUD_UPSCALE_VERT = /* glsl */ `
precision highp float;
varying vec2 vUv;
void main() {
  vUv = uv;
  gl_Position = vec4(position.xy, 0.0, 1.0);
}
`;

export const CLOUD_UPSCALE_FRAG = /* glsl */ `
precision highp float;
uniform sampler2D uCloud;
uniform vec2      uTexel;
uniform vec3      uSkyZenith;
uniform vec3      uSkyHorizon;
varying vec2      vUv;

void main() {
  // 5-tap tent filter upscale from half-res cloud FBO
  vec4 c = texture2D(uCloud, vUv)                          * 0.36
         + texture2D(uCloud, vUv + vec2( uTexel.x,  0.0)) * 0.16
         + texture2D(uCloud, vUv + vec2(-uTexel.x,  0.0)) * 0.16
         + texture2D(uCloud, vUv + vec2( 0.0,  uTexel.y)) * 0.16
         + texture2D(uCloud, vUv + vec2( 0.0, -uTexel.y)) * 0.16;


  // Sky gradient — this pass is fully opaque, replacing the EffectComposer output.
  float t  = smoothstep(0.0, 1.0, vUv.y);
  vec3 sky = mix(uSkyHorizon, uSkyZenith, t);

  // subtle dither to break gradient banding
  float h = fract(sin(dot(vUv * 512.0, vec2(127.1, 311.7))) * 43758.5453);
  sky += (h - 0.5) * 0.010;

  // c.rgb is straight (NoBlending on FBO material stores gl_FragColor directly).
  // Blend clouds over sky using straight alpha.
  float cloudAlpha = clamp(c.a, 0.0, 0.88);
  vec3  cloudRgb   = c.rgb;

  // Blend clouds (straight alpha) over sky
  vec3  final = mix(sky, cloudRgb, cloudAlpha);

  gl_FragColor = vec4(final, 1.0);
}
`;
