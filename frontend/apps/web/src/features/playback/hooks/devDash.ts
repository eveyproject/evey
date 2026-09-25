// Dev-only DASH playback of the transcode path, behind a flag: the same
// session as the HLS master (the manifest.mpd request carries the master's
// query), played by dash.js, with HLS as the fallback whenever the server does
// not offer DASH for the presentation (404) or the player fails. The player
// is exposed to HLSPlayer through a small adapter shaped like the part of the
// hls.js API it drives (levels, currentLevel, audioTracks, audioTrack,
// config, startLoad, destroy), so the rung pinning and the audio switch run
// unchanged. Turn it on with localStorage["evey.dev.dash"] = "1".
import type Hls from "hls.js";

export const DEV_DASH_KEY = "evey.dev.dash";

export function devDashEnabled(): boolean {
  try {
    return globalThis.localStorage?.getItem(DEV_DASH_KEY) === "1";
  } catch {
    return false;
  }
}

// The manifest.mpd of the master a transcode URL names, with its query.
export function dashUrlFor(hlsUrl: string): string {
  return hlsUrl.replace("/master.m3u8", "/manifest.mpd");
}

export interface DashLevel { width: number; height: number; bitrate: number }
export interface DashAudioTrack { name: string; lang: string }

export interface DevDashSession {
  // Stands in for the hls.js instance in hlsRef.
  shim: Hls;
  levels: DashLevel[];
  audioTracks: DashAudioTrack[];
  destroy(): void;
}

interface Callbacks {
  onQuality(level: DashLevel): void;
  onError(reason: string): void;
}

// startDevDash loads mpdUrl into video with dash.js. Resolves once the stream
// is initialised; rejects when the server offers no DASH for it (the caller
// falls back to HLS).
// The rebuilds of one <video> run one at a time: a player attaches only once
// the previous one is destroyed (its destroy() resets the element, and ran
// under the next one when the two raced), and a start cancelled before it
// attaches never does.
let chain: Promise<unknown> = Promise.resolve();

export function startDevDash(video: HTMLVideoElement, mpdUrl: string, startSec: number, cb: Callbacks, cancelled: () => boolean): Promise<DevDashSession> {
  const started = chain.then(() => open(video, mpdUrl, startSec, cb, cancelled));
  chain = started.catch(() => undefined);
  return started;
}

// retire queues a session's teardown in the same chain.
export function retireDevDash(session: Promise<DevDashSession>): void {
  chain = session.then((s) => s.destroy(), () => undefined);
}

async function open(video: HTMLVideoElement, mpdUrl: string, startSec: number, cb: Callbacks, cancelled: () => boolean): Promise<DevDashSession> {
  const res = await fetch(mpdUrl);
  if (!res.ok) throw new Error(`manifest.mpd ${res.status}`);
  const dashjs = await import("dashjs");
  if (cancelled()) throw new Error("cancelled");
  const player = dashjs.MediaPlayer().create();
  // One rung pinned at a time, as the HLS path does (HLSPlayer's level sync).
  player.updateSettings({ streaming: { abr: { autoSwitchBitrate: { video: false, audio: false } }, buffer: { bufferTimeAtTopQuality: 30 } } });
  player.on(dashjs.MediaPlayer.events.ERROR, (e: unknown) => cb.onError(JSON.stringify((e as { error?: unknown }).error ?? e).slice(0, 300)));
  const ready = new Promise<void>((resolve) => player.on(dashjs.MediaPlayer.events.STREAM_INITIALIZED, () => resolve()));
  player.initialize(video, mpdUrl, false, startSec);
  await ready;

  const reps = [...player.getRepresentationsByType("video")].sort((a, b) => a.bandwidth - b.bandwidth);
  const levels = reps.map((r) => ({ width: r.width, height: r.height, bitrate: r.bandwidth }));
  const tracks = player.getTracksFor("audio");
  const audioTracks = tracks.map((t) => ({ name: t.labels?.[0]?.text ?? t.lang ?? "", lang: t.lang ?? "" }));
  player.on(dashjs.MediaPlayer.events.QUALITY_CHANGE_RENDERED, (e: { mediaType?: string; newRepresentation?: { id: string } }) => {
    if (e.mediaType !== "video") return;
    const i = reps.findIndex((r) => r.id === e.newRepresentation?.id);
    if (i >= 0) cb.onQuality(levels[i]);
  });

  const shim = {
    levels,
    get currentLevel() {
      const cur = player.getCurrentRepresentationForType("video");
      return reps.findIndex((r) => r.id === cur?.id);
    },
    set currentLevel(i: number) {
      if (i >= 0 && i < reps.length) player.setRepresentationForTypeById("video", reps[i].id, true);
    },
    audioTracks,
    get audioTrack() {
      const cur = player.getCurrentTrackFor("audio");
      return tracks.findIndex((t) => t.id === cur?.id && t.lang === cur?.lang);
    },
    set audioTrack(i: number) {
      if (i >= 0 && i < tracks.length) player.setCurrentTrack(tracks[i]);
    },
    config: { maxBufferLength: 30 },
    startLoad() { /* dash.js retries on its own */ },
    destroy() { player.destroy(); },
  };
  return { shim: shim as unknown as Hls, levels, audioTracks, destroy: () => player.destroy() };
}
