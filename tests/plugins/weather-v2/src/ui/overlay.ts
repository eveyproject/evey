// Vanilla-DOM widget shell + HTML overlay (temperature, city, condition).
// Responsive rules live here as CSS clamp() on iframe-viewport units, so the
// text scales down cleanly on narrow (mobile) hosts instead of the fixed
// 44px of v1. The widget height itself is negotiated by the application via
// HostBridge.resize().

export interface OverlayHandle {
  canvasHost: HTMLElement;
  setData(tempLabel: string, subLabel: string): void;
  dispose(): void;
}

const BASE_CSS = `
.wx2-root {
  position: relative;
  width: 100%;
  height: 100vh;
  border-radius: 16px;
  overflow: hidden;
  background: transparent;
}
.wx2-scene { position: absolute; inset: 0; }
.wx2-overlay {
  position: absolute;
  top: clamp(8px, 4cqi, 16px);
  right: clamp(10px, 5cqi, 20px);
  max-width: 70%;
  text-align: right;
  color: #ffffff;
  font-family: system-ui, -apple-system, BlinkMacSystemFont, sans-serif;
  pointer-events: none;
  user-select: none;
}
.wx2-temp {
  font-size: clamp(28px, 12cqi, 46px);
  font-weight: 600;
  letter-spacing: -0.02em;
  line-height: 1;
  font-variant-numeric: tabular-nums;
  text-shadow: 0 2px 8px rgba(0,0,0,0.55), 0 1px 3px rgba(0,0,0,0.35);
}
.wx2-sub {
  font-size: clamp(10px, 3.2cqi, 13px);
  opacity: 0.88;
  margin-top: 6px;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  text-shadow: 0 1px 4px rgba(0,0,0,0.60);
}
`;

// cqi needs a container; fall back to vw when unsupported (iframe = viewport
// here, so vw is equivalent).
const FALLBACK_CSS = BASE_CSS.replace(/cqi/g, "vw");

export function createOverlay(parent: HTMLElement): OverlayHandle {
  const style = document.createElement("style");
  const supportsCq = window.CSS?.supports?.("width: 1cqi") === true;
  style.textContent = supportsCq
    ? `.wx2-root { container-type: inline-size; }\n${BASE_CSS}`
    : FALLBACK_CSS;
  document.head.appendChild(style);

  const root = document.createElement("div");
  root.className = "wx2-root";

  const scene = document.createElement("div");
  scene.className = "wx2-scene";
  root.appendChild(scene);

  const overlay = document.createElement("div");
  overlay.className = "wx2-overlay";
  const temp = document.createElement("div");
  temp.className = "wx2-temp";
  temp.textContent = "--";
  const sub = document.createElement("div");
  sub.className = "wx2-sub";
  sub.textContent = "";
  overlay.appendChild(temp);
  overlay.appendChild(sub);
  root.appendChild(overlay);

  parent.appendChild(root);

  return {
    canvasHost: scene,
    setData(tempLabel: string, subLabel: string) {
      temp.textContent = tempLabel;
      sub.textContent = subLabel;
    },
    dispose() {
      root.remove();
      style.remove();
    },
  };
}
