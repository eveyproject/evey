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
  // The hero card: a big, readable blow-up of one poster over a dimmed wall.
  // "preview" = pointer hover (non-interactive, wall stays hoverable beneath);
  // "pick" = the elected roulette winner (interactive: a live countdown +
  // "watch now" button). Same fixed centred position in both modes, so moving
  // between posters only swaps the image/title - no jumpy re-anchoring.
  showHero(mode: "preview" | "pick", posterUrl: string, title: string): void;
  setHeroCountdown(text: string | null): void;
  setHeroWatchLabel(text: string): void;
  onHeroWatch(cb: () => void): void;
  hideHero(): void;
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
  /* Above the hero veil (z-index 3) so "reroll" stays clickable while the
     elected pick is held. */
  z-index: 4;
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
.mq-hero {
  position: absolute;
  inset: 0;
  z-index: 3;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: clamp(14px, 4cqi, 40px);
  padding: clamp(10px, 3cqi, 26px) clamp(14px, 5cqi, 60px);
  opacity: 0;
  visibility: hidden;
  pointer-events: none;
  transition: opacity 200ms ease, visibility 0s linear 200ms;
}
.mq-hero.visible { opacity: 1; visibility: visible; transition: opacity 200ms ease; }
/* The dimming veil sits behind the card so the blown-up poster reads clearly. */
.mq-hero::before {
  content: "";
  position: absolute;
  inset: 0;
  background: radial-gradient(ellipse at center, rgba(4,5,9,0.62), rgba(3,4,7,0.9));
  backdrop-filter: blur(4px);
  opacity: 0;
  transition: opacity 200ms ease;
}
.mq-hero.preview::before { opacity: 0.7; }
.mq-hero.pick::before { opacity: 1; }
/* preview follows the mouse: never eat pointer events, keep the wall hoverable.
   pick is a deliberate stop: the card + its button become interactive. */
