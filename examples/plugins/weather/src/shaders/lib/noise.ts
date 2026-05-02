// Simplex-style 3D noise and fBm helpers.
// Gradient noise implementation adapted from:
//   Stefan Gustavson & Ashima Arts, "webgl-noise" (MIT)
//   https://github.com/ashima/webgl-noise
// fBm stacking pattern after Inigo Quilez:
//   https://iquilezles.org/articles/fbm/

export const NOISE_GLSL = /* glsl */ `
// ---- value noise helper ----
vec3 _hash3(vec3 p) {
  p = vec3(
    dot(p, vec3(127.1, 311.7,  74.7)),
    dot(p, vec3(269.5, 183.3, 246.1)),
    dot(p, vec3(113.5, 271.9, 124.6))
  );
  return fract(sin(p) * 43758.5453123) * 2.0 - 1.0;
}

float vnoise(vec3 p) {
  vec3 i = floor(p);
  vec3 f = fract(p);
  vec3 u = f * f * (3.0 - 2.0 * f);
  return mix(
    mix(mix(dot(_hash3(i + vec3(0,0,0)), f - vec3(0,0,0)),
            dot(_hash3(i + vec3(1,0,0)), f - vec3(1,0,0)), u.x),
        mix(dot(_hash3(i + vec3(0,1,0)), f - vec3(0,1,0)),
            dot(_hash3(i + vec3(1,1,0)), f - vec3(1,1,0)), u.x), u.y),
    mix(mix(dot(_hash3(i + vec3(0,0,1)), f - vec3(0,0,1)),
            dot(_hash3(i + vec3(1,0,1)), f - vec3(1,0,1)), u.x),
        mix(dot(_hash3(i + vec3(0,1,1)), f - vec3(0,1,1)),
            dot(_hash3(i + vec3(1,1,1)), f - vec3(1,1,1)), u.x), u.y), u.z);
}

// Fractional Brownian Motion — 4 octaves
float fbm4(vec3 p) {
  float v = 0.0;
  float a = 0.5;
  mat3 rot = mat3(
    0.00,  0.80,  0.60,
   -0.80,  0.36, -0.48,
   -0.60, -0.48,  0.64);
  for (int i = 0; i < 4; i++) {
    v += a * vnoise(p);
    p  = rot * p * 2.0;
    a *= 0.5;
  }
  return v;
}

// Fractional Brownian Motion — 6 octaves (high quality clouds)
float fbm6(vec3 p) {
  float v = 0.0;
  float a = 0.5;
  mat3 rot = mat3(
    0.00,  0.80,  0.60,
   -0.80,  0.36, -0.48,
   -0.60, -0.48,  0.64);
  for (int i = 0; i < 6; i++) {
    v += a * vnoise(p);
    p  = rot * p * 2.0;
    a *= 0.5;
  }
  return v;
}
`;
