// Single-pass weather shader. Everything (golden-hour wash, moon, two FBM
// cloud layers, fog, precipitation, sun) is composited front-to-back in ONE
// fragment shader on ONE fullscreen triangle:
//   - no framebuffers, no multi-scene passes, no per-frame CPU particle
//     uploads (v1 re-uploaded 3000 particle positions every frame)
//   - sun/moon occlusion by clouds is per-pixel IN the same pass, so it can
//     never desync from the cloud layout (v1 sampled a cloud FBO with a
//     hard-coded -0.32 screen offset, wrong at any other widget height)
// Output is straight (non-premultiplied) RGBA over a transparent canvas:
// the host page shows through wherever nothing is drawn.

export const VERT_SRC = `
attribute vec2 aPos;
varying vec2 vUv;
void main() {
  vUv = aPos * 0.5 + 0.5;
  gl_Position = vec4(aPos, 0.0, 1.0);
}
`;

export const FRAG_SRC = `
#ifdef GL_FRAGMENT_PRECISION_HIGH
precision highp float;
#else
precision mediump float;
#endif

uniform vec2  uRes;
uniform float uTime;

uniform float uCover;      // 0..1
uniform float uDensity;    // 0..1
uniform float uWindDrift;  // CPU-integrated cloud drift offset (already * speed)
uniform float uWind;       // -1..1 horizontal wind for precipitation slant
uniform float uRain;       // 0..1
uniform float uSnow;       // 0..1
uniform float uFog;        // 0..1
uniform float uFlash;      // 0..1 thunder envelope
uniform vec2  uFlashPos;   // widget uv, y up: where the flash bursts from

uniform vec2  uSunPos;     // widget uv, y up
uniform float uSunI;       // 0..1
uniform float uGolden;     // 0..1
uniform vec2  uMoonPos;
uniform float uMoonI;
uniform float uMoonPhase;  // 0..1
uniform sampler2D uMoonTex;
uniform sampler2D uSunTex;

uniform vec3 uCloudCol;
uniform vec3 uCloudShadow;
uniform vec3 uSunlight;
uniform vec3 uFogCol;
uniform vec3 uGlowCol;

uniform float uAurora;     // 0..1, real latitude/night/clear-sky gated
uniform vec2  uStarPos;    // widget uv, y up: current shooting-star head
uniform vec2  uStarDir;    // unit travel direction, y up
uniform float uStarLife;   // 0..1, 0 when no star is currently streaking

varying vec2 vUv;

// Sin-free hash: stable on mediump mobile GPUs.
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
  return v; // ~0..0.94
}

// Straight-alpha "src over dst".
vec4 ovr(vec4 dst, vec3 srcC, float srcA) {
  float a = clamp(srcA, 0.0, 1.0);
  float outA = a + dst.a * (1.0 - a);
  vec3 outC = srcC * a + dst.rgb * dst.a * (1.0 - a);
  return vec4(outC / max(outA, 1e-4), outA);
}

// One cloud layer: coverage alpha + shaded color, rim-lit toward the light.
void cloudLayer(
  vec2 uv, float aspect, float scale, float drift, float seed, vec2 lightDir,
  out vec3 col, out float alpha
) {
  vec2 p = vec2(uv.x * aspect + uWindDrift * drift, uv.y * 0.9 + seed) * scale;
  float raw = fbm(p);
  float thr = mix(0.68, 0.28, uCover);
  float coverage = smoothstep(thr, thr + 0.30, raw);

  // Rim light: density gradient toward the sun/moon.
  float rawLit = fbm(p + lightDir * 0.35);
  float rim = clamp((raw - rawLit) * 3.5, 0.0, 1.0);

  // Dense cores read as shadow.
  float core = smoothstep(thr + 0.10, thr + 0.55, raw);
  col = mix(uCloudCol, uCloudShadow, core * mix(0.35, 1.0, uDensity));
  col += uSunlight * rim * coverage * (0.35 + 0.40 * uGolden);

  // Thunder flash lights cloud bodies from within.
  col += vec3(0.85, 0.90, 1.10) * uFlash * core;

  // Vertical mask: puffs live in the upper band; overcast reaches lower.
  float bandLo = mix(0.38, 0.08, uCover);
  float vm = smoothstep(0.02, bandLo, uv.y);
  alpha = coverage * mix(0.40, 0.92, uDensity) * vm;
}

float rainLayer(vec2 uv, float aspect, float t, vec2 grid, float speed, float slant, float seed) {
  uv.x += (uv.y - 1.0) * slant;
  vec2 p = vec2(uv.x * aspect, uv.y) * grid;
  float colId = floor(p.x);
  float h = hash21(vec2(colId, seed));
  float yy = p.y + t * speed * (0.75 + 0.5 * h) + h * 37.0;
  float cell = floor(yy);
  float f = fract(yy);
  float gate = step(hash21(vec2(colId, cell)), uRain * 0.40 + 0.03);
  float xline = smoothstep(0.20, 0.02, abs(fract(p.x) - 0.5));
  float streak = smoothstep(0.10, 0.40, f) * smoothstep(1.0, 0.55, f);
  return gate * xline * streak;
}

float snowLayer(vec2 uv, float aspect, float t, float scale, float fall, float seed) {
  vec2 p = vec2(uv.x * aspect, uv.y) * scale;
  p.y += t * fall + seed * 19.0;
  p.x += uWind * t * 0.35 * scale * 0.1 + sin(t * 0.6 + seed) * 0.3;
  vec2 id = floor(p);
  vec2 f = fract(p);
  float h1 = hash21(id);
  float h2 = hash21(id + 71.3);
  vec2 c = vec2(0.25 + 0.5 * h1, 0.25 + 0.5 * h2);
  c.x += 0.12 * sin(t * (1.0 + h1) + h2 * 6.283);
  float d = length(f - c);
  float rad = 0.05 + 0.09 * h2;
  float gate = step(h1, uSnow * 0.70 + 0.10);
  return smoothstep(rad, rad * 0.35, d) * gate;
}

void main() {
  float aspect = uRes.x / max(uRes.y, 1.0);
  // Noise coordinates only. hash21 squares its input (p += dot(p, p + k)), so
  // past a certain magnitude fract() keeps too few mantissa bits and the field
  // breaks into flat tiles. A television banner runs at aspect ~9 and showed
  // exactly that (2026-09-20); a card is under 3, so this clamp never fires on
  // a phone or a browser. The celestial discs keep the TRUE aspect below or
  // they turn into ellipses.
  float noiseAspect = min(aspect, 4.5);
  vec2 uv = vUv; // y up

  vec4 acc = vec4(0.0);

  // 1. Golden-hour horizon wash (bottom of the widget, sunrise/sunset only).
  float glowA = uGolden * 0.20 * smoothstep(0.55, 0.0, uv.y);
  acc = ovr(acc, uGlowCol, glowA);

  // 1b. Aurora (real latitude + night + clear-sky gated, see domain/services/
  // aurora.ts): soft drifting bands in the upper sky, painted before the
  // clouds so they occlude it exactly like they occlude the moon below.
  if (uAurora > 0.004) {
    float bandY = smoothstep(0.75, 0.05, uv.y);
    float n1 = fbm(vec2(uv.x * 3.0 + uTime * 0.02, uv.y * 1.5));
    float n2 = fbm(vec2(uv.x * 5.0 - uTime * 0.015 + 4.0, uv.y * 2.0 + 1.7));
    float bands = n1 * 0.6 + n2 * 0.4;
    vec3 auroraCol = mix(vec3(0.20, 1.00, 0.55), vec3(0.55, 0.35, 1.00), n2);
    acc = ovr(acc, auroraCol, bandY * bands * uAurora * 0.55);
  }

  // 2. Moon (under the clouds).
  if (uMoonI > 0.004) {
    vec2 dm = vec2((uv.x - uMoonPos.x) * aspect, uv.y - uMoonPos.y);
    float rm = length(dm);
    float MR = 0.40; // matches the sun's visible disc (its texture has more margin)
    // Cool halo behind the disc, falloff scaled to the larger radius.
    acc = ovr(acc, vec3(0.78, 0.85, 1.0), exp(-rm * rm * 7.0) * 0.26 * uMoonI);
    float discAA = smoothstep(MR, MR * 0.965, rm);
    if (discAA > 0.001) {
      vec2 tuv = clamp(dm / MR * 0.5 + 0.5, 0.0, 1.0);
      vec3 mtex = texture2D(uMoonTex, tuv).rgb;
      // Lift the dark maria so they read grey, not black, on dark hosts.
      vec3 lifted = mix(vec3(0.42), vec3(1.0), pow(mtex, vec3(0.65)));
      // Phase terminator: offset shadow disc sweeping across the face.
      vec2 pn = dm / MR;
      float shadowX = uMoonPhase < 0.5 ? -4.0 * uMoonPhase : 4.0 - 4.0 * uMoonPhase;
      float shadowMask = 1.0 - smoothstep(0.92, 1.04, length(pn - vec2(shadowX, 0.0)));
      float lit = mix(0.12, 1.0, 1.0 - shadowMask);
      acc = ovr(acc, lifted * lit, discAA * uMoonI);
    }
  }

  // 2b. Occasional shooting star (see domain/services/shootingStar.ts for the
  // rare, deterministic trigger). A short fading tail behind a bright head,
  // drawn as a distance-to-segment so it costs one branch when absent.
  if (uStarLife > 0.004) {
    vec2 ds = vec2((uv.x - uStarPos.x) * aspect, uv.y - uStarPos.y);
    vec2 dir = normalize(vec2(uStarDir.x * aspect, uStarDir.y));
    float tailLen = 0.14;
    float t = clamp(dot(ds, dir) / tailLen, 0.0, 1.0);
    float distToTail = length(ds - dir * t * tailLen);
    float streak = smoothstep(0.006, 0.0, distToTail) * (1.0 - t);
    float head = exp(-dot(ds, ds) * 1400.0);
    acc = ovr(acc, vec3(1.0, 1.0, 0.96), (streak * 0.7 + head) * uStarLife);
  }

  // 3. Clouds: far layer (slow, big) then near layer (faster, smaller).
  vec2 lightPos = uSunI >= uMoonI ? uSunPos : uMoonPos;
  vec2 lightDir = lightPos - uv;
  lightDir = length(lightDir) > 1e-4 ? normalize(vec2(lightDir.x * aspect, lightDir.y)) : vec2(0.0, 1.0);

  float cloudsA = 0.0;
  if (uCover > 0.004) {
    vec3 cF; float aF;
    cloudLayer(uv, noiseAspect, 2.2, 1.0, 7.3, lightDir, cF, aF);
    vec3 cN; float aN;
    cloudLayer(uv, noiseAspect, 3.6, 1.8, 41.7, lightDir, cN, aN);
    aN *= 0.85;
    acc = ovr(acc, cF, aF);
    acc = ovr(acc, cN, aN);
    cloudsA = clamp(aF + aN * (1.0 - aF), 0.0, 1.0);
  }

  // 4. Thunder: a localized burst from the flash origin inside the storm
  // cloud (uFlashPos), plus a much fainter whole-scene wash so the rest of
  // the sky still reacts a little - the burst itself carries the "silhouette"
  // read (cloudLayer's own uFlash term already lights the cloud cores it sits
  // behind, from within).
  if (uFlash > 0.004) {
    vec2 df = vec2((uv.x - uFlashPos.x) * aspect, uv.y - uFlashPos.y);
    float burst = exp(-dot(df, df) * 6.0) * uFlash;
    acc = ovr(acc, vec3(0.90, 0.94, 1.05), burst * 0.55);
    acc = ovr(acc, vec3(0.85, 0.90, 1.05), uFlash * 0.08);
  }

  // 5. Fog: denser toward the bottom, with a soft drifting inner layer (two
  // noise octaves at different scale/speed) so it reads with some depth
  // instead of a flat wash - same fbm the clouds already pay for, no extra
  // uniforms or CPU-side allocation.
  float fogNear = fbm(vec2(uv.x * 1.6 + uWindDrift * 0.02, uv.y * 1.1 - uTime * 0.010));
  float fogFar = fbm(vec2(uv.x * 0.9 - uWindDrift * 0.008 + 5.0, uv.y * 0.7 - uTime * 0.004));
  float fogMix = mix(0.75, 1.15, fogNear * 0.6 + fogFar * 0.4);
  acc = ovr(acc, uFogCol, uFog * (0.28 + 0.38 * (1.0 - uv.y)) * fogMix);

  // 6. Precipitation.
  if (uRain > 0.004) {
    float slant = uWind * 0.25;
    float r = rainLayer(uv, noiseAspect, uTime, vec2(26.0, 3.2), 9.0, slant, 3.1)
            + rainLayer(uv, noiseAspect, uTime, vec2(42.0, 4.6), 13.0, slant * 1.2, 17.9) * 0.8
            + rainLayer(uv, noiseAspect, uTime, vec2(64.0, 6.5), 18.0, slant * 1.4, 29.3) * 0.55;
    vec3 rainCol = vec3(0.62, 0.78, 1.00);
    acc = ovr(acc, rainCol, clamp(r, 0.0, 1.0) * (0.20 + 0.22 * uRain));
  }
  if (uSnow > 0.004) {
    float s = snowLayer(uv, noiseAspect, uTime, 9.0, 0.7, 1.7)
            + snowLayer(uv, noiseAspect, uTime, 14.0, 1.0, 12.9) * 0.8
            + snowLayer(uv, noiseAspect, uTime, 22.0, 1.5, 31.1) * 0.6;
    acc = ovr(acc, vec3(0.91, 0.94, 1.00), clamp(s, 0.0, 1.0) * (0.5 + 0.4 * uSnow));
  }

  // 7. Sun, on top, per-pixel occluded by the clouds it just drew over.
  // Disc + corona: real SDO/AIA 171 photo (NASA, public domain) with the
  // coronal loops and active regions baked in. The shader only grades the
  // photo's gold tones toward sunset colors and adds a soft outer bloom.
  if (uSunI > 0.004) {
    // Atmospheric refraction flattens the disc near the horizon.
    vec2 ds = vec2((uv.x - uSunPos.x) * aspect, (uv.y - uSunPos.y) * (1.0 + 0.28 * uGolden));
    float rs = length(ds);
    float SR = 0.29;
    vec2 pn = ds / SR;
    float rr = length(pn);

    // Very slow apparent rotation keeps the corona alive.
    float ca = cos(uTime * 0.008);
    float sa = sin(uTime * 0.008);
    vec2 tuv = clamp(mat2(ca, -sa, sa, ca) * pn * 0.47 + 0.5, 0.0, 1.0);
    vec3 stex = texture2D(uSunTex, tuv).rgb;
    float texLum = dot(stex, vec3(0.299, 0.587, 0.114));

    // Disc core is fully opaque; past the limb the photo's own corona
    // drives alpha, giving the irregular flame-like edge of the real sun.
    float inDisc = 1.0 - smoothstep(0.82, 0.88, rr);
    float discA = max(inDisc, smoothstep(0.02, 0.12, texLum))
                * (1.0 - smoothstep(0.90, 1.0, rr));

    // Color grade: by day the multiplier is near-neutral (a touch warm) so
    // each skin keeps its own tones - the tri-color's browns/blues and the
    // PFSS field lines survive; at the horizon everything warms to sunset
    // orange, as the real atmosphere reddens the disc.
    vec3 grade = mix(vec3(1.28, 1.22, 1.12), vec3(1.25, 0.62, 0.28), uGolden);
    vec3 discCol = stex * grade;
    // Gentle luminous fill driven by the source brightness (keeps the disc
    // reading as a light source, brightens active regions, stays hue-neutral).
    discCol += vec3(0.20, 0.17, 0.12) * texLum * (1.0 - smoothstep(0.45, 0.95, rr)) * (1.0 - 0.5 * uGolden);

    // Soft procedural bloom around the photo (kept subtle so the coronal
    // texture stays visible). Falloff constants track the disc radius so the
    // bloom stays proportional now that SR is larger.
    float corona = exp(-rs * rs * 15.0) * 0.38;
    float wide   = exp(-rs * rs * 3.5)  * 0.26;
    float outer  = exp(-rs * rs * 1.25) * 0.12;
    float ang = atan(ds.y, ds.x);
    float rays = (0.55 + 0.45 * sin(ang * 9.0 + uTime * 0.10))
               * (0.55 + 0.45 * sin(ang * 5.0 - uTime * 0.063 + 1.7));
    rays *= exp(-rs * 3.0) * 0.18 * smoothstep(SR * 0.9, SR * 1.8, rs);
    float flicker = 1.0 + 0.015 * sin(uTime * 2.3) * sin(uTime * 3.7);
    float glow = (corona + wide + outer + rays) * flicker;

    vec3 cHalo = mix(vec3(1.00, 0.85, 0.50), vec3(1.00, 0.55, 0.25), uGolden);
    vec3 sunC = mix(cHalo, discCol, clamp(discA, 0.0, 1.0));

    float occ = cloudsA * 0.9;
    float sunA = clamp(discA + glow, 0.0, 1.0) * uSunI * mix(1.0, 0.22, occ);
    sunC *= mix(1.0, 0.55, occ);
    acc = ovr(acc, sunC, sunA);
  }

  gl_FragColor = acc;
}
`;
