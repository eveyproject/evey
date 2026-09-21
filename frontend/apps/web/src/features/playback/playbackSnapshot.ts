// playbackSnapshot is the picture of the <video> element at the moment an
// error is beaconed: what a stall report cannot do without. "bufferStalledError
// at 329 s" says the picture froze; it does not say whether the buffer was
// empty (server late), full (decoder or renderer stalled - dropped frames),
// or whether the tab was even visible. The 2026-09-03 tester report could not
// be settled from the logs for exactly that reason.
//
// Compact on purpose: it rides the beacon's `state` field, which the server
// clamps like `details`.
export interface EngineStats {
  mode?: string;
  ctxState?: string;
  anchored?: boolean;
  activeSources?: number;
  decoded?: number;
  driftMs?: number;
  leadMs?: number;
  leadMeasured?: boolean;
  userOffsetMs?: number;
  outputLatencyMs?: number;
  // Audio-output heals attempted this session (audioHealth.ts); omitted from
  // the line while zero, so a healthy session's snapshot reads as before.
  heals?: number;
}

// Engine stats are registered per <video> element by the wasm audio hook, so
// the hls.js error beacon (which never sees the engine) can still carry the
// audio leg's state in the same snapshot.
const engineStatsByVideo = new WeakMap<HTMLVideoElement, () => EngineStats>();

export function registerEngineStats(video: HTMLVideoElement, stats: () => EngineStats): () => void {
  engineStatsByVideo.set(video, stats);
  return () => {
    if (engineStatsByVideo.get(video) === stats) engineStatsByVideo.delete(video);
  };
}

// Same idea for the render watch: how many frames actually reached the screen,
// and how late a 250 ms timer ran. Registered per element so EVERY beacon
// carries it, not just the slowdown report - the interesting comparison is a
// stall's numbers against a healthy moment's.
const renderStatsByVideo = new WeakMap<HTMLVideoElement, () => string | null>();

export function registerRenderStats(video: HTMLVideoElement, stats: () => string | null): () => void {
  renderStatsByVideo.set(video, stats);
  return () => {
    if (renderStatsByVideo.get(video) === stats) renderStatsByVideo.delete(video);
  };
}

function fmt(n: number): string {
  return Number.isFinite(n) ? n.toFixed(1) : "-";
}

export function playbackSnapshot(video: HTMLVideoElement, engine?: EngineStats | null): string {
  if (engine === undefined) {
    const stats = engineStatsByVideo.get(video);
    if (stats) {
      try {
        engine = stats();
      } catch {
        engine = null;
      }
    }
  }
  const ct = video.currentTime;
  let inRange = "none";
  let nextGap = "";
  try {
    const b = video.buffered;
    for (let i = 0; i < b.length; i++) {
      const s = b.start(i);
      const e = b.end(i);
      if (s <= ct + 0.05 && ct <= e) {
        inRange = `${fmt(s)}-${fmt(e)}`;
        if (i + 1 < b.length) nextGap = ` next+${fmt(b.start(i + 1) - e)}`;
        break;
      }
      if (s > ct && inRange === "none") {
        inRange = `none next@${fmt(s)}`;
        break;
      }
    }
  } catch {
    inRange = "?";
  }
  const parts = [`rs=${video.readyState}`, `buf=${inRange}${nextGap}`, `rate=${video.playbackRate}`];
  const q = (video as HTMLVideoElement & { getVideoPlaybackQuality?: () => { droppedVideoFrames: number; totalVideoFrames: number } }).getVideoPlaybackQuality?.();
  if (q) parts.push(`drop=${q.droppedVideoFrames}/${q.totalVideoFrames}`);
  if (typeof document !== "undefined") parts.push(`vis=${document.visibilityState}`);
  if (engine) {
    parts.push(`wasm=${engine.mode ?? "?"}/${engine.ctxState ?? "?"}/${engine.anchored ? "anch" : "free"}/src${engine.activeSources ?? 0}/dec${engine.decoded ?? 0}`);
    if (engine.driftMs != null && Number.isFinite(engine.driftMs)) parts.push(`drift=${Math.round(engine.driftMs)}ms`);
    // The delay the schedule adds (measured or base) and the sink's own
    // latency: a viewer's "audio late" report reads against these, not drift.
    if (engine.leadMs != null && Number.isFinite(engine.leadMs)) parts.push(`lead=${Math.round(engine.leadMs)}ms${engine.leadMeasured ? "" : "*"}`);
    if (engine.userOffsetMs) parts.push(`off=${Math.round(engine.userOffsetMs)}ms`);
    if (engine.outputLatencyMs != null && Number.isFinite(engine.outputLatencyMs)) parts.push(`out=${Math.round(engine.outputLatencyMs)}ms`);
    if (engine.heals) parts.push(`heal=${engine.heals}`);
  }
  const render = renderStatsByVideo.get(video);
  if (render) {
    try {
      const line = render();
      if (line) parts.push(line);
    } catch { /* a broken watch must never cost us the rest of the snapshot */ }
  }
  return parts.join(" ");
}
