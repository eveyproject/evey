// Two minimal WebGL1 programs, one context, no framebuffers:
//
//   SKY_* - a single fullscreen triangle painting the dome background: a
//   day/twilight/night gradient driven by the Sun's real altitude, a soft
//   procedural Milky Way band (value noise, not an image), a faint sparkle
//   wash, and an occasional shooting star streak (CPU decides when/where,
//   the shader just draws the fading segment).
//
//   BODY_* - every point-like thing (named stars, the four planets, the
//   Moon, the Sun) AND every constellation line, all in one vertex/fragment
//   pair. A per-vertex "kind" selects the fragment behavior: lines get a
//   flat translucent stroke, points get a soft radial sprite (gl_PointCoord)
//   with per-star twinkle, a bigger halo for planets, and a phase terminator
//   for the Moon.

export const SKY_VERT_SRC = `
attribute vec2 aPos;
varying vec2 vUv;
void main() {
  vUv = aPos * 0.5 + 0.5;
  gl_Position = vec4(aPos, 0.0, 1.0);
}
`;

export const SKY_FRAG_SRC = `
#ifdef GL_FRAGMENT_PRECISION_HIGH
precision highp float;
#else
precision mediump float;
#endif

uniform vec2  uRes;
uniform float uTime;
uniform float uStarVis;   // 0 day .. 1 astronomical night
uniform float uTwilight;  // 0..1, peaks at sunset/sunrise
uniform vec2  uSunPos;    // dome-projected sun position, NDC-ish (-1..1), y up
uniform vec4  uShootStar; // x0,y0,x1,y1 in NDC
uniform float uShootProgress; // 0..1, <0 means inactive

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
  for (int i = 0; i < 5; i++) {
    v += a * vnoise(p);
    p = m * p;
    a *= 0.5;
  }
  return v;
}

void main() {
  vec2 uv = vUv;
  float aspect = uRes.x / max(uRes.y, 1.0);
  vec2 p = (uv - 0.5) * vec2(aspect, 1.0) * 2.0;

  // 1. Base sky gradient: deep night blue -> twilight indigo/gold -> day blue.
  vec3 nightTop = vec3(0.02, 0.03, 0.07);
  vec3 nightBot = vec3(0.05, 0.06, 0.12);
  vec3 dayTop   = vec3(0.25, 0.55, 0.95);
  vec3 dayBot   = vec3(0.55, 0.75, 0.98);
  vec3 duskTop  = vec3(0.10, 0.08, 0.20);
  vec3 duskBot  = vec3(0.85, 0.45, 0.25);

  float dayness = 1.0 - uStarVis;
  vec3 top = mix(mix(nightTop, duskTop, uTwilight), dayTop, dayness);
  vec3 bot = mix(mix(nightBot, duskBot, uTwilight), dayBot, dayness);
  vec3 col = mix(bot, top, clamp(uv.y, 0.0, 1.0));

  // 2. Procedural Milky Way band: a soft diagonal fbm ridge, brighter where
  // the noise is denser, masked to a band and faded out in daylight.
  float bandAxis = (p.x + p.y * 1.3) * 0.6 + uTime * 0.002;
  float bandDist = abs(fract(bandAxis * 0.5 + 0.15) - 0.5) * 2.0;
  float band = smoothstep(0.85, 0.0, bandDist);
  float cloudDetail = fbm(p * 1.8 + vec2(0.0, uTime * 0.003));
  float milkyWay = band * (0.35 + 0.65 * cloudDetail) * uStarVis;
  col += vec3(0.65, 0.68, 0.80) * milkyWay * 0.35;

  // 3. Faint background sparkle (distinct from the explicit named-star
  // points drawn by the body program): a dense, cheap dust of sub-pixel
  // twinkle so the sky doesn't look empty between the named stars.
  vec2 gp = p * 140.0;
  vec2 gi = floor(gp);
  float starChance = hash21(gi);
  float twinkle = 0.5 + 0.5 * sin(uTime * (2.0 + hash21(gi + 5.1) * 3.0) + hash21(gi) * 6.283);
  float dust = step(0.9925, starChance) * twinkle;
  col += vec3(dust) * uStarVis * 0.5;

  // 4. Sun glow bleeding into the gradient near the horizon (golden hour).
  float sunGlow = exp(-length(p - uSunPos) * 1.4) * uTwilight * 0.5;
  col += vec3(1.0, 0.65, 0.35) * sunGlow;

  // 5. Rare shooting star: a short fading streak, CPU-scheduled.
  if (uShootProgress >= 0.0) {
    vec2 a = uShootStar.xy;
    vec2 b = uShootStar.xy + (uShootStar.zw - uShootStar.xy) * uShootProgress;
    vec2 ab = b - a;
    float len2 = max(dot(ab, ab), 1e-5);
    float t = clamp(dot(p - a, ab) / len2, 0.0, 1.0);
    vec2 proj = a + ab * t;
    float d = length(p - proj);
    float fade = (1.0 - uShootProgress) * smoothstep(0.0, 1.0, t);
    float streak = exp(-d * 220.0) * fade;
    col += vec3(1.0, 1.0, 0.95) * streak;
  }

  gl_FragColor = vec4(col, 1.0);
}
`;

