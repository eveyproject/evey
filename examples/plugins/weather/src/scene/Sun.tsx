// Sun billboard — positioned via sunAltitude/sunAzimuth.
// Rendered at useFrame priority=2 (after EffectComposer) so bloom never captures
// the sun's high-luminance disk, which would cause full-canvas white wash.
import { useRef, useMemo, useEffect } from "react";
import { useThree, useFrame } from "@react-three/fiber";
import * as THREE from "three";
import { SUN_VERT, SUN_FRAG } from "../shaders/sun";
import { useSceneParams } from "./WeatherScene";
import { useFrameThrottle } from "../runtime/useFrameThrottle";

export function Sun() {
  const params  = useSceneParams();
  const { gl }  = useThree();
  const timeRef = useRef(0);

  const uniforms = useMemo(() => ({
    uColor:     { value: new THREE.Color(...params.palette.sun) },
    uIntensity: { value: 0.0 },
    uTime:      { value: 0.0 },
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }), []);

  // Off-screen scene + orthographic camera dedicated to the sun overlay.
  // Positioned in world space: the sun quad is moved each frame.
  const sunScene  = useMemo(() => new THREE.Scene(), []);
  const sunCamera = useMemo(
    () => new THREE.PerspectiveCamera(40, 1100 / 280, 0.1, 50),
    []
  );

  // Keep camera in sync with the R3F camera (position fixed at [0,0,4]).
  useEffect(() => {
    sunCamera.position.set(0, 0, 4);
    sunCamera.lookAt(0, 0, 0);
  }, [sunCamera]);

  const meshRef = useRef<THREE.Mesh>(null!);

  // Build the sun quad in the dedicated scene.
  useEffect(() => {
    const geo = new THREE.PlaneGeometry(1.8, 1.8);
    const mat = new THREE.ShaderMaterial({
      uniforms,
      vertexShader: SUN_VERT,
      fragmentShader: SUN_FRAG,
      transparent: true,
      depthWrite: false,
      blending: THREE.AdditiveBlending,
    });
    const mesh = new THREE.Mesh(geo, mat);
    meshRef.current = mesh;
    sunScene.add(mesh);
    return () => { sunScene.remove(mesh); geo.dispose(); mat.dispose(); };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sunScene]);

  // Update uniforms + position at throttled 30fps.
  useFrameThrottle((_, dt) => {
    if (!meshRef.current) return;
    timeRef.current += dt;
    uniforms.uTime.value  = timeRef.current;
    uniforms.uColor.value.setRGB(...params.palette.sun);

    // Daytime only, dimmed by cloud cover.
    const dayFactor = params.isDay
      ? Math.max(0, params.sunAltitude / (Math.PI / 2))
      : 0;
    const targetIntensity = dayFactor * Math.pow(1 - params.cloudCover * 0.85, 1.5);
    uniforms.uIntensity.value +=
      (targetIntensity - uniforms.uIntensity.value) * 0.04;

    // Position sun in world space on a sphere of radius 4.
    const r = 4;
    meshRef.current.position.set(
      r * Math.cos(params.sunAltitude) * Math.sin(params.sunAzimuth),
      r * Math.sin(params.sunAltitude),
      -r * Math.cos(params.sunAltitude) * Math.cos(params.sunAzimuth)
    );
    meshRef.current.lookAt(0, 0, 0);
  });

  // Blit the sun onto the canvas AFTER EffectComposer (priority=2).
  // autoClear=false preserves the bloom-composited frame.
  useFrame((state) => {
    if (uniforms.uIntensity.value < 0.005) return;
    const renderer = state.gl;
    const prevAutoClear = renderer.autoClear;
    renderer.autoClear = false;
    renderer.setRenderTarget(null);
    renderer.render(sunScene, sunCamera);
    renderer.autoClear = prevAutoClear;
  }, 2);

  return null;
}
