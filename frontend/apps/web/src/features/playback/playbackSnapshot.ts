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
  }
  return parts.join(" ");
}