.mq-hero.pick { pointer-events: auto; }
.mq-hero-card {
  position: relative;
  display: flex;
  align-items: center;
  gap: clamp(14px, 4cqi, 40px);
  max-width: 92%;
}
.mq-hero-poster {
  flex: 0 0 auto;
  height: clamp(150px, 70vh, 300px);
  aspect-ratio: 2 / 3;
  object-fit: cover;
  border-radius: 12px;
  background: #0c0f14;
  box-shadow: 0 18px 48px rgba(0,0,0,0.7), 0 0 0 1px rgba(255,255,255,0.12), 0 0 60px rgba(120,150,255,0.12);
  transform: scale(0.86) translateY(6px);
  transition: transform 260ms cubic-bezier(0.22, 1, 0.36, 1);
}
.mq-hero.visible .mq-hero-poster { transform: scale(1) translateY(0); }
.mq-hero-info {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: clamp(6px, 1.6cqi, 14px);
  min-width: 0;
  max-width: min(46%, 420px);
}
.mq-hero-title {
  color: #fff;
  font-family: system-ui, -apple-system, BlinkMacSystemFont, sans-serif;
  font-size: clamp(16px, 4.6cqi, 30px);
  font-weight: 700;
  line-height: 1.12;
  letter-spacing: 0.01em;
  text-shadow: 0 2px 10px rgba(0,0,0,0.85);
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.mq-hero-count {
  display: none;
  align-items: center;
  gap: 8px;
  color: rgba(245,239,224,0.82);
  font-family: system-ui, -apple-system, BlinkMacSystemFont, sans-serif;
  font-size: clamp(11px, 3cqi, 15px);
  font-variant-numeric: tabular-nums;
}
.mq-hero-count::before {
  content: "";
  width: 7px; height: 7px;
  border-radius: 50%;
  background: #8ab4ff;
  box-shadow: 0 0 8px #8ab4ff;
  animation: mq-pulse 1s ease-in-out infinite;
}
@keyframes mq-pulse { 0%,100% { opacity: 1; } 50% { opacity: 0.35; } }
.mq-hero.pick .mq-hero-count { display: flex; }
.mq-hero-watch {
  display: none;
  margin-top: clamp(2px, 1cqi, 8px);
  padding: clamp(8px, 2.2cqi, 12px) clamp(16px, 4.5cqi, 26px);
  border-radius: 999px;
  border: 1px solid rgba(255,255,255,0.16);
  background: linear-gradient(135deg, #f5efe0, #d8cbb0);
  color: #141210;
  font-family: system-ui, -apple-system, BlinkMacSystemFont, sans-serif;
  font-size: clamp(12px, 3.4cqi, 16px);
  font-weight: 700;
  letter-spacing: 0.01em;
  cursor: pointer;
  transition: transform 140ms ease, box-shadow 140ms ease;
  box-shadow: 0 6px 20px rgba(0,0,0,0.4);
}
.mq-hero-watch:hover { transform: translateY(-1px) scale(1.03); box-shadow: 0 10px 26px rgba(0,0,0,0.5); }
.mq-hero.pick .mq-hero-watch { display: inline-block; }
@media (max-width: 520px) {
  .mq-hero-card { flex-direction: column; gap: 10px; }
  .mq-hero-poster { height: clamp(120px, 46vh, 190px); }
  .mq-hero-info { align-items: center; text-align: center; max-width: 88%; }
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

  const hero = document.createElement("div");
  hero.className = "mq-hero";
  const heroCard = document.createElement("div");
  heroCard.className = "mq-hero-card";
  const heroPoster = document.createElement("img");
  heroPoster.className = "mq-hero-poster";
  heroPoster.alt = "";
  heroPoster.decoding = "async";
  const heroInfo = document.createElement("div");
  heroInfo.className = "mq-hero-info";
  const heroTitle = document.createElement("div");
  heroTitle.className = "mq-hero-title";
  const heroCount = document.createElement("div");
  heroCount.className = "mq-hero-count";
  const heroWatch = document.createElement("button");
  heroWatch.type = "button";
  heroWatch.className = "mq-hero-watch";
  heroInfo.appendChild(heroTitle);
  heroInfo.appendChild(heroCount);
  heroInfo.appendChild(heroWatch);
  heroCard.appendChild(heroPoster);
  heroCard.appendChild(heroInfo);
  hero.appendChild(heroCard);
  root.appendChild(hero);

  parent.appendChild(root);

  let ctaCb: (() => void) | null = null;
  const onCtaClick = () => ctaCb?.();
  cta.addEventListener("click", onCtaClick);

  let heroWatchCb: (() => void) | null = null;
  const onHeroWatchClick = () => heroWatchCb?.();
  heroWatch.addEventListener("click", onHeroWatchClick);

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
    showHero(mode: "preview" | "pick", posterUrl: string, title: string) {
      heroTitle.textContent = title;
      // Only swap the src when it actually changed, so sweeping across posters
      // that share a (missing) poster doesn't retrigger the load/flash.
      if (posterUrl && heroPoster.src !== posterUrl) heroPoster.src = posterUrl;
      heroPoster.style.visibility = posterUrl ? "visible" : "hidden";
      if (mode === "pick") {
        hero.classList.add("pick");
        hero.classList.remove("preview");
      } else {
        hero.classList.add("preview");
        hero.classList.remove("pick");
        heroCount.textContent = "";
      }
      hero.classList.add("visible");
    },
    setHeroCountdown(text: string | null) {
      heroCount.textContent = text ?? "";
    },
    setHeroWatchLabel(text: string) {
      heroWatch.textContent = text;
    },
    onHeroWatch(cb: () => void) {
      heroWatchCb = cb;
    },
    hideHero() {
      hero.classList.remove("visible", "pick", "preview");
      heroCount.textContent = "";
    },
    dispose() {
      cta.removeEventListener("click", onCtaClick);
      heroWatch.removeEventListener("click", onHeroWatchClick);
      root.remove();
      style.remove();
    },
  };
}
