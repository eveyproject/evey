// Volumetric clouds — port of vanta.js CLOUDS effect (MIT, Copyright 2020
// Teng Bao), itself a near-verbatim adaptation of Inigo Quilez "Clouds"
// Shadertoy XslGRr (https://www.shadertoy.com/view/XslGRr).
//
// 4-level-of-detail raymarcher: 115 steps total split across map5/map4/map3/map2.
// Front-to-back compositing with self-shadowing approximation via density
// gradient toward sun. Rendered into a half-res FBO and upscaled with a
// 5-tap tent filter. Output is straight RGBA; alpha = cloud accumulation
// only — the host page background shows through where there are no clouds.

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

uniform vec2  iResolution;
uniform float iTime;

uniform vec3  uSkyColor;
uniform vec3  uCloudColor;
uniform vec3  uCloudShadow;
uniform vec3  uSunColor;
uniform vec3  uSunGlare;
uniform vec3  uSunlight;

uniform vec3  uSunDir;
uniform float uSpeed;
uniform float uCover;       // 0..1
uniform float uDensity;     // 0..1
uniform float uPrecip;      // 0..1 rain intensity → darkens dense cloud cores
uniform vec2  uMoonNDC;     // moon center in NDC (-1..1)
uniform float uMoonI;       // 0..1 — 0 hides the halo entirely

varying vec2 vUv;

float hash(float p) {
  p = fract(p * 0.011);
  p *= (p + 7.5);
  p *= (p + p);
  return fract(p);
}

float noise(vec3 x) {
  vec3 p = floor(x);
  vec3 f = fract(x);
  f = f * f * (3.0 - 2.0 * f);
  float n = p.x + p.y * 57.0 + 113.0 * p.z;
  return mix(
    mix(mix(hash(n +   0.0), hash(n +   1.0), f.x),
        mix(hash(n +  57.0), hash(n +  58.0), f.x), f.y),
    mix(mix(hash(n + 113.0), hash(n + 114.0), f.x),
        mix(hash(n + 170.0), hash(n + 171.0), f.x), f.y),
    f.z);
}

const float K_TIME = 1000.0;

// Vanta / IQ slab + a low-frequency presence mask that punches volumetric
// holes in the cloud field, breaking the horizon line into distinct puffs.
float shape(float f, vec3 p) {
  float raw  = clamp(1.5 - p.y - 2.0 + 1.75 * f, 0.0, 1.0);
  float bias = mix(0.45, 0.18, uCover);
  float d    = clamp((raw - bias) * 2.4, 0.0, 1.0);

  // Large-scale presence mask — sinusoidal in x+z so cloud cells alternate
  // with empty sky. uCover raises the presence floor so overcast isn't full
  // of holes.
  float presence = 0.5 + 0.5 * sin(p.x * 0.6 + 1.7) * cos(p.z * 0.4 - 0.9);
  float gate     = smoothstep(0.30, 0.65, presence + uCover * 0.55);
  return d * gate * mix(0.30, 0.85, uCover) * uDensity;
}

float map5(in vec3 p) {
  vec3 drift = vec3(0.5, 0.01, 1.0) * 0.5 * uSpeed;
  vec3 q = p - drift * (iTime + K_TIME);
  float f;
  f  = 0.50000 * noise(q); q *= 2.02;
  f += 0.25000 * noise(q); q *= 2.03;
  f += 0.12500 * noise(q); q *= 2.01;
  f += 0.06250 * noise(q); q *= 2.02;
  f += 0.03125 * noise(q);
  return shape(f, p);
}
float map4(in vec3 p) {
  vec3 drift = vec3(0.5, 0.01, 1.0) * 0.5 * uSpeed;
  vec3 q = p - drift * (iTime + K_TIME);
  float f;
  f  = 0.50000 * noise(q); q *= 2.02;
  f += 0.25000 * noise(q); q *= 2.03;
  f += 0.12500 * noise(q); q *= 2.01;
  f += 0.06250 * noise(q);
  return shape(f, p);
}
float map3(in vec3 p) {
  vec3 drift = vec3(0.5, 0.01, 1.0) * 0.5 * uSpeed;
  vec3 q = p - drift * (iTime + K_TIME);
  float f;
  f  = 0.50000 * noise(q); q *= 2.02;
  f += 0.25000 * noise(q); q *= 2.03;
  f += 0.12500 * noise(q);
  return shape(f, p);
}
float map2(in vec3 p) {
  vec3 drift = vec3(0.5, 0.01, 1.0) * 0.5 * uSpeed;
  vec3 q = p - drift * (iTime + K_TIME);
  float f;
  f  = 0.50000 * noise(q); q *= 2.02;
  f += 0.25000 * noise(q);
  return shape(f, p);
}

vec4 integrate(in vec4 sum, in float dif, in float den, in vec3 bgcol, in float t) {
  vec3 lin = uCloudColor * 1.4 + uSunlight * dif;
  vec4 col = vec4(mix(vec3(1.0, 0.95, 0.85), uCloudShadow, den), den);
  col.xyz *= lin;
  col.xyz = mix(col.xyz, bgcol, 1.0 - exp(-0.003 * t * t));
  col.a *= 0.4;
  col.rgb *= col.a;
  return sum + col * (1.0 - sum.a);
}