export const BODY_VERT_SRC = `
attribute vec2 aPos;
attribute vec3 aColor;
attribute float aSize;
attribute float aKind; // 0=line 1=star 2=planet 3=moon 4=sun
attribute float aSeed;

uniform float uTime;
uniform float uStarVis;

varying vec3 vColor;
varying float vKind;
varying float vAlpha;

void main() {
  gl_Position = vec4(aPos, 0.0, 1.0);
  vColor = aColor;
  vKind = aKind;

  float alpha = 1.0;
  if (aKind >= 0.5 && aKind < 1.5) {
    // Star: fade with real day/night visibility and twinkle per-star.
    float tw = 0.65 + 0.35 * sin(uTime * (1.3 + aSeed * 2.1) + aSeed * 6.283);
    alpha = uStarVis * tw;
    gl_PointSize = aSize * (0.85 + 0.3 * tw);
  } else if (aKind >= 3.5) {
    // Sun: only meaningful in daylight/twilight.
    alpha = 1.0;
    gl_PointSize = aSize;
  } else {
    gl_PointSize = aSize;
  }
  vAlpha = alpha;
}
`;

export const BODY_FRAG_SRC = `
#ifdef GL_FRAGMENT_PRECISION_HIGH
precision highp float;
#else
precision mediump float;
#endif

uniform float uMoonPhase; // 0..1, 0.5 = full
uniform float uMoonWaxing; // 1.0 waxing, 0.0 waning

varying vec3 vColor;
varying float vKind;
varying float vAlpha;

void main() {
  if (vKind < 0.5) {
    // Constellation line: flat, faint stroke.
    gl_FragColor = vec4(vColor, 0.35 * vAlpha);
    return;
  }

  vec2 c = gl_PointCoord * 2.0 - 1.0;
  float r = length(c);

  if (vKind < 1.5) {
    // Star point sprite: soft core + gentle falloff.
    float core = smoothstep(1.0, 0.0, r);
    float glow = exp(-r * r * 3.0);
    float a = clamp(core * 1.2 + glow * 0.4, 0.0, 1.0) * vAlpha;
    gl_FragColor = vec4(vColor, a);
    return;
  }

  if (vKind < 2.5) {
    // Planet: brighter disc with a soft halo.
    float disc = smoothstep(0.55, 0.35, r);
    float halo = exp(-r * r * 2.0) * 0.5;
    float a = clamp(disc + halo, 0.0, 1.0) * vAlpha;
    gl_FragColor = vec4(vColor, a);
    return;
  }

  if (vKind < 3.5) {
    // Moon: procedural disc with a phase terminator (no photo texture).
    if (r > 1.0) discard;
    float shadowX = uMoonWaxing > 0.5 ? (1.0 - 2.0 * uMoonPhase) : (2.0 * uMoonPhase - 1.0);
    float side = uMoonWaxing > 0.5 ? -1.0 : 1.0;
    float lit = smoothstep(0.03, -0.03, side * (c.x - shadowX * 1.6));
    vec3 litColor = vec3(0.96, 0.96, 0.92);
    vec3 shadowColor = vec3(0.10, 0.11, 0.14);
    vec3 col = mix(shadowColor, litColor, lit);
    float edge = smoothstep(1.0, 0.9, r);
    gl_FragColor = vec4(col, edge * vAlpha);
    return;
  }

  // Sun: warm disc with rays.
  float disc = smoothstep(0.5, 0.3, r);
  float rays = exp(-r * r * 1.2) * 0.6;
  vec3 col = mix(vec3(1.0, 0.6, 0.25), vec3(1.0, 0.92, 0.6), disc);
  gl_FragColor = vec4(col, clamp(disc + rays, 0.0, 1.0) * vAlpha);
}
`;
