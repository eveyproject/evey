import { useEffect, useRef } from "react";
import { noopReporter, type PlayerEventReporter } from "@evey/api";
import { buildTranscodeHlsUrl } from "../hooks/hlsPlayerUtils";
import { WasmAudioEngine } from "./WasmAudioEngine";
import { playbackSnapshot, registerEngineStats } from "../playbackSnapshot";
import { extractAacAudioURI, extractAudioURI, injectToken } from "./wasmAudioShared";

// Two steady-state sync beacons per session: after the relay hand-over has
// settled, and well into continuous playback.
const SYNC_SAMPLE_AFTER_MS = [20_000, 120_000];

// Compatibility re-export: the pure helpers live in wasmAudioShared
// (shared with the Cast Web Receiver, which must not pull in React/hls.js).
export { extractAudioURI, injectToken } from "./wasmAudioShared";

interface UseWasmAudioPlaybackArgs {
  videoRef: React.RefObject<HTMLVideoElement | null>;
  mediaId: string;
  currentTrackIndex: number;
  streamTokenRef: React.MutableRefObject<string>;
  // onStreamTokenRefresh mints a fresh stream token and writes it into
  // streamTokenRef.current - the SAME callback the video/HLS leg already uses
  // on its own 401 (see useTranscodePlayback). Reused, not duplicated: the
  // engine has no way to heal a 401 without it (2026-09-08 storm - a token
  // expiring during a long pause 401'd every audio segment forever).
  onStreamTokenRefresh: React.MutableRefObject<() => Promise<string>>;
  // active gates the whole path: the browser can't decode AC-3/E-AC-3 natively
  // (wasm-capable) AND the selected audio track is AC-3/E-AC-3 AND we're in
  // transcode/HLS mode. When false the hook is inert (native audio plays).
  active: boolean;
  // onFallback fires when the wasm path cannot run (wasm load / playlist /
  // first-segment decode failed, or the server didn't emit an audio URI). The
  // host re-negotiates without wasmAudio. Volume/mute are NOT props: the engine
  // mirrors the <video> element's volume/muted directly (the native UI's target).
  onFallback: () => void;
  // reportEvent carries the engine's telemetry to the server's player-event
  // beacon: which leg is playing (wasm_audio_mode), why the relay latched
  // (wasm_audio_guard), what killed it (wasm_audio_fatal) and how often the
  // stall gate cut the sound (wasm_audio_gate). The cast receiver has had the
  // first two since 2026-07-30; the web player had NONE, so a beta tester's
  // "no sound" on 2026-09-02 could only be inferred from hls.js stall lines.
  reportEvent?: PlayerEventReporter;
  // onLegChange names the relay leg the viewer is HEARING: "aac" while the wasm
  // decoder boots, "wasm" once it carries the film, "aac-latched" when it gave
  // up and the server's AAC transcode finishes the film. The host's own flag
  // only says which master was asked for, so without this the stream panel kept
  // announcing "décodé client" over a latched session (prod, 2026-09-20).
  onLegChange?: (leg: string) => void;
  // audioOffsetMs is the viewer's menu correction (positive = audio later),
  // applied live to the engine's schedule - never by reloading the video leg.
  audioOffsetMs?: number;
}

// stallGateReportDue thins the stall-gate stream: report the first cut, then
// every doubling (1, 2, 4, 8, ...). The reporter's per-session budget is 20
// events; a server serving every segment late trips the gate at each 3 s
// boundary, and reporting each cut would drain the budget in one minute.
export function stallGateReportDue(cuts: number): boolean {
  return cuts > 0 && (cuts & (cuts - 1)) === 0;
}

