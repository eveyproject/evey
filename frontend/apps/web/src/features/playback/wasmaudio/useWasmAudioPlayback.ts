import { useEffect, useRef } from "react";
import { buildTranscodeHlsUrl } from "../hooks/hlsPlayerUtils";
import { WasmAudioEngine } from "./WasmAudioEngine";
import { extractAudioURI, injectToken } from "./wasmAudioShared";

// Ré-export de compatibilité : les helpers purs vivent dans wasmAudioShared
// (partagés avec le Web Receiver Cast, qui ne doit pas tirer React/hls.js).
export { extractAudioURI, injectToken } from "./wasmAudioShared";

interface UseWasmAudioPlaybackArgs {
  videoRef: React.RefObject<HTMLVideoElement | null>;
  mediaId: string;
  currentTrackIndex: number;
  streamTokenRef: React.MutableRefObject<string>;
  // active gates the whole path: the browser can't decode AC-3/E-AC-3 natively
  // (wasm-capable) AND the selected audio track is AC-3/E-AC-3 AND we're in
  // transcode/HLS mode. When false the hook is inert (native audio plays).
  active: boolean;
  // onFallback fires when the wasm path cannot run (wasm load / playlist /
  // first-segment decode failed, or the server didn't emit an audio URI). The
  // host re-negotiates without wasmAudio. Volume/mute are NOT props: the engine
  // mirrors the <video> element's volume/muted directly (the native UI's target).
  onFallback: () => void;
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
  active,
  onFallback,
}: UseWasmAudioPlaybackArgs): void {
  const engineRef = useRef<WasmAudioEngine | null>(null);
  const onFallbackRef = useRef(onFallback);
  onFallbackRef.current = onFallback;

  // (Re)build the engine when the media, the selected audio track, or the active
  // gate changes.
  useEffect(() => {
    if (!active) return;
    const video = videoRef.current;
    if (!video) return;

    let cancelled = false;
    // The video-only stream carries no audio track, so we do NOT mute the
    // element: video.volume / video.muted stay under the native UI's control and
    // the engine mirrors them (they are silent no-ops on the element itself).

    const masterUrl = buildTranscodeHlsUrl(
      mediaId,
      streamTokenRef.current,
      currentTrackIndex,
      null,
      0,
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
          onFallbackRef.current();
          return;
        }
        const base = new URL(masterUrl, window.location.origin);
        const playlistUrl = new URL(audioUri, base).toString();
        const engine = new WasmAudioEngine(video, playlistUrl, {
          resolveUrl: (u) => injectToken(u, streamTokenRef.current),
          onFatal: () => onFallbackRef.current(),
        });
        engineRef.current = engine;
        await engine.start();
      } catch (err) {
        if (cancelled) return;
        console.warn("[useWasmAudioPlayback] setup failed, falling back:", err);
        onFallbackRef.current();
      }
    })();

    return () => {
      cancelled = true;
      engineRef.current?.destroy();
      engineRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [active, mediaId, currentTrackIndex, videoRef, streamTokenRef]);
}
