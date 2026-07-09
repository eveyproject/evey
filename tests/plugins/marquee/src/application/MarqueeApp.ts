// Application layer: orchestrates ports around the domain.
//   init -> load the caller's library -> pick a wall-sized, seeded sample of
//   posters -> lay out the wall -> push it to the renderer.
// The renderer owns per-frame timing/pointer-picking; this layer only reacts
// to its activate/hover callbacks and drives the roulette moment.
// All timers/listeners are owned here and released in dispose().

import type { HostBridge, HostInit } from "../domain/ports/HostBridge";
import type { SceneRenderer } from "../domain/ports/SceneRenderer";
import type { MediaSummary } from "../domain/model/media";
import { buildWallLayout } from "../domain/services/wallLayout";
import { selectWallPosters } from "../domain/services/posterSelection";
import { planRouletteSpin } from "../domain/services/roulette";
import { WALL_GEOMETRY } from "../domain/services/wallGeometry";
import { translate } from "../i18n/labels";
import { createOverlay, type OverlayHandle } from "../ui/overlay";

// Long-open dashboard tabs still occasionally pick up newly added titles.
const LIBRARY_REFRESH_MS = 30 * 60 * 1000;

interface Deps {
  renderer: SceneRenderer;
  bridge: HostBridge;
}

export class MarqueeApp {
  private overlay: OverlayHandle | null = null;
  private init: HostInit | null = null;
  private posters: MediaSummary[] = [];
  private spinning = false;

  private refreshTimer = 0;
  private disposed = false;
  private booted = false;

  private onPageHide = () => this.dispose();
  private onResize = () => this.requestHeight();
  private onVisible = () => {
    if (!document.hidden) void this.loadLibrary();
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

    document.documentElement.style.background = "transparent";
    document.body.style.margin = "0";
    document.body.style.background = "transparent";
    document.body.style.overflow = "hidden";

    this.overlay = createOverlay(document.body);
    this.deps.renderer.mount(this.overlay.canvasHost);
    this.overlay.setCtaLabel(translate("cta", init.locale));
    this.overlay.onCtaClick(() => this.spin());
    this.deps.renderer.onActivate((index) => this.activate(index));
    this.deps.renderer.onHover((index) => this.hover(index));

    this.requestHeight();
    window.addEventListener("resize", this.onResize);
    window.addEventListener("pagehide", this.onPageHide);
    document.addEventListener("visibilitychange", this.onVisible);

    await this.loadLibrary();
    if (this.disposed) return;
    this.refreshTimer = window.setInterval(() => void this.loadLibrary(), LIBRARY_REFRESH_MS);
  }

  private requestHeight(): void {
    const w = window.innerWidth || document.documentElement.clientWidth || 800;
    const h = w < 420 ? 220 : w < 700 ? 280 : 340;
    this.deps.bridge.resize(h);
  }

  private async loadLibrary(): Promise<void> {
    if (this.disposed || !this.overlay) return;
    const items = await this.deps.bridge.listMedia();
    if (this.disposed) return;

    const capacity = this.deps.renderer.getCapacity();
    // Reseeded on every (re)load so a periodic refresh brings a fresh mix
    // without needing the user to reload the page.
    const seed = (Date.now() ^ (items.length * 2654435761)) >>> 0;
    this.posters = selectWallPosters(items, capacity.posterCount, seed);

    if (this.posters.length === 0) {
      this.overlay.setEmptyMessage(translate("empty", this.init?.locale));
      return;
    }
    this.overlay.setEmptyMessage(null);

    const layout = buildWallLayout({
      count: this.posters.length,
      rows: capacity.rows,
      colSpacing: WALL_GEOMETRY.colSpacing,
      rowRise: WALL_GEOMETRY.rowRise,
      rowScaleFalloff: WALL_GEOMETRY.rowScaleFalloff,
      rowStaggerFraction: WALL_GEOMETRY.rowStaggerFraction,
    });

    this.deps.renderer.setWall({
      layout,
      posters: this.posters.map((p) => ({ id: p.id, title: p.title, posterUrl: p.posterUrl })),
    });
  }

  // "Ce soir, on regarde ?" - spins the wheel, then the renderer's onLanded
  // callback shows the elected poster's title as a "tap to watch" hint.
  private spin(): void {
    if (this.spinning || this.posters.length === 0 || !this.overlay) return;
    const plan = planRouletteSpin(this.posters.length, Date.now());
    if (plan.winnerIndex < 0) return;
    this.spinning = true;
    this.overlay.setCtaBusy(true);
    this.overlay.setCtaLabel(translate("spinning", this.init?.locale));
    this.deps.renderer.runRoulette(plan, (index) => this.landed(index));
  }

  private landed(index: number): void {
    this.spinning = false;
    if (this.disposed || !this.overlay) return;
    this.overlay.setCtaBusy(false);
    this.overlay.setCtaLabel(translate("cta", this.init?.locale));
    const poster = this.posters[index];
    if (poster) this.overlay.setTitle(`${poster.title} · ${translate("play_hint", this.init?.locale)}`);
  }

  private hover(index: number | null): void {
    if (!this.overlay) return;
    if (index === null) {
      this.overlay.setTitle(null);
      return;
    }
    const poster = this.posters[index];
    this.overlay.setTitle(poster ? poster.title : null);
  }

  private activate(index: number): void {
    const poster = this.posters[index];
    if (poster) this.deps.bridge.navigate(`/media/${poster.id}`);
  }
}
