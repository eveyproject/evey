// GLSL ES 1.00 sources. Two tiny programs, same philosophy as weather-v2/
// cosmos: no scene graph, no framebuffers, everything hand-rolled.
//   - BG: the cinema-room ambiance (vignette, warm curtain gradient, film
//     grain, the sweeping/locked spotlight beam).
//   - POSTER: one textured quad per poster, blended from its placeholder
//     gradient to the real photo as it loads, dimmed by depth/row and lit by
//     the spotlight beam passing over it.

export const BG_VERT_SRC = `
attribute vec2 aPos;
varying vec2 vUv;
void main() {
  vUv = aPos * 0.5 + 0.5;
  gl_Position = vec4(aPos, 0.0, 1.0);
}
`;

export const BG_FRAG_SRC = `
precision mediump float;
varying vec2 vUv;
uniform vec2 uRes;
uniform float uTime;
uniform float uSpotX;
uniform float uSpotIntensity;
uniform float uSpotWidth;
uniform float uLock;

void main() {
  vec2 uv = vUv;
  vec3 base = mix(vec3(0.015, 0.012, 0.018), vec3(0.09, 0.045, 0.03), uv.y);

  vec2 c = (uv - 0.5) * vec2(uRes.x / max(uRes.y, 1.0), 1.0);
  float vig = smoothstep(0.95, 0.2, length(c));
  vec3 col = base * mix(0.35, 1.0, vig);

  float dx = abs(uv.x - (uSpotX * 0.5 + 0.5));
  float beam = smoothstep(uSpotWidth, 0.0, dx) * (1.0 - uv.y * 0.35);
  float intensity = uSpotIntensity * mix(1.0, 1.9, uLock);
  col += vec3(1.0, 0.86, 0.56) * beam * intensity * 0.55;

  float n = fract(sin(dot(uv * uRes.xy + uTime * 53.0, vec2(12.9898, 78.233))) * 43758.5453);
  col += (n - 0.5) * 0.018;

  gl_FragColor = vec4(col, 1.0);
}
`;

export const POSTER_VERT_SRC = `
attribute vec2 aCorner;
uniform vec2 uCenter;
uniform vec2 uSize;
uniform float uAspect;
varying vec2 vUv;

void main() {
  vUv = aCorner + 0.5;
  vec2 pos = uCenter + aCorner * uSize;
  gl_Position = vec4(pos.x / max(uAspect, 0.0001), pos.y, 0.0, 1.0);
}
`;

export const POSTER_FRAG_SRC = `
precision mediump float;
varying vec2 vUv;
uniform sampler2D uTex;
uniform vec3 uPlaceholder;
uniform float uLoaded;
uniform float uDepthFade;
uniform float uHighlight;
uniform float uSpotLight;
uniform float uAlpha;

void main() {
  vec3 texColor = texture2D(uTex, vec2(vUv.x, 1.0 - vUv.y)).rgb;
  vec3 placeholderColor = uPlaceholder * (0.82 + 0.32 * vUv.y);
  vec3 col = mix(placeholderColor, texColor, uLoaded);
  col *= mix(0.5, 1.0, 1.0 - uDepthFade);
  col += uHighlight * 0.22;
  col *= (0.68 + 0.32 * uSpotLight);

  vec2 edge = abs(vUv - 0.5) * 2.0;
  float border = smoothstep(0.95, 1.0, max(edge.x, edge.y));
  col = mix(col, vec3(0.0), border * 0.55);

  gl_FragColor = vec4(col, uAlpha);
}
`;
