// Vanilla-DOM widget shell: just the WebGL canvas host. Lumina shows no
// text/data overlay (pure ambient wallpaper, no library or weather data), so
// this is the thinnest of the three plugins' overlays - its only other job
// is turning raw pointer movement into a normalized (-1..1) offset from the
// widget center for the renderer's lazy particle parallax.

export interface OverlayHandle {
  canvasHost: HTMLElement;
  // Registers the callback fed with normalized pointer coordinates on every
  // move, and (0, 0) on pointer leave so the parallax eases back to rest.
  onPointer(cb: (nx: number, ny: number) => void): void;
  dispose(): void;
}

const BASE_CSS = `
.lum-root {
  position: relative;
  width: 100%;
  height: 100vh;
  border-radius: 16px;
  overflow: hidden;
  background: #05030a;
}
.lum-scene { position: absolute; inset: 0; cursor: default; }
`;

function clamp(v: number, lo: number, hi: number): number {
  return Math.min(hi, Math.max(lo, v));
}

export function createOverlay(parent: HTMLElement): OverlayHandle {
  const style = document.createElement("style");
  style.textContent = BASE_CSS;
  document.head.appendChild(style);

  const root = document.createElement("div");
  root.className = "lum-root";

  const scene = document.createElement("div");
  scene.className = "lum-scene";
  root.appendChild(scene);

  parent.appendChild(root);

  let pointerCb: ((nx: number, ny: number) => void) | null = null;

  const onPointerMove = (e: PointerEvent) => {
    const rect = scene.getBoundingClientRect();
    const halfW = rect.width / 2 || 1;
    const halfH = rect.height / 2 || 1;
    const nx = clamp((e.clientX - rect.left - halfW) / halfW, -1, 1);
    const ny = clamp((e.clientY - rect.top - halfH) / halfH, -1, 1);
    pointerCb?.(nx, ny);
  };
  const onPointerLeave = () => pointerCb?.(0, 0);

  scene.addEventListener("pointermove", onPointerMove);
  scene.addEventListener("pointerleave", onPointerLeave);

  return {
    canvasHost: scene,
    onPointer(cb) {
      pointerCb = cb;
    },
    dispose() {
      scene.removeEventListener("pointermove", onPointerMove);
      scene.removeEventListener("pointerleave", onPointerLeave);
      root.remove();
      style.remove();
    },
  };
}
