// Vanilla-DOM widget shell: the WebGL canvas host, the "tonight" info banner,
// and a small floating label that appears near a constellation's centroid on
// hover (mouse) or tap (touch). Responsive sizing follows the weather-v2
// pattern: CSS clamp() on container-query units, falling back to vw.

export interface OverlayHandle {
  canvasHost: HTMLElement;
  setBanner(text: string): void;
  // Shows a constellation label at normalized scene coordinates (-1..1,
  // -1..1, y up) or hides it when key is null.
  setHoverLabel(text: string | null, sceneX: number, sceneY: number): void;
  // Registers the callback used to resolve a pointer position (CSS pixels,
  // origin at the scene center) into "nearest constellation label" lookups.
  onPointer(cb: (cssX: number, cssY: number) => void, onLeave: () => void): void;
  dispose(): void;
}

const BASE_CSS = `
.cos-root {
  position: relative;
  width: 100%;
  height: 100vh;
  border-radius: 16px;
  overflow: hidden;
  background: #05070f;
}
.cos-scene { position: absolute; inset: 0; cursor: default; }
.cos-banner {
  position: absolute;
  left: clamp(10px, 5cqi, 20px);
  bottom: clamp(8px, 4cqi, 16px);
  max-width: 85%;
  color: #f2f4ff;
  font-family: system-ui, -apple-system, BlinkMacSystemFont, sans-serif;
  font-size: clamp(11px, 3cqi, 14px);
  letter-spacing: 0.01em;
  text-shadow: 0 1px 4px rgba(0,0,0,0.7);
  pointer-events: none;
  user-select: none;
}
.cos-label {
  position: absolute;
  transform: translate(-50%, -100%);
  color: #ffffff;
  font-family: system-ui, -apple-system, BlinkMacSystemFont, sans-serif;
  font-size: clamp(10px, 2.6cqi, 13px);
  font-weight: 600;
  letter-spacing: 0.04em;
  text-shadow: 0 1px 4px rgba(0,0,0,0.85);
  pointer-events: none;
  white-space: nowrap;
  opacity: 0;
  transition: opacity 120ms ease;
}
.cos-label.visible { opacity: 1; }
`;

const FALLBACK_CSS = BASE_CSS.replace(/cqi/g, "vw");

export function createOverlay(parent: HTMLElement): OverlayHandle {
  const style = document.createElement("style");
  const supportsCq = window.CSS?.supports?.("width: 1cqi") === true;
  style.textContent = supportsCq
    ? `.cos-root { container-type: inline-size; }\n${BASE_CSS}`
    : FALLBACK_CSS;
  document.head.appendChild(style);

  const root = document.createElement("div");
  root.className = "cos-root";

  const scene = document.createElement("div");
  scene.className = "cos-scene";
  root.appendChild(scene);

  const banner = document.createElement("div");
  banner.className = "cos-banner";
  root.appendChild(banner);

  const label = document.createElement("div");
  label.className = "cos-label";
  root.appendChild(label);

  parent.appendChild(root);

  const onPointerMove = (e: PointerEvent) => {
    const rect = scene.getBoundingClientRect();
    pointerCb?.(e.clientX - rect.left - rect.width / 2, e.clientY - rect.top - rect.height / 2);
  };
  const onPointerLeave = () => leaveCb?.();
  let pointerCb: ((x: number, y: number) => void) | null = null;
  let leaveCb: (() => void) | null = null;

  scene.addEventListener("pointermove", onPointerMove);
  scene.addEventListener("pointerdown", onPointerMove);
  scene.addEventListener("pointerleave", onPointerLeave);

  return {
    canvasHost: scene,
    setBanner(text: string) {
      banner.textContent = text;
    },
    setHoverLabel(text, sceneX, sceneY) {
      if (!text) {
        label.classList.remove("visible");
        return;
      }
      const rect = scene.getBoundingClientRect();
      const half = Math.min(rect.width, rect.height) / 2;
      label.style.left = `${rect.width / 2 + sceneX * half}px`;
      label.style.top = `${rect.height / 2 + sceneY * half - 10}px`;
      label.textContent = text;
      label.classList.add("visible");
    },
    onPointer(cb, onLeave) {
      pointerCb = cb;
      leaveCb = onLeave;
    },
    dispose() {
      scene.removeEventListener("pointermove", onPointerMove);
      scene.removeEventListener("pointerdown", onPointerMove);
      scene.removeEventListener("pointerleave", onPointerLeave);
      root.remove();
      style.remove();
    },
  };
}