#define MARCH(STEPS, MAPLOD) \
  for (int i = 0; i < STEPS; i++) { \
    vec3 pos = ro + t * rd; \
    if (pos.y < -3.0 || pos.y > 2.0 || sum.a > 0.99) break; \
    float den = MAPLOD(pos); \
    if (den > 0.01) { \
      float dif = clamp((den - MAPLOD(pos + 0.3 * uSunDir)) / 0.6, 0.0, 1.0); \
      sum = integrate(sum, dif, den, bgcol, t); \
    } \
    t += max(0.075, 0.02 * t); \
  }

vec4 raymarch(in vec3 ro, in vec3 rd, in vec3 bgcol) {
  vec4 sum = vec4(0.0);
  float t = 0.0;
  MARCH(20, map5)
  MARCH(25, map4)
  MARCH(30, map3)
  MARCH(40, map2)
  return clamp(sum, 0.0, 1.0);
}

mat3 setCamera(in vec3 ro, in vec3 ta, float cr) {
  vec3 cw = normalize(ta - ro);
  vec3 cp = vec3(sin(cr), cos(cr), 0.0);
  vec3 cu = normalize(cross(cw, cp));
  vec3 cv = normalize(cross(cu, cw));
  return mat3(cu, cv, cw);
}

void main() {
  // Aspect-correct screen coords centered: p = (-1..1, ~-asp..asp).
  vec2 p = (vUv * 2.0 - 1.0);
  p.x *= iResolution.x / iResolution.y;

  // Slow horizontal sway so the scene feels alive.
  float swayX = sin(iTime * 0.05 + 3.1415) * 0.25 + 0.25;
  vec2 m = vec2(swayX * 0.25, 0.85);

  // Camera low + target high → ray climbs into the cloud slab so puffs
  // sit in the upper portion of the widget rather than along the horizon.
  vec3 ro = vec3(4.0 * sin(3.0 * m.x), 0.10, 4.0 * cos(3.0 * m.x));
  vec3 ta = vec3(0.0, 2.2, 0.0);
  mat3 ca = setCamera(ro, ta, 0.0);
  vec3 rd = ca * normalize(vec3(p, 1.5));

  // Sky / sun glow used as the bg color the cloud march fades toward.
  float sun = clamp(dot(uSunDir, rd), 0.0, 1.0);
  vec3 bgcol = uSkyColor - rd.y * 0.2 * vec3(1.0, 0.5, 1.0) + 0.075;
  bgcol += 0.2 * uSunColor * pow(sun, 8.0);

  vec4 res = raymarch(ro, rd, bgcol);

  // Final color = clouds composited over sky tint, plus sun glare fringe.
  vec3 col = bgcol * (1.0 - res.w) + res.xyz;
  col += 0.2 * uSunGlare * pow(sun, 3.0);

  // Rain — darken the dense cloud cores (storm bellies). Wide smoothstep
  // range plus a tiny dither breaks the visible band the half-res FBO
  // would otherwise show along the mask threshold.
  float coreMask = smoothstep(0.15, 1.05, res.w);
  float dither   = (fract(sin(dot(gl_FragCoord.xy, vec2(12.9898, 78.233))) * 43758.5453) - 0.5) * 0.04;
  coreMask = clamp(coreMask + dither, 0.0, 1.0);
  vec3 stormCol  = vec3(0.18, 0.20, 0.26);
  col = mix(col, stormCol, coreMask * uPrecip * 0.85);

  // Moonlight halo: cloud puffs in front of the moon catch a soft cool
  // glow. screenNDC distance from the moon center, modulated by cloud
  // density so it only lights actual cloud pixels (not the empty sky).
  vec2 ndc = vUv * 2.0 - 1.0;
  float moonDist = length(ndc - uMoonNDC);
  float moonHalo = exp(-moonDist * moonDist * 5.0) * uMoonI;
  vec3 moonGlow  = vec3(0.78, 0.85, 1.00);  // cool blue-white
  // The halo only paints clouds (res.w), preserving transparent sky.
  col += moonGlow * moonHalo * res.w * 0.65;

  // Output: alpha tied to cloud accumulation so the host page background
  // shows through where there are no clouds. Clamp top to 0.92 to keep
  // a hint of transparency even on dense cloud cores.
  // Cap below 0.85 so even thick cloud cores let a whisper of background
  // through — looks like real cumulus, not painted-on fog.
  float alpha = clamp(res.w * 1.05, 0.0, 0.82);
  gl_FragColor = vec4(col, alpha);
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
varying vec2      vUv;

void main() {
  // Shift cloud band upward so puffs sit higher in the widget.
  vec2 sUv = vec2(vUv.x, vUv.y - 0.32);
  // 5-tap tent filter to soften the half-res FBO upscale.
  vec4 c = texture2D(uCloud, sUv)                         * 0.36
         + texture2D(uCloud, sUv + vec2( uTexel.x, 0.0)) * 0.16
         + texture2D(uCloud, sUv + vec2(-uTexel.x, 0.0)) * 0.16
         + texture2D(uCloud, sUv + vec2(0.0,  uTexel.y)) * 0.16
         + texture2D(uCloud, sUv + vec2(0.0, -uTexel.y)) * 0.16;
  gl_FragColor = c;
}
`;
