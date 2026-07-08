// Application layer: orchestrates ports around the domain.
//   init -> resolve location (per-profile pref, else admin config, else the
//   embedded gazetteer default) -> tick loop: build the full SkyScene for
//   "now" and push it to the renderer + banner.
// Real sky positions barely move between ticks (stars ~0.25deg/min), so a
// 60s recompute cadence is imperceptible - the shader alone carries the
// continuous "wow" motion (twinkle, Milky Way drift, shooting stars).
// All timers/listeners are owned here and released in dispose().

import type { HostBridge, HostInit } from "../domain/ports/HostBridge";
import type { SceneRenderer } from "../domain/ports/SceneRenderer";
import type { ConstellationLabel } from "../domain/model/scene";
import { buildSkyScene } from "../domain/services/sceneBuilder";
import { resolveCity } from "../domain/data/cityGazetteer";
import { translate } from "../i18n/labels";
import { createOverlay, type OverlayHandle } from "../ui/overlay";

const TICK_MS = 60 * 1000;
const CITY_KEY = "residence.city";
const HOVER_THRESHOLD = 0.14; // normalized scene units

interface Deps {
  renderer: SceneRenderer;
  bridge: HostBridge;
}

export class CosmosApp {
  private overlay: OverlayHandle | null = null;
  private init: HostInit | null = null;
  private city = "";
  private coord = { latDeg: 48.8566, lonDeg: 2.3522 };
  private labels: ConstellationLabel[] = [];

  private tickTimer = 0;
  private disposed = false;

  private onPageHide = () => this.dispose();
  private onResize = () => this.requestHeight();
  private onVisible = () => {
    if (!document.hidden) this.tick();
  };

  constructor(private deps: Deps) {}

  start(): void {
    this.deps.bridge.onReady((init) => void this.boot(init));
  }

  dispose(): void {
    if (this.disposed) return;
    this.disposed = true;
    clearInterval(this.tickTimer);
    window.removeEventListener("pagehide", this.onPageHide);
    window.removeEventListener("resize", this.onResize);
    document.removeEventListener("visibilitychange", this.onVisible);
    this.deps.renderer.dispose();
    this.overlay?.dispose();
    this.overlay = null;
  }

  // ---- internals --------------------------------------------------------

  private async boot(init: HostInit): Promise<void> {
    this.init = init;
    this.city = init.city;

    document.documentElement.style.background = "transparent";
    document.body.style.margin = "0";
    document.body.style.background = "transparent";
    document.body.style.overflow = "hidden";

    this.overlay = createOverlay(document.body);
    this.deps.renderer.mount(this.overlay.canvasHost);
    this.overlay.onPointer(
      (x, y) => this.handlePointer(x, y),
      () => this.overlay?.setHoverLabel(null, 0, 0),
    );

    this.requestHeight();
    window.addEventListener("resize", this.onResize);
    window.addEventListener("pagehide", this.onPageHide);
    document.addEventListener("visibilitychange", this.onVisible);

    await this.loadResidence();
    if (this.disposed) return;
    this.tick();
    this.tickTimer = window.setInterval(() => void this.refreshAndTick(), TICK_MS);
  }

  private async refreshAndTick(): Promise<void> {
    await this.loadResidence();
    if (this.disposed) return;
    this.tick();
  }

  private async loadResidence(): Promise<void> {
    const init = this.init;
    if (!init) return;
    try {
      const c = await this.deps.bridge.getPref(CITY_KEY);
      this.city = (c && c.trim()) || init.city;
    } catch {
      /* keep current city */
    }
    const resolved = resolveCity(this.city);
    this.coord = { latDeg: resolved.lat, lonDeg: resolved.lon };
  }

  private requestHeight(): void {
    const w = window.innerWidth || document.documentElement.clientWidth || 800;
    const h = w < 420 ? 260 : w < 700 ? 320 : 380;
    this.deps.bridge.resize(h);
  }

  private tick(): void {
    if (this.disposed || !this.overlay) return;
    const scene = buildSkyScene(Date.now(), this.coord);
    this.labels = scene.constellationLabels;
    this.deps.renderer.setTarget(scene);

    const locale = this.init?.locale;
    this.overlay.setBanner(this.buildBannerText(scene.banner, locale));
  }

  private buildBannerText(
    banner: { moonPhaseKey: string; daysToFullMoon: number; planetName: string | null; planetCompass: string | null },
    locale: string | undefined,
  ): string {
    const tonight = translate("banner_tonight", locale);
    const planetPart = banner.planetName
      ? `${translate(`planet_${banner.planetName}`, locale)} ${translate("banner_planet_visible", locale)} ${banner.planetCompass}`
      : translate("banner_no_planet", locale);

    const days = Math.round(banner.daysToFullMoon);
    const moonPart =
      days <= 0
        ? translate("phase_full", locale)
        : `${translate("banner_full_moon_in", locale)} ${days}${translate("banner_days_short", locale)}`;

    return `${tonight} : ${planetPart} · ${moonPart}`;
  }

  private handlePointer(cssX: number, cssY: number): void {
    const host = this.overlay?.canvasHost;
    if (!host || this.labels.length === 0) return;
    const rect = host.getBoundingClientRect();
    const half = Math.min(rect.width, rect.height) / 2;
    if (half <= 0) return;
    const nx = cssX / half;
    const ny = -cssY / half; // CSS y-down -> scene y-up

    let best: ConstellationLabel | null = null;
    let bestDist = HOVER_THRESHOLD;
    for (const label of this.labels) {
      if (!label.visible) continue;
      const d = Math.hypot(label.x - nx, label.y - ny);
      if (d < bestDist) {
        bestDist = d;
        best = label;
      }
    }

    if (best) {
      this.overlay?.setHoverLabel(translate(best.labelKey, this.init?.locale), best.x, best.y);
    } else {
      this.overlay?.setHoverLabel(null, 0, 0);
    }
  }
}
