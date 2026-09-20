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
import type { GeoCoord, SunTimes, WeatherSnapshot, Units } from "../domain/model/weather";
import type { SceneParams } from "../domain/model/scene";
import { conditionToScene } from "../domain/services/conditionToScene";
import { computeDayCycle } from "../domain/services/dayCycle";
import { mixPalette } from "../domain/services/paletteMixer";
import { moonPhase } from "../domain/services/moonPhase";
import { sunModelForDay } from "../domain/services/sunModel";
import { auroraIntensity } from "../domain/services/aurora";
import { translate } from "../i18n/labels";
import { createOverlay, type OverlayHandle } from "../ui/overlay";

// Leaves room for a full poster row under the widget on a 1080p panel.
const TV_BANNER_HEIGHT = 130;

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
  // Effective location/units: the user's own per-profile residence pref when
  // set, otherwise the admin config default (init). Re-read on every refresh so
  // a change made in profile settings is picked up without a hard reload.
  private city = "";
  private units: Units = "celsius";
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

  // Guards against a host that calls onInit more than once in the same page
  // lifetime (e.g. an iframe soft-reload): without this, a second boot() ran
  // the whole mount sequence again on top of the first - a second WebGL
  // context/canvas the first is never disposed of, duplicate resize/pagehide/
  // visibilitychange listeners, and the refreshTimer/tickTimer handles
  // overwritten so the original intervals kept firing forever, uncancellable.
  private booted = false;

  private boot(init: HostInit): void {
    if (this.booted) return;
    this.booted = true;
    this.init = init;
    this.city = init.city;
    this.units = init.units;
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
  // snapshot stays. The backend seeds this asset with the bundled snapshot at
  // build time, so the request resolves (200) from the very first load: there
  // is never a 404 in the browser console, even before the first live fetch.
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

  // Height negotiation with the host: shorter widget on narrow screens, and a
  // BANNER on a television. A TV row is as wide as a desktop page but the panel
  // has a fraction of the vertical room: 280 there is a third of the screen and
  // pushed the user's library below the fold (2026-09-20). Width cannot tell
  // the two apart, so the host names the surface in its init payload.
  private requestHeight(): void {
    if (this.init?.surface === "tv") {
      this.deps.bridge.resize(TV_BANNER_HEIGHT);
      return;
    }
    const w = window.innerWidth || document.documentElement.clientWidth || 800;
    const h = w < 420 ? 216 : w < 700 ? 248 : 280;
    this.deps.bridge.resize(h);
  }

  // Read the user's per-profile residence pref (city + units) over the admin
  // default. A city change invalidates the cached geocode. Silent fallback to
  // the current values when prefs are unavailable.
  private async loadResidence(): Promise<void> {
    const init = this.init;
    if (!init) return;
    try {
      const [c, u, latS, lonS] = await Promise.all([
        this.deps.bridge.getPref("residence.city"),
        this.deps.bridge.getPref("residence.units"),
        this.deps.bridge.getPref("residence.lat"),
        this.deps.bridge.getPref("residence.lon"),
      ]);
      const nextCity = (c && c.trim()) || init.city;
      const nextUnits: Units = u === "fahrenheit" ? "fahrenheit" : u === "celsius" ? "celsius" : init.units;

      // A disambiguated pick stores exact coordinates: use them directly and
      // never geocode the bare name (which Open-Meteo resolves by population and
      // would land on the wrong homonym, e.g. Brest, Belarus over Brest, France).
      const lat = latS && latS.trim() ? Number(latS) : NaN;
      const lon = lonS && lonS.trim() ? Number(lonS) : NaN;
      if (Number.isFinite(lat) && Number.isFinite(lon)) {
        if (!this.coord || this.coord.lat !== lat || this.coord.lon !== lon || this.coord.name !== nextCity) {
          this.coord = { lat, lon, name: nextCity };
        }
      } else if (nextCity !== this.city || !this.coord) {
        this.coord = null; // legacy name-only pref: (re-)geocode the name
      }
      this.city = nextCity;
      this.units = nextUnits;
    } catch {
      /* keep current city/units */
    }
  }

  private async refresh(): Promise<void> {
    const init = this.init;
    if (!init || this.disposed) return;
    try {
      await this.loadResidence();
      if (this.disposed) return;
      if (!this.coord) {
        this.coord = await this.deps.provider.geocode(this.city, init.locale);
      }
      const res = await this.deps.provider.getCurrent(this.coord, this.units);
      if (this.disposed) return;
      this.snapshot = { ...res.current, cityName: this.coord.name, units: this.units };
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
    const aurora = auroraIntensity(this.coord?.lat ?? 0, cycle, this.params.mood, this.params.cloudCover);
    this.deps.renderer.setTarget({
      params: this.params,
      cycle,
      palette,
      moonPhase: moonPhase(now),
      sunModelId,
      auroraIntensity: aurora,
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
