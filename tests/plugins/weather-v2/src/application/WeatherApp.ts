// Application layer: orchestrates ports around the domain.
//   init -> geocode -> fetch weather -> map to scene -> push to renderer
//   then two independent clocks:
//     - weather refresh every 15 min (with bounded retry on failure)
//     - day-cycle tick every 5 s: the sun/moon positions, golden hour and
//       day/night flip evolve continuously while the page stays open
//       (v1 computed the sun position exactly once at load). The renderer
//       smooths every step, so motion is seamless.
// All timers/listeners are owned here and released in dispose().

import type { WeatherProvider } from "../domain/ports/WeatherProvider";
import type { HostBridge, HostInit } from "../domain/ports/HostBridge";
import type { SceneRenderer } from "../domain/ports/SceneRenderer";
import type { GeoCoord, SunTimes, WeatherSnapshot } from "../domain/model/weather";
import type { SceneParams } from "../domain/model/scene";
import { conditionToScene } from "../domain/services/conditionToScene";
import { computeDayCycle } from "../domain/services/dayCycle";
import { mixPalette } from "../domain/services/paletteMixer";
import { moonPhase } from "../domain/services/moonPhase";
import { sunModelForDay } from "../domain/services/sunModel";
import { translate } from "../i18n/labels";
import { createOverlay, type OverlayHandle } from "../ui/overlay";

const WEATHER_REFRESH_MS = 15 * 60 * 1000;
const CYCLE_TICK_MS = 5 * 1000;
const RETRY_DELAYS_MS = [5_000, 15_000, 60_000];

// Calm placeholder while loading / on error.
const IDLE_PARAMS: SceneParams = {
  cloudCover: 0.12,
  cloudDensity: 0.15,
  cloudSpeed: 0.1,
  precipitation: "none",
  precipIntensity: 0,
  thunderRate: 0,
  fog: 0,
  mood: "clear",
  label: "Loading…",
  labelKey: "loading",
};

interface Deps {
  provider: WeatherProvider;
  renderer: SceneRenderer;
  bridge: HostBridge;
}

export class WeatherApp {
  private overlay: OverlayHandle | null = null;
  private init: HostInit | null = null;
  private coord: GeoCoord | null = null;
  private snapshot: WeatherSnapshot | null = null;
  private sunTimes: SunTimes | null = null;
  private params: SceneParams = IDLE_PARAMS;
  private failed = false;
  private lastSunModel: string | null = null;

  private refreshTimer = 0;
  private tickTimer = 0;
  private retryTimer = 0;
  private retryCount = 0;
  private disposed = false;

  private onPageHide = () => this.dispose();
  private onResize = () => this.requestHeight();
  private onVisible = () => {
    if (!document.hidden) this.push();
  };

  constructor(private deps: Deps) {}

  start(): void {
    this.deps.bridge.onReady((init) => this.boot(init));
  }

  dispose(): void {
    if (this.disposed) return;
    this.disposed = true;
    clearInterval(this.refreshTimer);
    clearInterval(this.tickTimer);
    clearTimeout(this.retryTimer);
    window.removeEventListener("pagehide", this.onPageHide);
    window.removeEventListener("resize", this.onResize);
    document.removeEventListener("visibilitychange", this.onVisible);
    this.deps.renderer.dispose();
    this.overlay?.dispose();
    this.overlay = null;
  }

  // ---- internals ----------------------------------------------------------

  private boot(init: HostInit): void {
    this.init = init;
    document.documentElement.style.background = "transparent";
    document.body.style.margin = "0";
    document.body.style.background = "transparent";
    document.body.style.overflow = "hidden";

    this.overlay = createOverlay(document.body);
    this.deps.renderer.mount(this.overlay.canvasHost);

    this.requestHeight();
    window.addEventListener("resize", this.onResize);
    window.addEventListener("pagehide", this.onPageHide);
    document.addEventListener("visibilitychange", this.onVisible);

    this.push(); // idle scene right away
    void this.syncLiveSun(); // upgrade to the backend-fetched live image if present

    void this.refresh();
    this.refreshTimer = window.setInterval(() => void this.refresh(), WEATHER_REFRESH_MS);
    this.tickTimer = window.setInterval(() => this.push(), CYCLE_TICK_MS);
  }

