// Application layer: orchestrates ports around the domain.
//   init -> load the caller's library -> filter to watchable items -> bucket
//   by decade -> lay out the galaxy (capped by the renderer's device-tier
//   capacity) -> compute the taste profile -> push everything to the
//   renderer/overlay. The renderer owns per-frame timing/pointer-picking;
//   this layer only reacts to its hover/activate callbacks. All timers/
//   listeners are owned here and released in dispose().

import type { HostBridge, HostInit } from "../domain/ports/HostBridge";
import type { SceneRenderer } from "../domain/ports/SceneRenderer";
import type { CinephileItem } from "../domain/model/media";
import { filterWatchable } from "../domain/services/mediaFilter";
import { aggregateByDecade, formatDecadeRange } from "../domain/services/decadeAggregation";
import { buildGalaxyLayout, type GalaxyNode } from "../domain/services/galaxyLayout";
import { buildTimeline } from "../domain/services/timeline";
import { computeTasteProfile } from "../domain/services/tasteProfile";
import { translate } from "../i18n/labels";
import { createOverlay, type OverlayHandle, type ProfileRow } from "../ui/overlay";

// Long-open dashboard tabs still occasionally pick up newly added titles.
const LIBRARY_REFRESH_MS = 30 * 60 * 1000;
const REPRESENTATIVE_PER_NODE = 4;

interface Deps {
  renderer: SceneRenderer;
  bridge: HostBridge;
}

export class CinephileApp {
  private overlay: OverlayHandle | null = null;
  private init: HostInit | null = null;
  private nodes: GalaxyNode[] = [];

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

  // ---- internals ------------------------------------------------------

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
    const h = w < 420 ? 380 : w < 700 ? 460 : 520;
    this.deps.bridge.resize(h);
  }

  private async loadLibrary(): Promise<void> {
    if (this.disposed || !this.overlay) return;
    const raw = await this.deps.bridge.listMedia();
    if (this.disposed) return;

    const items = filterWatchable(raw);
    if (items.length === 0) {
      this.nodes = [];
      this.deps.renderer.setScene({ nodes: [] });
      this.overlay.setProfile(null, "");
      this.overlay.setTimeline([], null);
      this.overlay.setTooltip(null);
      this.overlay.setEmptyMessage(translate("empty", this.init?.locale));
      return;
    }
    this.overlay.setEmptyMessage(null);

    const buckets = aggregateByDecade(items);
    const capacity = this.deps.renderer.getCapacity();
    this.nodes = buildGalaxyLayout(buckets, {
      maxNodes: capacity.maxNodes,
      representativePerNode: REPRESENTATIVE_PER_NODE,
    });
    this.deps.renderer.setScene({ nodes: this.nodes });

    this.overlay.setProfile(this.buildProfileRows(items), translate("profileHeading", this.init?.locale));

    const timeline = buildTimeline(buckets);
    const edgeLabels =
      timeline.length > 1
        ? { start: formatDecadeRange(timeline[0].decade), end: formatDecadeRange(timeline[timeline.length - 1].decade) }
        : null;
    this.overlay.setTimeline(timeline, edgeLabels);
  }

  private buildProfileRows(items: CinephileItem[]): ProfileRow[] {
    const locale = this.init?.locale;
    const profile = computeTasteProfile(items);
    const rows: ProfileRow[] = [{ label: translate("statTotal", locale), value: String(profile.totalCount) }];
    if (profile.dominantDecade !== null) {
      rows.push({ label: translate("statDominantDecade", locale), value: formatDecadeRange(profile.dominantDecade) });
    }
    rows.push({ label: translate("statMovies", locale), value: String(profile.movieCount) });
    rows.push({ label: translate("statSeries", locale), value: String(profile.seriesCount) });
    if (profile.oldestYear !== null && profile.newestYear !== null) {
      rows.push({ label: translate("statSpan", locale), value: `${profile.oldestYear} - ${profile.newestYear}` });
    }
    return rows;
  }

  private hover(index: number | null): void {
    if (!this.overlay) return;
    if (index === null) {
      this.overlay.setTooltip(null);
      return;
    }
    const node = this.nodes[index];
    if (!node) {
      this.overlay.setTooltip(null);
      return;
    }
    const label = node.decade === null ? translate("otherDecade", this.init?.locale) : formatDecadeRange(node.decade);
    const suffix = translate("titlesSuffix", this.init?.locale);
    const titles = node.representative
      .slice(0, 3)
      .map((item) => item.title)
      .join(", ");
    const parts = [`${label} · ${node.count} ${suffix}`];
    if (titles) parts.push(titles);
    this.overlay.setTooltip(parts.join(" · "));
  }

  // Cluster click navigates to a representative title's page: the `public`
  // media projection carries no library id, so a decade-filtered library
  // view isn't addressable here - the representative title is the next best
  // "show me those" moment.
  private activate(index: number): void {
    const node = this.nodes[index];
    const pick = node?.representative[0];
    if (pick) this.deps.bridge.navigate(`/media/${pick.id}`);
  }
}
