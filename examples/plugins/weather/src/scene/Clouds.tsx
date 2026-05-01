// Cloud system — two-pass: FBM cloud FBO (half-res) + overlay composite.
// The overlay is rendered AFTER the EffectComposer (priority 2) so the HDR
// bloom pass never touches it — preventing bloom blow-out on cloud pixels.
import { useRef, useMemo, useEffect } from "react";
import { useThree, useFrame } from "@react-three/fiber";
import * as THREE from "three";
import { useFBO } from "@react-three/drei";
import { CLOUDS_VERT, CLOUDS_FRAG, CLOUD_UPSCALE_VERT, CLOUD_UPSCALE_FRAG } from "../shaders/clouds";
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

// Respect prefers-reduced-motion
const prefersReduced = typeof window !== "undefined" &&
  window.matchMedia("(prefers-reduced-motion: reduce)").matches;

export function Clouds() {
  const params   = useSceneParams();
  const { size, gl } = useThree();
  const tierRef  = useRef<Tier>(detectTier());
  const tier     = tierRef.current;
  const timeRef  = useRef(0);

  // Half-resolution FBO for cloud rendering
  const fboW = Math.max(1, Math.floor(size.width  / 2));
  const fboH = Math.max(1, Math.floor(size.height / 2));
  const fbo  = useFBO(fboW, fboH, {
    minFilter: THREE.LinearFilter,
    magFilter: THREE.LinearFilter,
    format: THREE.RGBAFormat,
    type: THREE.UnsignedByteType,
    depthBuffer: false,
  });

  // Dedicated off-screen scene + camera for the cloud FBO pass
  const rmScene  = useMemo(() => new THREE.Scene(), []);
  const rmCamera = useMemo(() => new THREE.OrthographicCamera(-1, 1, 1, -1, 0, 1), []);

  const cloudUniforms = useMemo(() => ({
    uTime:     { value: 0 },
    uCover:    { value: params.cloudCover },
    uDensity:  { value: params.cloudDensity },
    uSpeed:    { value: params.cloudSpeed },
    uWind:     { value: new THREE.Vector2(...params.windVec) },
    uSunDir:   { value: new THREE.Vector3(0, 1, 0) },
    uSunColor: { value: new THREE.Color(...params.palette.sun) },
    uAmbient:  { value: new THREE.Color(...params.palette.zenith) },
    uSteps:    { value: tier === "high" ? 64 : 32 },
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }), []);

  // Upscale pass — separate scene + camera, rendered directly to canvas AFTER
  // EffectComposer so the HDR bloom never captures cloud pixels.
  const upscaleScene  = useMemo(() => new THREE.Scene(), []);
  const upscaleCamera = useMemo(() => new THREE.OrthographicCamera(-1, 1, 1, -1, 0, 1), []);

  const upscaleUniforms = useMemo(() => ({
    uCloud:      { value: fbo.texture },
    uTexel:      { value: new THREE.Vector2(1 / fboW, 1 / fboH) },
    uSkyZenith:  { value: new THREE.Color(...params.palette.zenith) },
    uSkyHorizon: { value: new THREE.Color(...params.palette.horizon) },
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }), [fbo, fboW, fboH]);

  // Build cloud FBO quad.
  // NoBlending: the shader outputs straight RGBA directly into the FBO without
  // GL alpha-compositing (which would premultiply RGB by alpha).
  useEffect(() => {
    const geo = new THREE.PlaneGeometry(2, 2);
    const mat = new THREE.ShaderMaterial({
      uniforms: cloudUniforms,
      vertexShader: CLOUDS_VERT,
      fragmentShader: CLOUDS_FRAG,
      depthWrite: false,
      blending: THREE.NoBlending,
    });
    const mesh = new THREE.Mesh(geo, mat);
    rmScene.add(mesh);
    return () => { rmScene.remove(mesh); geo.dispose(); mat.dispose(); };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [rmScene]);

  // Build upscale quad in its own scene (never enters the EffectComposer FBO).
  // Outputs alpha=1 (opaque sky+clouds) replacing the EffectComposer's white output.
  useEffect(() => {
    const geo = new THREE.PlaneGeometry(2, 2);
    const mat = new THREE.ShaderMaterial({
      uniforms: upscaleUniforms,
      vertexShader: CLOUD_UPSCALE_VERT,
      fragmentShader: CLOUD_UPSCALE_FRAG,
      depthWrite: false,
      blending: THREE.NoBlending,
    });
    const mesh = new THREE.Mesh(geo, mat);
    upscaleScene.add(mesh);
    return () => { upscaleScene.remove(mesh); geo.dispose(); mat.dispose(); };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [upscaleScene]);

  // Pass 1 (priority 1): update uniforms + render FBM into cloud FBO
  useFrameThrottle((_, dt) => {
    // Always update sky uniforms so the upscale pass has current palette.
    upscaleUniforms.uSkyZenith.value.setRGB(...params.palette.zenith);
    upscaleUniforms.uSkyHorizon.value.setRGB(...params.palette.horizon);

    if (tier === "low" || params.cloudCover < 0.05) return;

    timeRef.current += dt * (prefersReduced ? 0.25 : 1);
    cloudUniforms.uTime.value    = timeRef.current;
    cloudUniforms.uCover.value  += (params.cloudCover   - cloudUniforms.uCover.value)  * 0.05;
    cloudUniforms.uDensity.value += (params.cloudDensity - cloudUniforms.uDensity.value) * 0.05;
    cloudUniforms.uSpeed.value   = params.cloudSpeed;
    cloudUniforms.uWind.value.set(...params.windVec);
    cloudUniforms.uSunColor.value.setRGB(...params.palette.sun);
    cloudUniforms.uAmbient.value.setRGB(...params.palette.zenith);
    cloudUniforms.uSunDir.value.set(
      Math.cos(params.sunAltitude) * Math.sin(params.sunAzimuth),
      Math.sin(params.sunAltitude),
      -Math.cos(params.sunAltitude) * Math.cos(params.sunAzimuth)
    ).normalize();

    const prevTarget    = gl.getRenderTarget();
    const prevClearClr  = gl.getClearColor(new THREE.Color());
    const prevClearAlph = gl.getClearAlpha();
    gl.setRenderTarget(fbo);
    gl.setClearColor(0x000000, 0);
    gl.clear();
    gl.render(rmScene, rmCamera);
    gl.setClearColor(prevClearClr, prevClearAlph);
    gl.setRenderTarget(prevTarget);
  }, 1);

  // Pass 2 (priority 2): blit sky+clouds onto the canvas AFTER bloom post-processing.
  // Always runs (even with no clouds) to apply the sky gradient that replaces the
  // EffectComposer's opaque output with our transparent-capable composite.
  // autoClear=false preserves the bloom-composited frame for Sun/Moon overlays.
  useFrame((state) => {
    const renderer = state.gl;
    const prevAutoClear = renderer.autoClear;
    renderer.autoClear = false;
    renderer.setRenderTarget(null);
    renderer.render(upscaleScene, upscaleCamera);
    renderer.autoClear = prevAutoClear;
  }, 2);

  return null;
}
