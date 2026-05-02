// CelestialBody — renders sun and/or moon as billboard quads in 3D world space.
//
// Strategy: place each body on a sphere of radius 4 (matching R3F camera setup),
// billboard-facing the camera, rendered in a dedicated off-screen scene with a
// PerspectiveCamera that mirrors the R3F main camera (fov=40, pos=[0,0,4]).
// This guarantees a circular disk independent of canvas aspect ratio.
//
// Position on sphere from sunAltitude/sunAzimuth:
//   x = r * cos(alt) * sin(az)
//   y = r * sin(alt)
//   z = -r * cos(alt) * cos(az)
//
// Quad world size calibrated so the disk appears ~80px tall on a 280px canvas.
// Rendered AFTER clouds at useFrame priority=3.

import { useRef, useMemo, useEffect } from "react";
import { useThree, useFrame } from "@react-three/fiber";
import * as THREE from "three";
import { CELESTIAL_VERT, SUN_FRAG, MOON_FRAG } from "../shaders/celestial";
import { useSceneParams } from "./WeatherScene";
import { useFrameThrottle } from "../runtime/useFrameThrottle";
import { cloudTexRef, moonStateRef } from "./cloudTextureRef";
// Moon texture: NASA/GSFC via Wikipedia Commons, public domain.
// Source: https://commons.wikimedia.org/wiki/File:FullMoon2010.jpg
// Original credit: NASA / Goddard Space Flight Center / Arizona State University
import moonTextureUrl from "../assets/moon.png";

// Sphere radius matching R3F camera z=4
const R = 4;

// Quad half-size calibrated for ~80px disk diameter on 280px canvas at fov=40.
// half-canvas-height in world at z=4: tan(20°) * 4 ≈ 1.456 units = 140px
// disk_radius_px = diskR(0.22) * quad_half * (140 / 1.456)
// For 40px:  quad_half = 40 * 1.456 / (0.22 * 140) ≈ 1.88
// Sphere of radius 4 from camera at z=4 → dist_to_obj ≈ 8 for most positions.
// disk_radius_px = diskR * quad_half * canvas_half_px / (tan(fov/2) * dist)
//                = 0.22 * quad_half * 140 / (0.364 * 8) = 10.58 * quad_half
// For 40px disk radius (80px diameter): quad_half = 40 / 10.58 ≈ 3.78
// For 31px moon disk radius: quad_half = 31 / 10.58 ≈ 2.93
const SUN_QUAD_HALF  = 3.80;   // → ~80px disk diameter
const MOON_QUAD_HALF = 10.00;  // → ~210px disk diameter (2× the previous size)

