// Application layer: orchestrates ports around the domain.
//   init -> resolve settings (per-profile prefs override the admin config
//   default) -> push the render target -> periodic re-check (60s) so a
//   change made in profile settings while the widget stays open is picked
//   up without a hard reload.
// There is no external data to fetch and no per-tick recompute: every
// continuous motion (palette drift, veils, caustics, particles) happens
// inside the renderer's own frame loop - see adapters/render/GlRenderer.ts.
// All timers/listeners are owned here and released in dispose().

import type { HostBridge, HostInit } from "../domain/ports/HostBridge";
import type { SceneRenderer } from "../domain/ports/SceneRenderer";
import type { PresetId } from "../domain/model/scene";
import { presetOrDefault } from "../domain/services/presets";
import { createOverlay, type OverlayHandle } from "../ui/overlay";

const SETTINGS_REFRESH_MS = 60 * 1000;
const PALETTE_KEY = "appearance.palette";
const INTENSITY_KEY = "appearance.intensity";
const SPEED_KEY = "appearance.speed";
const INTENSITY_MIN = 0.2;
const INTENSITY_MAX = 1;
const SPEED_MIN = 0.3;
const SPEED_MAX = 2;

interface Deps {
  renderer: SceneRenderer;
  bridge: HostBridge;
}

function clamp(v: number, lo: number, hi: number): number {
  return Math.min(hi, Math.max(lo, v));
}

export class LuminaApp {
  private overlay: OverlayHandle | null = null;
  private init: HostInit | null = null;
  private palette: PresetId = "aurore";
  private intensity = 0.7;
  private speed = 1;

  private refreshTimer = 0;
  private disposed = false;
  // Guards against a host that calls onInit more than once in the same page
  // lifetime (see the identical guard in weather-v2/cosmos): without it, a
  // second boot() re-mounted a whole second WebGL context/canvas on top of
  // the first and overwrote the refreshTimer handle, leaking the original
  // interval forever.
  private booted = false;

  private onPageHide = () => this.dispose();
  private onResize = () => this.requestHeight();
  private onVisible = () => {
    if (!document.hidden) this.push();
  };

  constructor(private deps: Deps) {}

  start(): void {
    this.deps.bridge.onReady((init) => void this.boot(init));
  }

  dispose(): void {
    if (this.disposed) return;
    this.disposed = true;
    clearInterval(this.refreshTimer);
    window.removeEventListener("pagehide", this.onPageHide);
    window.removeEventListener("resize", this.onResize);
    document.removeEventListener("visibilitychange", this.onVisible);
    this.deps.renderer.dispose();
    this.overlay?.dispose();
    this.overlay = null;
  }

  // ---- internals ----------------------------------------------------------

  private async boot(init: HostInit): Promise<void> {
    if (this.booted) return;
    this.booted = true;
    this.init = init;
    this.palette = init.palette;
    this.intensity = init.intensity;
    this.speed = init.speed;

    document.documentElement.style.background = "transparent";
    document.body.style.margin = "0";
    document.body.style.background = "transparent";
    document.body.style.overflow = "hidden";

    this.overlay = createOverlay(document.body);
    this.deps.renderer.mount(this.overlay.canvasHost);
    this.overlay.onPointer((nx, ny) => this.deps.renderer.setPointer(nx, ny));

    this.requestHeight();
    window.addEventListener("resize", this.onResize);
    window.addEventListener("pagehide", this.onPageHide);
    document.addEventListener("visibilitychange", this.onVisible);

    this.push(); // paint immediately with the admin/init defaults

    await this.loadSettings();
    if (this.disposed) return;
    this.push();
    this.refreshTimer = window.setInterval(() => void this.refreshAndPush(), SETTINGS_REFRESH_MS);
  }

  private async refreshAndPush(): Promise<void> {
    await this.loadSettings();
    if (this.disposed) return;
    this.push();
  }

  // Read the user's per-profile appearance prefs over the admin default.
  // Silent fallback to the current values when prefs are unavailable.
  private async loadSettings(): Promise<void> {
    const init = this.init;
    if (!init) return;
    try {
      const [p, i, s] = await Promise.all([
        this.deps.bridge.getPref(PALETTE_KEY),
        this.deps.bridge.getPref(INTENSITY_KEY),
        this.deps.bridge.getPref(SPEED_KEY),
      ]);
      this.palette = p !== null ? presetOrDefault(p) : init.palette;
      const iNum = i !== null ? Number(i) : NaN;
      this.intensity = Number.isFinite(iNum) ? clamp(iNum, INTENSITY_MIN, INTENSITY_MAX) : init.intensity;
      const sNum = s !== null ? Number(s) : NaN;
      this.speed = Number.isFinite(sNum) ? clamp(sNum, SPEED_MIN, SPEED_MAX) : init.speed;
    } catch {
      /* keep current settings */
    }
  }

  // Height negotiation with the host: a taller ambient canvas on wide
  // screens, shorter on narrow ones.
  private requestHeight(): void {
    const w = window.innerWidth || document.documentElement.clientWidth || 800;
    const h = w < 420 ? 260 : w < 700 ? 340 : 420;
    this.deps.bridge.resize(h);
  }

  private push(): void {
    if (this.disposed || !this.overlay) return;
    this.deps.renderer.setTarget({ paletteId: this.palette, intensity: this.intensity, speedMul: this.speed });
  }
}
