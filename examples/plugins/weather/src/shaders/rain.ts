// Rain streak shader - instanced billboard particles.
// Inspired by: rye terrell's rain shader technique,
// and "Heavy Rain" by sirenian on Shadertoy.
// Each instance is a thin vertical quad; fragment elongates the streak.

export const RAIN_VERT = /* glsl */ `
precision highp float;
attribute vec3  aOffset;    // per-instance world offset
attribute float aSpeed;     // fall speed multiplier
attribute float aPhase;     // initial vertical phase
uniform   float uTime;
uniform   vec2  uWind;      // drift direction
varying   float vAlpha;
varying   float vStretch;

void main() {
  float t      = mod(uTime * aSpeed + aPhase, 1.0);
  // wrap vertically 3..−3
  float yWorld = mix(3.0, -3.0, t);
  float xDrift = uWind.x * t * 0.8;
  vec3  worldPos = aOffset + vec3(xDrift, yWorld, 0.0);
  // Billboard: keep local geometry but translate by instance offset
  vec4 mvPos = modelViewMatrix * vec4(position + worldPos, 1.0);
  gl_Position = projectionMatrix * mvPos;
  vAlpha   = smoothstep(0.0, 0.1, t) * smoothstep(1.0, 0.85, t);
  vStretch = 1.0;
}
`;

export const RAIN_FRAG = /* glsl */ `
precision highp float;
uniform float uIntensity;
varying float vAlpha;
varying float vStretch;

void main() {
  vec2  p  = gl_PointCoord * 2.0 - 1.0;
  float d  = abs(p.x) * 3.0 + abs(p.y) * 0.2;
  float a  = smoothstep(1.0, 0.0, d);
  gl_FragColor = vec4(0.70, 0.82, 0.96, a * vAlpha * uIntensity * 0.7);
}
`;
