// 3D scene rendered by R3F. Uses a custom shader sphere + procedural sky
// background + particle systems for rain/snow. No external assets — all
// procedural.

import { useFrame } from "@react-three/fiber";
import { useMemo, useRef } from "react";
import * as THREE from "three";
import type { WeatherProfile } from "./types";

// Sky shader: vertical gradient + faint sun/moon glow at upper-left.
// Explicit precision in BOTH vertex + fragment so uniforms with the same
// name link correctly across stages (three.js otherwise injects different
// platform defaults and the GL linker rejects "uniform not linkable").
const skyVS = /* glsl */ `
  precision highp float;
  varying vec2 vUv;
  void main() {
    vUv = uv;
    gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
  }
`;
const skyFS = /* glsl */ `
  precision highp float;
  uniform vec3 uTop;
  uniform vec3 uBottom;
  uniform float uSun;
  uniform float uTime;
  varying vec2 vUv;
  float hash(vec2 p) { return fract(sin(dot(p, vec2(127.1, 311.7))) * 43758.5453); }
  void main() {
    vec3 col = mix(uBottom, uTop, smoothstep(0.0, 1.0, vUv.y));
    // Sun/moon glow at upper-left, slow drift
    vec2 sunPos = vec2(0.20 + 0.03 * sin(uTime * 0.1), 0.78);
    float d = distance(vUv, sunPos);
    col += uSun * smoothstep(0.35, 0.0, d) * vec3(1.0, 0.9, 0.65) * 0.6;
    // Subtle film grain so the gradient never looks flat
    col += (hash(vUv * 200.0 + uTime) - 0.5) * 0.015;
    gl_FragColor = vec4(col, 1.0);
  }
`;

// Orb shader: noise-displaced sphere, pulsing fresnel rim.
const orbVS = /* glsl */ `
  precision highp float;
  uniform float uTime;
  varying vec3 vNormal;
  varying vec3 vViewDir;
  varying float vDisp;
  // 3D simplex-style noise (compact)
  vec3 hash3(vec3 p) {
    p = vec3(dot(p, vec3(127.1, 311.7, 74.7)),
             dot(p, vec3(269.5, 183.3, 246.1)),
             dot(p, vec3(113.5, 271.9, 124.6)));
    return -1.0 + 2.0 * fract(sin(p) * 43758.5453123);
  }
  float noise(vec3 p) {
    vec3 i = floor(p); vec3 f = fract(p);
    vec3 u = f * f * (3.0 - 2.0 * f);
    return mix(mix(mix(dot(hash3(i + vec3(0,0,0)), f - vec3(0,0,0)),
                       dot(hash3(i + vec3(1,0,0)), f - vec3(1,0,0)), u.x),
                   mix(dot(hash3(i + vec3(0,1,0)), f - vec3(0,1,0)),
                       dot(hash3(i + vec3(1,1,0)), f - vec3(1,1,0)), u.x), u.y),
               mix(mix(dot(hash3(i + vec3(0,0,1)), f - vec3(0,0,1)),
                       dot(hash3(i + vec3(1,0,1)), f - vec3(1,0,1)), u.x),
                   mix(dot(hash3(i + vec3(0,1,1)), f - vec3(0,1,1)),
                       dot(hash3(i + vec3(1,1,1)), f - vec3(1,1,1)), u.x), u.y), u.z);
  }
  void main() {
    float n = noise(position * 1.6 + uTime * 0.25);
    vec3 displaced = position + normal * n * 0.18;
    vDisp = n;
    vNormal = normalize(normalMatrix * normal);
    vec4 mv = modelViewMatrix * vec4(displaced, 1.0);
    vViewDir = normalize(-mv.xyz);
    gl_Position = projectionMatrix * mv;
  }
`;
const orbFS = /* glsl */ `
  precision highp float;
  uniform vec3 uColor;
  uniform float uTime;
  varying vec3 vNormal;
  varying vec3 vViewDir;
  varying float vDisp;
  void main() {
    // Fresnel rim — brighter at glancing angles, gives the orb a "powered" feel.
    float fres = pow(1.0 - max(dot(normalize(vNormal), normalize(vViewDir)), 0.0), 2.5);
    vec3 base = uColor * (0.55 + 0.45 * (vDisp * 0.5 + 0.5));
    vec3 rim = uColor * 1.6 + vec3(0.2);
    vec3 col = mix(base, rim, fres);
    // Animated inner pulse
    col += uColor * 0.3 * (0.5 + 0.5 * sin(uTime * 2.0 + vDisp * 4.0));
    gl_FragColor = vec4(col, 1.0);
  }
`;

