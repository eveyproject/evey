// Celestial body shaders — sun disk + gaussian halo, and procedural moon with phase.
// Both are screen-space quads positioned by the caller via NDC offset uniforms.
//
// The PlaneGeometry quad is sized as: width = size / aspect, height = size.
// With the ortho camera [-1,1]², this produces square pixels because:
//   1 UV unit in x = size/aspect * (canvas_w/2) px = size * (canvas_h/2) px
//   1 UV unit in y = size        * (canvas_h/2) px
// So the UV space is already isotropic — no aspect correction needed in the shader.

// ─── Shared vertex shader ────────────────────────────────────────────────────
export const CELESTIAL_VERT = /* glsl */ `
precision highp float;
varying vec2 vUv;
void main() {
  vUv = uv;
  gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
}
`;

// ─── Sun ─────────────────────────────────────────────────────────────────────
// Disk: white-yellow center, warm orange halo. diskR in UV [-1,1]² space.
// Disk diameter in pixels = diskR * 2 * size_half * (canvas_height/2)
//   = 0.22 * 2 * 1.30 * 140 ≈ 80px on a 280px-tall canvas.
export const SUN_FRAG = /* glsl */ `
precision highp float;
uniform float     uIntensity;
uniform float     uTime;
uniform sampler2D uClouds;
uniform vec2      uScreenSize;
varying vec2      vUv;

void main() {
  vec2 p = vUv * 2.0 - 1.0;
  float d = length(p);

  // Larger solid disk so the sun reads as a strong celestial body.
  float diskR = 0.30;
  float disk  = smoothstep(diskR + 0.050, diskR - 0.025, d);
  // Inner solid plateau (flat white core).
  float core  = step(d, diskR - 0.030);

  // Subtle limb-darkening
  float limb  = 1.0 - smoothstep(0.0, diskR, d) * 0.18;

  // Bright tight corona that hugs the disk
  float corona = exp(-d * d * 8.0) * 1.10;

  // Wide warm glow extending well past the disk
  float wideGlow = exp(-d * d * 3.0) * 0.55;

  // Outer atmospheric ring
  float outerGlow = exp(-d * d * 1.2) * 0.28;

  // Slow flicker
  float flicker = 1.0 + 0.025 * sin(uTime * 2.8) * sin(uTime * 4.7);
  corona *= flicker;

  // Disk drives alpha to 1.0 — the body is fully opaque.
  float alpha = max(disk, clamp(corona + wideGlow + outerGlow, 0.0, 1.0));

  // Color: solid near-white core → warm yellow/orange halo
  vec3 cCore   = vec3(1.00, 0.99, 0.92);
  vec3 cEdge   = vec3(1.00, 0.85, 0.50);
  vec3 cHalo   = vec3(1.00, 0.72, 0.35);
  vec3 col = mix(cEdge, cCore, disk * limb);
  // Core override — inside the disk we want no halo color bleed.
  col = mix(col, cCore, core);
  col += cEdge * corona * 0.85;
  col += cHalo * wideGlow * 0.90;
  col += cHalo * outerGlow * 0.55;

  // Per-pixel cloud occlusion: sample the cloud texture at the same screen
  // UV (matching the upscale's -0.32 vertical offset). Where clouds cover
  // the sun, dim its color and alpha so the disc dulls behind the puff
  // without affecting cloud-free pixels.
  vec2 uvScreen = gl_FragCoord.xy / uScreenSize;
  vec2 cloudUV  = vec2(uvScreen.x, uvScreen.y - 0.32);
  float cloudA  = 0.0;
  if (cloudUV.y >= 0.0 && cloudUV.y <= 1.0) {
    cloudA = texture2D(uClouds, cloudUV).a;
  }
  float occ = cloudA * 0.85;
  col   *= mix(1.0, 0.45, occ);
  alpha *= mix(1.0, 0.55, occ);

  gl_FragColor = vec4(col, alpha * uIntensity);
}
`;

// ─── Moon ────────────────────────────────────────────────────────────────────
// Texture-based: samples a real HD photo of the moon (NASA, public domain).
// Phase shadow: offset disc mask sweeping across the disk (same as before).
// diskR = 0.20 → diameter ~62px on 280px canvas (slightly smaller than sun).
export const MOON_FRAG = /* glsl */ `
precision highp float;
uniform float     uPhase;      // 0..1  (0=new, 0.5=full, 1=new)
uniform float     uIntensity;  // 0..1
uniform sampler2D uTexture;
varying vec2      vUv;

void main() {
  vec2 p = vUv * 2.0 - 1.0;
  float d = length(p);

  float diskR = 0.20;
  float edgeFade = smoothstep(diskR + 0.025, diskR - 0.005, d);

  // Outside the disk: soft halo only, no texture sample.
  if (edgeFade < 0.001) {
    float halo = exp(-d * d * 8.0) * 0.20;
    gl_FragColor = vec4(vec3(0.85, 0.88, 0.95) * halo, halo * uIntensity);
    return;
  }

  // Map disk space to texture UV (texture is square, moon disc fills it).
  vec2 texUV = (p / diskR) * 0.5 + 0.5;
  texUV = clamp(texUV, 0.0, 1.0);
  vec4 moonTex = texture2D(uTexture, texUV);

  // ── Phase shadow (same geometry as the previous procedural version) ──
  const float PI = 3.14159265;
  float phaseAngle = uPhase * 2.0 * PI;
  // terminatorX: +1 at new(0), 0 at quarter(0.25), -1 at full(0.5)
  float terminatorX = cos(phaseAngle);
  vec2 pn = p / diskR;
  float shadowMask;
  if (uPhase < 0.5) {
    // Waxing: shadow disc on left, shrinks as phase → 0.5
    vec2 sc = vec2(terminatorX, 0.0);
    shadowMask = 1.0 - smoothstep(0.95, 1.05, length(pn - sc));
  } else {
    // Waning: shadow disc on right
    vec2 sc = vec2(-terminatorX, 0.0);
    shadowMask = 1.0 - smoothstep(0.95, 1.05, length(pn - sc));
  }
  // Dark side retains faint earthshine glow (0.10 base)
  float litFactor = mix(0.10, 1.0, 1.0 - shadowMask);

  vec3 col = moonTex.rgb * litFactor;
  // Subtle blue-grey lift for night-mood
  col = mix(col, col * vec3(0.95, 0.97, 1.05), 0.15);

  gl_FragColor = vec4(col, edgeFade * uIntensity * moonTex.a);
}
`;