  // Ask the host for the plugin's own live sun asset (written by the backend)
  // and upgrade the texture. Silent no-op when unavailable - the bundled
  // snapshot stays. Concurrency-guarded; cheap enough to call on refresh.
  private liveSunBusy = false;
  private async syncLiveSun(): Promise<void> {
    if (this.disposed || this.liveSunBusy) return;
    this.liveSunBusy = true;
    try {
      const id = sunModelForDay(Date.now()).id;
      const dataUrl = await this.deps.bridge.readAsset(`sun-live-${id}.jpg`);
      if (!this.disposed && dataUrl) this.deps.renderer.setSunImageData(dataUrl);
    } finally {
      this.liveSunBusy = false;
    }
  }

  // Height negotiation with the host: shorter widget on narrow screens.
  private requestHeight(): void {
    const w = window.innerWidth || document.documentElement.clientWidth || 800;
    const h = w < 420 ? 216 : w < 700 ? 248 : 280;
    this.deps.bridge.resize(h);
  }

  private async refresh(): Promise<void> {
    const init = this.init;
    if (!init || this.disposed) return;
    try {
      if (!this.coord) {
        this.coord = await this.deps.provider.geocode(init.city, init.locale);
      }
      const res = await this.deps.provider.getCurrent(this.coord, init.units);
      if (this.disposed) return;
      this.snapshot = { ...res.current, cityName: this.coord.name, units: init.units };
      this.sunTimes = res.sunTimes;
      this.params = conditionToScene(this.snapshot);
      this.failed = false;
      this.retryCount = 0;
      this.push();
      void this.syncLiveSun(); // refresh the live sun image periodically too
    } catch {
      if (this.disposed) return;
      this.failed = true;
      this.push();
      if (this.retryCount < RETRY_DELAYS_MS.length) {
        const delay = RETRY_DELAYS_MS[this.retryCount++];
        clearTimeout(this.retryTimer);
        this.retryTimer = window.setTimeout(() => void this.refresh(), delay);
      }
    }
  }

  // Recompute time-dependent state and hand everything to the renderer.
  private push(): void {
    if (this.disposed || !this.overlay) return;
    const now = Date.now();
    const cycle = computeDayCycle(now, this.sunTimes, this.snapshot?.isDay ?? true);
    const palette = mixPalette(this.params.mood, cycle);
    const sunModelId = sunModelForDay(now).id;
    this.deps.renderer.setTarget({
      params: this.params,
      cycle,
      palette,
      moonPhase: moonPhase(now),
      sunModelId,
    });
    // Day rolled over to a new sun skin: fetch that skin's live image now
    // rather than waiting for the next weather refresh.
    if (this.lastSunModel !== null && this.lastSunModel !== sunModelId) {
      void this.syncLiveSun();
    }
    this.lastSunModel = sunModelId;

    const locale = this.init?.locale;
    if (this.failed && !this.snapshot) {
      this.overlay.setData("--", translate("unavailable", locale));
      return;
    }
    if (!this.snapshot) {
      this.overlay.setData("--", translate("loading", locale));
      return;
    }
    const unit = this.snapshot.units === "fahrenheit" ? "°F" : "°C";
    const tempLabel = `${Math.round(this.snapshot.temperatureDisplay)}${unit}`;
    const condition = translate(this.params.labelKey, locale);
    const stale = this.failed ? " !" : "";
    this.overlay.setData(tempLabel, `${this.snapshot.cityName} · ${condition}${stale}`);
  }
}
