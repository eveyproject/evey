// Vanilla-DOM widget shell: the WebGL canvas host, the discreet "Tonight,
// what are we watching?" CTA button, a hover title label, and the empty-
// library message. Responsive sizing follows the weather-v2/cosmos pattern:
// CSS clamp() on container-query units, falling back to vw.

export interface OverlayHandle {
  canvasHost: HTMLElement;
  setCtaLabel(text: string): void;
  setCtaBusy(busy: boolean): void;
  onCtaClick(cb: () => void): void;
  setTitle(text: string | null): void;
  setEmptyMessage(text: string | null): void;
  dispose(): void;
}

const BASE_CSS = `
.mq-root {
  position: relative;
  width: 100%;
  height: 100vh;
  border-radius: 16px;
  overflow: hidden;
  background: #05070a;
}
.mq-scene { position: absolute; inset: 0; }
.mq-cta {
  position: absolute;
  left: 50%;
  bottom: clamp(10px, 5cqi, 22px);
  transform: translateX(-50%);
  padding: clamp(6px, 2cqi, 10px) clamp(14px, 4cqi, 22px);
  border-radius: 999px;
  border: 1px solid rgba(255,255,255,0.28);
  background: rgba(10,10,14,0.55);
  backdrop-filter: blur(6px);
  color: #f5efe0;
  font-family: system-ui, -apple-system, BlinkMacSystemFont, sans-serif;
  font-size: clamp(11px, 3.2cqi, 14px);
  letter-spacing: 0.02em;
  cursor: pointer;
  transition: background 150ms ease, transform 150ms ease, opacity 150ms ease;
  user-select: none;
}
.mq-cta:hover { background: rgba(30,24,16,0.7); transform: translateX(-50%) scale(1.03); }
.mq-cta[data-busy="true"] { opacity: 0.65; pointer-events: none; }
.mq-title {
  position: absolute;
  left: 50%;
  top: clamp(8px, 4cqi, 16px);
  transform: translateX(-50%);
  max-width: 80%;
  color: #ffffff;
  font-family: system-ui, -apple-system, BlinkMacSystemFont, sans-serif;
  font-size: clamp(11px, 3.4cqi, 15px);
  font-weight: 600;
  letter-spacing: 0.02em;
  text-align: center;
  text-shadow: 0 1px 4px rgba(0,0,0,0.85);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  pointer-events: none;
  opacity: 0;
  transition: opacity 120ms ease;
}
.mq-title.visible { opacity: 1; }
.mq-empty {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0 10%;
  text-align: center;
  color: rgba(245,239,224,0.75);
  font-family: system-ui, -apple-system, BlinkMacSystemFont, sans-serif;
  font-size: clamp(11px, 3.2cqi, 14px);
  pointer-events: none;
}
`;

const FALLBACK_CSS = BASE_CSS.replace(/cqi/g, "vw");

export function createOverlay(parent: HTMLElement): OverlayHandle {
  const style = document.createElement("style");
  const supportsCq = window.CSS?.supports?.("width: 1cqi") === true;
  style.textContent = supportsCq
    ? `.mq-root { container-type: inline-size; }\n${BASE_CSS}`
    : FALLBACK_CSS;
  document.head.appendChild(style);

  const root = document.createElement("div");
  root.className = "mq-root";

  const scene = document.createElement("div");
  scene.className = "mq-scene";
  root.appendChild(scene);

  const title = document.createElement("div");
  title.className = "mq-title";
  root.appendChild(title);

  const cta = document.createElement("button");
  cta.type = "button";
  cta.className = "mq-cta";
  root.appendChild(cta);

  const empty = document.createElement("div");
  empty.className = "mq-empty";
  empty.style.display = "none";
  root.appendChild(empty);

  parent.appendChild(root);

  let ctaCb: (() => void) | null = null;
  const onCtaClick = () => ctaCb?.();
  cta.addEventListener("click", onCtaClick);

  return {
    canvasHost: scene,
    setCtaLabel(text: string) {
      cta.textContent = text;
    },
    setCtaBusy(busy: boolean) {
      cta.dataset.busy = busy ? "true" : "false";
    },
    onCtaClick(cb: () => void) {
      ctaCb = cb;
    },
    setTitle(text: string | null) {
      if (!text) {
        title.classList.remove("visible");
        return;
      }
      title.textContent = text;
      title.classList.add("visible");
    },
    setEmptyMessage(text: string | null) {
      empty.textContent = text ?? "";
      empty.style.display = text ? "flex" : "none";
      cta.style.display = text ? "none" : "block";
    },
    dispose() {
      cta.removeEventListener("click", onCtaClick);
      root.remove();
      style.remove();
    },
  };
}
