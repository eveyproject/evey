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

// How long the elected winner is held (hero card + live countdown) before we
// auto-open its detail page - long enough to decide "watch it, or reroll".
const WINNER_HOLD_MS = 8000;

interface Deps {
  renderer: SceneRenderer;
  bridge: HostBridge;
}

export class MarqueeApp {
  private overlay: OverlayHandle | null = null;
  private init: HostInit | null = null;
  private posters: MediaSummary[] = [];
  private posterUrls: string[] = [];
  private spinning = false;
  private picking = false;
  private pickedId = "";
  private countdownTimer = 0;
  private landDeadline = 0;

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
    clearInterval(this.countdownTimer);
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
    this.overlay.setHeroWatchLabel(translate("watch_now", init.locale));
    this.overlay.onHeroWatch(() => this.watchPicked());
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

    // The sandbox CSP blocks the external CDN poster URL and its opaque origin
    // taints cross-origin textures - so resolve each poster to a host-relayed
    // data: URL. Parallel + best-effort: a failed one keeps its placeholder.
    const dataUrls = await Promise.all(
      this.posters.map((p) => this.deps.bridge.readPoster(p.id).catch(() => "")),
    );
    if (this.disposed) return;
    // Kept for the DOM hero card, which blows up the same host-relayed poster
    // the wall texture uses (no second fetch).
    this.posterUrls = dataUrls;

    this.deps.renderer.setWall({
      layout,
      posters: this.posters.map((p, i) => ({ id: p.id, title: p.title, posterUrl: dataUrls[i] || "" })),
    });
  }

  // "Ce soir, on regarde ?" - spins the wheel, then the renderer's onLanded
  // callback shows the elected poster's title as a "tap to watch" hint.
  private spin(): void {
    if (this.spinning || this.posters.length === 0 || !this.overlay) return;
    this.stopCountdown();
    this.picking = false;
    this.overlay.hideHero();
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
    const poster = this.posters[index];
    if (!poster) {
      this.overlay.setCtaLabel(translate("cta", this.init?.locale));
      return;
    }
    // The wheel picked one: blow it up in the hero card (poster + title), let
    // the CTA become "reroll", and run a LIVE countdown so the user can decide
    // to watch it, tap "watch now", or reroll before it auto-opens.
    this.picking = true;
    this.pickedId = poster.id;
    this.overlay.showHero("pick", this.posterUrls[index] || "", poster.title);
    this.overlay.setCtaLabel(translate("retry", this.init?.locale));
    this.landDeadline = Date.now() + WINNER_HOLD_MS;
    this.tickCountdown(poster.id);
    this.stopCountdown();
    this.countdownTimer = window.setInterval(() => this.tickCountdown(poster.id), 200);
  }

  private tickCountdown(posterId: string): void {
    if (this.disposed || !this.overlay) return;
    const remainingMs = this.landDeadline - Date.now();
    if (remainingMs <= 0) {
      this.stopCountdown();
      this.goToMedia(posterId);
      return;
    }
    const secs = Math.ceil(remainingMs / 1000);
    this.overlay.setHeroCountdown(
      translate("opening_in", this.init?.locale).replace("{s}", String(secs)),
    );
  }

  private stopCountdown(): void {
    clearInterval(this.countdownTimer);
    this.countdownTimer = 0;
  }

  // The "watch now" button on the pick card: skip the rest of the countdown.
  private watchPicked(): void {
    if (this.picking && this.pickedId) this.goToMedia(this.pickedId);
  }

  private hover(index: number | null): void {
    if (!this.overlay || this.picking) return; // never override the pick card
    if (index === null) {
      this.overlay.hideHero();
      return;
    }
    const poster = this.posters[index];
    if (!poster) {
      this.overlay.hideHero();
      return;
    }
    this.overlay.showHero("preview", this.posterUrls[index] || "", poster.title);
  }

  private activate(index: number): void {
    const poster = this.posters[index];
    if (!poster) return;
    this.goToMedia(poster.id);
  }

  private goToMedia(id: string): void {
    this.stopCountdown();
    this.picking = false;
    if (!this.disposed) this.deps.bridge.navigate(`/media/${id}`);
  }
}
