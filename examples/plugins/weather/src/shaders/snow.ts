// Snow flake shader - instanced points with drift and rotation.
// Procedural flake using noise-based soft disk with turbulence.

import { NOISE_GLSL } from "./lib/noise";

export const SNOW_VERT = /* glsl */ `
precision highp float;
attribute vec3  aOffset;
attribute float aSpeed;
attribute float aPhase;
attribute float aSize;
uniform   float uTime;
uniform   vec2  uWind;
varying   float vAlpha;

void main() {
  float t    = mod(uTime * aSpeed * 0.35 + aPhase, 1.0);
  float y    = mix(3.5, -3.5, t);
  float xOff = sin(uTime * 0.7 + aPhase * 3.14159) * 0.3 + uWind.x * t * 0.5;
  vec3 wpos  = aOffset + vec3(xOff, y, 0.0);
  vec4 mv    = modelViewMatrix * vec4(position + wpos, 1.0);
  gl_Position  = projectionMatrix * mv;
  gl_PointSize = aSize * (220.0 / -mv.z);
  vAlpha = smoothstep(0.0, 0.12, t) * smoothstep(1.0, 0.88, t);
}
`;

export const SNOW_FRAG = /* glsl */ `
precision highp float;

${NOISE_GLSL}

uniform float uIntensity;
uniform float uTime;
varying float vAlpha;

void main() {
  vec2  p = gl_PointCoord * 2.0 - 1.0;
  float d = length(p);
  // Soft circular flake
  float flake = smoothstep(1.0, 0.5, d);
  // Add a subtle turbulent texture to the flake surface
  float n = vnoise(vec3(p * 4.0, uTime * 0.1)) * 0.3 + 0.7;
  float alpha = flake * n * vAlpha * uIntensity;
  gl_FragColor = vec4(0.93, 0.96, 1.0, alpha);
}
`;
