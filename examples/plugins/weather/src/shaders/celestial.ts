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
  // Smooth shader-side AA — the PNG bleeds RGB past the disc so the
  // bilinear filter never reads pure black, and we can do a proper
  // smoothstep without a dark fringe.
  float discAA = smoothstep(diskR + 0.003, diskR - 0.003, d);
  if (discAA < 0.001) {
    gl_FragColor = vec4(0.0);
    return;
  }

  // Map disk space to texture UV (texture is square, moon disc fills it).
  vec2 texUV = (p / diskR) * 0.5 + 0.5;
  texUV = clamp(texUV, 0.0, 1.0);
  vec4 moonTex = texture2D(uTexture, texUV);

  // ── Phase shadow ──
  // Shadow disc is the same size as the moon disc (radius 1 in moon-disc
  // coords). Its center moves with phase so:
  //   phase=0   (new):     centerX=0  → covers entire moon (all shadow)
  //   phase=0.25 (waxing): centerX=-1 → covers left half  (right half lit)
  //   phase=0.5  (full):   centerX=-2 → off-disc          (all lit)
  //   phase=0.75 (waning): centerX=+1 → covers right half (left half lit)
  //   phase=1   (new):     centerX=0  → covers entire moon
  vec2 pn = p / diskR;
  float shadowX;
  if (uPhase < 0.5) {
    shadowX = -4.0 * uPhase;          // 0 → -2 over [0, 0.5]
  } else {
    shadowX =  4.0 - 4.0 * uPhase;    // 2 → 0 over [0.5, 1.0]
  }
  vec2 sc = vec2(shadowX, 0.0);
  // Inside the shadow disc → in shadow; smooth at the terminator.
  float shadowMask = 1.0 - smoothstep(0.92, 1.04, length(pn - sc));
  // Dark side retains faint earthshine glow (0.12 base) so it's not pitch black.
  float litFactor = mix(0.12, 1.0, 1.0 - shadowMask);

  // Remap source luminance from [0..1] to [floor..1] so the darkest maria
  // (≈12% in the NASA photo) read as a soft mid-grey instead of pitch
  // black against a dark widget background. Power curve preserves the
  // crater texture without flattening contrast.
  vec3 lifted = mix(vec3(0.42), vec3(1.0), pow(moonTex.rgb, vec3(0.65)));
  // Light limb compensation against natural rim darkening.
  float limbBoost = 1.0 + smoothstep(diskR * 0.55, diskR * 1.0, d) * 0.4;
  vec3 col = clamp(lifted * limbBoost, 0.0, 1.0) * litFactor;

  // Alpha = shader-side smooth AA × intensity.
  gl_FragColor = vec4(col, discAA * uIntensity);
}
`;