function Sky({ profile }: { profile: WeatherProfile }) {
  const matRef = useRef<THREE.ShaderMaterial>(null!);
  const uniforms = useMemo(() => ({
    uTop: { value: new THREE.Vector3(...profile.skyTop) },
    uBottom: { value: new THREE.Vector3(...profile.skyBottom) },
    uSun: { value: profile.sunBrightness },
    uTime: { value: 0 },
  }), []);
  useFrame((_, dt) => {
    uniforms.uTime.value += dt;
    uniforms.uTop.value.set(...profile.skyTop);
    uniforms.uBottom.value.set(...profile.skyBottom);
    uniforms.uSun.value += (profile.sunBrightness - uniforms.uSun.value) * 0.05;
  });
  return (
    <mesh position={[0, 0, -10]} renderOrder={-1}>
      <planeGeometry args={[40, 30]} />
      <shaderMaterial
        ref={matRef}
        uniforms={uniforms}
        vertexShader={skyVS}
        fragmentShader={skyFS}
        depthWrite={false}
        depthTest={false}
      />
    </mesh>
  );
}

function Orb({ profile }: { profile: WeatherProfile }) {
  const meshRef = useRef<THREE.Mesh>(null!);
  const uniforms = useMemo(() => ({
    uColor: { value: new THREE.Vector3(...profile.orbColor) },
    uTime: { value: 0 },
  }), []);
  useFrame((_, dt) => {
    uniforms.uTime.value += dt;
    uniforms.uColor.value.set(...profile.orbColor);
    if (meshRef.current) {
      meshRef.current.rotation.y += dt * 0.25;
      meshRef.current.rotation.x = Math.sin(uniforms.uTime.value * 0.4) * 0.15;
    }
  });
  return (
    <mesh ref={meshRef} position={[0.7, 0, 0]}>
      <icosahedronGeometry args={[1.0, 24]} />
      <shaderMaterial uniforms={uniforms} vertexShader={orbVS} fragmentShader={orbFS} />
    </mesh>
  );
}

function Particles({ profile }: { profile: WeatherProfile }) {
  const pointsRef = useRef<THREE.Points>(null!);
  const count = 220;
  const geom = useMemo(() => {
    const g = new THREE.BufferGeometry();
    const pos = new Float32Array(count * 3);
    const speeds = new Float32Array(count);
    for (let i = 0; i < count; i++) {
      pos[i * 3] = (Math.random() - 0.5) * 8;
      pos[i * 3 + 1] = Math.random() * 6 + 1;
      pos[i * 3 + 2] = (Math.random() - 0.5) * 4 - 1;
      speeds[i] = 0.5 + Math.random() * 1.2;
    }
    g.setAttribute("position", new THREE.BufferAttribute(pos, 3));
    g.setAttribute("speed", new THREE.BufferAttribute(speeds, 1));
    return g;
  }, []);

  useFrame((_, dt) => {
    if (!profile.hasRain && !profile.hasSnow) return;
    const positions = geom.attributes.position.array as Float32Array;
    const speeds = geom.attributes.speed.array as Float32Array;
    const isSnow = profile.hasSnow;
    for (let i = 0; i < count; i++) {
      const v = isSnow ? speeds[i] * 0.6 : speeds[i] * 4.5;
      positions[i * 3 + 1] -= v * dt;
      if (isSnow) positions[i * 3] += Math.sin(positions[i * 3 + 1] * 2 + i) * dt * 0.3;
      if (positions[i * 3 + 1] < -3) {
        positions[i * 3 + 1] = 5 + Math.random();
        positions[i * 3] = (Math.random() - 0.5) * 8;
      }
    }
    geom.attributes.position.needsUpdate = true;
  });

  if (!profile.hasRain && !profile.hasSnow) return null;
  return (
    <points ref={pointsRef} geometry={geom}>
      <pointsMaterial
        size={profile.hasSnow ? 0.08 : 0.04}
        color={profile.hasSnow ? "#ffffff" : "#9bc7ff"}
        transparent
        opacity={0.85}
        sizeAttenuation
      />
    </points>
  );
}

function StormFlash({ profile }: { profile: WeatherProfile }) {
  const lightRef = useRef<THREE.PointLight>(null!);
  useFrame((state) => {
    if (!profile.hasStorm || !lightRef.current) {
      if (lightRef.current) lightRef.current.intensity = 0;
      return;
    }
    const t = state.clock.elapsedTime;
    // Random flash every ~3-5 seconds
    const flash = Math.pow(Math.max(0, Math.sin(t * 0.7) - 0.96), 2.0) * 80;
    lightRef.current.intensity = flash;
  });
  return <pointLight ref={lightRef} position={[2, 3, 2]} color="#fff8d0" intensity={0} />;
}

export function Scene({ profile }: { profile: WeatherProfile }) {
  return (
    <>
      <Sky profile={profile} />
      <ambientLight intensity={0.5} />
      <pointLight position={[3, 4, 3]} intensity={0.8} color="#fff5d8" />
      <Orb profile={profile} />
      <Particles profile={profile} />
      <StormFlash profile={profile} />
    </>
  );
}