// useWasmAudioPlayback runs the ac3go wasm audio engine alongside hls.js. The
// video plays muted (video-only master); this hook fetches that master, reads
// the #EVEY-AUDIO-URI rendition, and drives WasmAudioEngine to play the AC-3
// audio in sync. It re-mounts on media/track change and tears down cleanly.
export function useWasmAudioPlayback({
  videoRef,
  mediaId,
  currentTrackIndex,
  streamTokenRef,
  onStreamTokenRefresh,
  active,
  onFallback,
  reportEvent = noopReporter,
  onLegChange,
  audioOffsetMs = 0,
}: UseWasmAudioPlaybackArgs): void {
  const engineRef = useRef<WasmAudioEngine | null>(null);
  const audioOffsetRef = useRef(audioOffsetMs);
  audioOffsetRef.current = audioOffsetMs;
  useEffect(() => {
    engineRef.current?.setUserOffsetSec(audioOffsetMs / 1000);
  }, [audioOffsetMs]);
  const onFallbackRef = useRef(onFallback);
  onFallbackRef.current = onFallback;
  const reportRef = useRef(reportEvent);
  reportRef.current = reportEvent;
  const legRef = useRef(onLegChange);
  legRef.current = onLegChange;

  // (Re)build the engine when the media, the selected audio track, or the active
  // gate changes.
  useEffect(() => {
    if (!active) return;
    const video = videoRef.current;
    if (!video) return;

    let cancelled = false;
    let unregisterStats: (() => void) | null = null;
    const syncTimers: ReturnType<typeof setTimeout>[] = [];
    // posSec is the picture's clock at the moment of the event: it is what
    // lets an operator line a mode change up with the server's segment lines.
    const report = (type: string, details: string, fatal = false): void =>
      reportRef.current({
        type,
        details,
        fatal,
        posSec: Math.round(video.currentTime * 10) / 10,
        state: playbackSnapshot(video),
      });
    // The video-only stream carries no audio track, so we do NOT mute the
    // element: video.volume / video.muted stay under the native UI's control and
    // the engine mirrors them (they are silent no-ops on the element itself).

    const masterUrl = buildTranscodeHlsUrl(
      mediaId,
      streamTokenRef.current,
      currentTrackIndex,
      null,
      video.currentTime, // pre-warm hint: the audio session starts at the position
      0,
      false,
      true, // wasmAudio
    );

    (async () => {
      try {
        const resp = await fetch(injectToken(masterUrl, streamTokenRef.current), { credentials: "include" });
        if (!resp.ok) throw new Error(`master ${resp.status}`);
        const text = await resp.text();
        const audioUri = extractAudioURI(text);
        if (cancelled) return;
        if (!audioUri) {
          // Server didn't take the wasm path - play natively.
          report("wasm_audio_mode", "declined");
          onFallbackRef.current();
          return;
        }
        const base = new URL(masterUrl, window.location.origin);
        const playlistUrl = new URL(audioUri, base).toString();
        // Bi-source relay: sound starts on the natively decoded AAC rendition
        // while the wasm decoder boots, then hands over at a segment boundary.
        // Absent tag (feature off, older server) = classic wasm-only start.
        const aacUri = extractAacAudioURI(text);
        const engine = new WasmAudioEngine(video, playlistUrl, {
          resolveUrl: (u) => injectToken(u, streamTokenRef.current),
          onFatal: (reason) => {
            report("wasm_audio_fatal", reason, true);
            onFallbackRef.current();
          },
          onAuthError: async () => {
            // Mirrors the video leg's own 401 handler (useTranscodePlayback):
            // mint a fresh token and write it into the SHARED streamTokenRef
            // so both legs' resolveUrl/xhrSetup pick it up on their next
            // request - one source of truth for the token, not two.
            const newToken = await onStreamTokenRefresh.current();
            streamTokenRef.current = newToken;
            return newToken;
          },
          aacPlaylistUrl: aacUri ? new URL(aacUri, base).toString() : undefined,
          userOffsetSec: audioOffsetRef.current / 1000,
          onModeChange: (mode) => {
            legRef.current?.(mode);
            report("wasm_audio_mode", mode);
          },
          onDiagnostic: (detail) => report("wasm_audio_guard", detail),
          onStallGate: (cuts) => {
            if (stallGateReportDue(cuts)) report("wasm_audio_gate", `cuts=${cuts}`);
          },
        });
        engineRef.current = engine;
        unregisterStats = registerEngineStats(video, () => engine.debugStats());
        // Steady-state sync samples: every other beacon fires on an event, so a
        // film that plays without incident left no trace of lead/drift/output
        // latency - the figures a "sound is late" report is checked against.
        for (const afterMs of SYNC_SAMPLE_AFTER_MS) {
          syncTimers.push(setTimeout(() => {
            if (!cancelled && !video.paused) report("wasm_audio_sync", `t=${Math.round(afterMs / 1000)}s`);
          }, afterMs));
        }
        await engine.start();
      } catch (err) {
        if (cancelled) return;
        console.warn("[useWasmAudioPlayback] setup failed, falling back:", err);
        report("wasm_audio_fatal", err instanceof Error ? err.message : String(err), true);
        onFallbackRef.current();
      }
    })();

    return () => {
      cancelled = true;
      for (const t of syncTimers) clearTimeout(t);
      unregisterStats?.();
      engineRef.current?.destroy();
      engineRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [active, mediaId, currentTrackIndex, videoRef, streamTokenRef, onStreamTokenRefresh]);
}
