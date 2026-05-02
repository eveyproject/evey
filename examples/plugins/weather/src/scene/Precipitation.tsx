// GPU-instanced rain and snow particles.
// Tier caps: high=3000, med=1200, low=500.
// prefers-reduced-motion halves count.
import { useRef, useMemo } from "react";
import * as THREE from "three";
import { useSceneParams } from "./WeatherScene";
import { useFrameThrottle } from "../runtime/useFrameThrottle";

type Tier = "high" | "med" | "low";

function detectTier(): Tier {
  try {
    const canvas = document.createElement("canvas");
    const gl = canvas.getContext("webgl2") || canvas.getContext("webgl");
    if (!gl) return "low";
    const maxTex = gl.getParameter(gl.MAX_TEXTURE_SIZE) as number;
    const cores  = navigator.hardwareConcurrency ?? 2;
    const isWebGL2 = "drawBuffers" in gl;
    if (isWebGL2 && maxTex >= 8192 && cores >= 4) return "high";
    if (maxTex >= 4096) return "med";
    return "low";
  } catch {
    return "low";
  }
}

const prefersReduced = typeof window !== "undefined" &&
  window.matchMedia("(prefers-reduced-motion: reduce)").matches;

const TIER_CAP: Record<Tier, number> = { high: 3000, med: 1200, low: 500 };

function makeParticleGeom(count: number, isRain: boolean) {
  const positions = new Float32Array(count * 3);
  const speeds    = new Float32Array(count);
  const phases    = new Float32Array(count);
  const sizes     = new Float32Array(count);

  for (let i = 0; i < count; i++) {
    positions[i * 3]     = (Math.random() - 0.5) * 10;
    positions[i * 3 + 1] = Math.random() * 8 - 4;
    positions[i * 3 + 2] = (Math.random() - 0.5) * 6;
    speeds[i]  = 0.6 + Math.random() * 0.8;
    phases[i]  = Math.random();
    sizes[i]   = isRain
      ? 0.03 + Math.random() * 0.02
      : 0.04 + Math.random() * 0.06;
  }

  const g = new THREE.BufferGeometry();
  g.setAttribute("position", new THREE.BufferAttribute(positions, 3));
  g.setAttribute("aSpeed",   new THREE.BufferAttribute(speeds, 1));
  g.setAttribute("aPhase",   new THREE.BufferAttribute(phases, 1));
  g.setAttribute("aSize",    new THREE.BufferAttribute(sizes, 1));
  return g;
}

export function Precipitation() {
  const params = useSceneParams();
  const tier   = useMemo(detectTier, []);
  const cap    = Math.floor(TIER_CAP[tier] * (prefersReduced ? 0.5 : 1));
  const isRain = params.precipitation === "rain";
  const isSnow = params.precipitation === "snow";

  const pointsRef = useRef<THREE.Points>(null!);
  const timeRef   = useRef(0);

  const geom = useMemo(
    () => makeParticleGeom(cap, isRain),
  // eslint-disable-next-line react-hooks/exhaustive-deps
  [cap, isRain]);

  useFrameThrottle((_, dt) => {
    if (!pointsRef.current) return;
    if (params.precipitation === "none") {
      pointsRef.current.visible = false;
      return;
    }
    pointsRef.current.visible = true;
    timeRef.current += dt;

    const pos    = geom.attributes.position.array as Float32Array;
    const speeds = geom.attributes.aSpeed.array as Float32Array;
    const [wx]   = params.windVec;

    for (let i = 0; i < cap; i++) {
      const fallSpeed = isRain
        ? speeds[i] * 5.5 * params.precipIntensity
        : speeds[i] * 1.0 * params.precipIntensity;
      pos[i * 3 + 1] -= fallSpeed * dt;
      if (isSnow) {
        // Gentle horizontal drift
        pos[i * 3] += Math.sin(timeRef.current * 0.5 + i * 0.3) * dt * 0.15;
        pos[i * 3] += wx * dt * 0.3;
      } else {
        pos[i * 3] += wx * dt * 1.2;
      }
      if (pos[i * 3 + 1] < -5) {
        pos[i * 3 + 1] = 5 + Math.random() * 2;
        pos[i * 3]     = (Math.random() - 0.5) * 10;
      }
    }
    geom.attributes.position.needsUpdate = true;
  });

  if (params.precipitation === "none") return null;

  return (
    <points ref={pointsRef} geometry={geom} renderOrder={3}>
      <pointsMaterial
        size={isRain ? 0.035 : 0.065}
        color={isRain ? "#9bc7ff" : "#e8f0ff"}
        transparent
        opacity={Math.min(0.85, params.precipIntensity * 0.9 + 0.1)}
        sizeAttenuation
        depthWrite={false}
      />
    </points>
  );
}