export function CelestialBody() {
  const params  = useSceneParams();
  const { gl, size } = useThree();

  // Two scenes: moon is rendered BEFORE the cloud upscale so cloud puffs
  // sit on top of it; sun is rendered AFTER everything so it stays on top.
  const moonScene = useMemo(() => new THREE.Scene(), []);
  const sunScene  = useMemo(() => new THREE.Scene(), []);
  const celestialCamera = useMemo(() => {
    const cam = new THREE.PerspectiveCamera(40, size.width / size.height, 0.1, 50);
    cam.position.set(0, 0, 4);
    cam.lookAt(0, 0, 0);
    return cam;
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    celestialCamera.aspect = size.width / size.height;
    celestialCamera.updateProjectionMatrix();
  }, [celestialCamera, size]);

  // ─── Sun ────────────────────────────────────────────────────────────────
  const sunMeshRef = useRef<THREE.Mesh>(null!);
  const timeRef    = useRef(0);

  const sunUniforms = useMemo(() => ({
    uIntensity:  { value: 0.0 },
    uTime:       { value: 0.0 },
    uClouds:     { value: null as THREE.Texture | null },
    uScreenSize: { value: new THREE.Vector2(size.width, size.height) },
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }), []);

  useEffect(() => {
    const s = SUN_QUAD_HALF * 2;
    const geo = new THREE.PlaneGeometry(s, s);
    const mat = new THREE.ShaderMaterial({
      uniforms:       sunUniforms,
      vertexShader:   CELESTIAL_VERT,
      fragmentShader: SUN_FRAG,
      transparent:    true,
      depthWrite:     false,
      blending:       THREE.NormalBlending,
    });
    const mesh = new THREE.Mesh(geo, mat);
    sunMeshRef.current = mesh;
    sunScene.add(mesh);
    return () => { sunScene.remove(mesh); geo.dispose(); mat.dispose(); };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sunScene]);

  // ─── Moon ───────────────────────────────────────────────────────────────
  const moonMeshRef = useRef<THREE.Mesh>(null!);

  const moonTexture = useMemo(() => {
    const tex = new THREE.TextureLoader().load(moonTextureUrl);
    tex.minFilter = THREE.LinearFilter;
    tex.magFilter = THREE.LinearFilter;
    tex.colorSpace = THREE.SRGBColorSpace;
    tex.anisotropy = 4;
    return tex;
  }, []);

  const moonUniforms = useMemo(() => ({
    uIntensity: { value: 0.0 },
    uPhase:     { value: params.moonPhase },
    uTexture:   { value: moonTexture },
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }), [moonTexture]);

  useEffect(() => {
    const s = MOON_QUAD_HALF * 2;
    const geo = new THREE.PlaneGeometry(s, s);
    const mat = new THREE.ShaderMaterial({
      uniforms:       moonUniforms,
      vertexShader:   CELESTIAL_VERT,
      fragmentShader: MOON_FRAG,
      transparent:    true,
      depthWrite:     false,
      blending:       THREE.NormalBlending,
    });
    const mesh = new THREE.Mesh(geo, mat);
    moonMeshRef.current = mesh;
    moonScene.add(mesh);
    return () => { moonScene.remove(mesh); geo.dispose(); mat.dispose(); };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [moonScene]);

  // ─── Update loop ────────────────────────────────────────────────────────
  useFrameThrottle((_, dt) => {
    const alt = params.sunAltitude;
    const az  = params.sunAzimuth;

    // Per-pixel cloud occlusion is now done in the sun shader itself.
    // Keep a tiny global dim so completely overcast skies still feel muted.
    const cloudDim = 1.0 - params.cloudCover * 0.10;

    // ── Sun ──
    // Source of truth: Open-Meteo's is_day flag (accurate for the season).
    // Our local sunPosition() is a simplified hour-of-day curve with no
    // latitude/season correction, so it can lie at long-day latitudes.
    const sunVisible = params.isDay || alt > 0.0;
    const sunFull    = sunVisible ? 1.0 : 0.0;
    const targetSunI = sunVisible ? sunFull * cloudDim : 0.0;
    sunUniforms.uIntensity.value += (targetSunI - sunUniforms.uIntensity.value) * 0.04;
    timeRef.current += dt;
    sunUniforms.uTime.value = timeRef.current;
    sunUniforms.uClouds.value = cloudTexRef.current;
    const drawBuffer = gl.getDrawingBufferSize(new THREE.Vector2());
    sunUniforms.uScreenSize.value.set(drawBuffer.x, drawBuffer.y);

    if (sunMeshRef.current) {
      // Squash vertical mapping so the sun stays inside the frustum
      // (camera fov=40°, aspect ~4:1 → vertical range only ±1.45 units).
      // When Open-Meteo says day but our local altitude curve drops below
      // zero (long-day latitudes), force a positive visual altitude so the
      // sun stays in-frame instead of sliding out the bottom.
      const visualAlt = params.isDay && alt < 0.15 ? 0.45 : alt;
      const yMaxWorld = 0.95;
      sunMeshRef.current.position.set(
        R * Math.cos(visualAlt) * Math.sin(az),
        Math.sin(visualAlt) * yMaxWorld,
        -R * Math.cos(visualAlt) * Math.cos(az)
      );
      sunMeshRef.current.lookAt(0, 0, 0);
    }

    // ── Moon ──
    const moonAlt   = -alt;
    const moonAz    = az + Math.PI;
    // Hide moon whenever Open-Meteo reports daytime, regardless of what
    // our simplified hour curve says.
    const moonVisible = !params.isDay && alt < 0.0;
    const moonFull    = moonVisible ? 1.0 : 0.0;
    const targetMoonI = moonVisible ? moonFull * cloudDim : 0.0;
    moonUniforms.uIntensity.value += (targetMoonI - moonUniforms.uIntensity.value) * 0.04;
    moonUniforms.uPhase.value      = params.moonPhase;

    if (moonMeshRef.current) {
      const yMaxWorld = 1.05;
      moonMeshRef.current.position.set(
        R * Math.cos(moonAlt) * Math.sin(moonAz),
        Math.sin(moonAlt) * yMaxWorld,
        -R * Math.cos(moonAlt) * Math.cos(moonAz)
      );
      moonMeshRef.current.lookAt(0, 0, 0);

      // Project moon world position to screen NDC for the cloud halo shader.
      const ndc = moonMeshRef.current.position.clone().project(celestialCamera);
      moonStateRef.screenX = ndc.x;
      moonStateRef.screenY = ndc.y;
      moonStateRef.intensity = moonUniforms.uIntensity.value;
    } else {
      moonStateRef.intensity = 0;
    }
  });

  // ─── Moon pass (priority=1.9): rendered BEFORE the cloud upscale (which
  // runs at priority=2) so cloud puffs occlude the moon naturally. ────────
  useFrame((state) => {
    if (moonUniforms.uIntensity.value <= 0.005) return;
    const renderer = state.gl;
    const prevAutoClear = renderer.autoClear;
    renderer.autoClear = false;
    renderer.setRenderTarget(null);
    renderer.render(moonScene, celestialCamera);
    renderer.autoClear = prevAutoClear;
  }, 1.9);

  // ─── Sun pass (priority=3): rendered AFTER everything so the disc and
  // its halo sit on top, with per-pixel cloud occlusion handled inside
  // SUN_FRAG. ─────────────────────────────────────────────────────────────
  useFrame((state) => {
    if (sunUniforms.uIntensity.value <= 0.005) return;
    const renderer = state.gl;
    const prevAutoClear = renderer.autoClear;
    renderer.autoClear = false;
    renderer.setRenderTarget(null);
    renderer.render(sunScene, celestialCamera);
    renderer.autoClear = prevAutoClear;
  }, 3);

  void gl;
  return null;
}
