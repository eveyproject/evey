import { useCallback, useEffect, useRef } from "react";

// useVolumeBoost lifts the player's audio above the browser's 1.0 ceiling, the
// same amplification trick a desktop player uses: an <video>'s `volume` maxes
// out at 100%, so anything louder has to run through a Web Audio GainNode whose
// gain can go past 1. A DynamicsCompressor sits after the gain as a soft
// limiter - raw amplification clips already-loud passages, the compressor
// rounds that overshoot off instead of letting it distort.
//
//   video → MediaElementAudioSourceNode → Gain(1..2) → Compressor → destination
//
// The graph is built lazily on the first boost above 1×, for two reasons:
//   - createMediaElementSource may be called only ONCE per element and, once
//     called, *captures* the element's output - users who never touch the boost
//     keep the plain native path (and if Web Audio is unavailable the player
//     still works, it just can't exceed 100%);
//   - an AudioContext starts suspended until a user gesture, and the first boost
//     drag IS that gesture, so we can resume() it right there.
//
// The base 0-100% range stays on the native `video.volume`; this hook only owns
// the 1×-2× amplification on top.
export function useVolumeBoost(videoRef: React.RefObject<HTMLVideoElement | null>) {
  const ctxRef = useRef<AudioContext | null>(null);
  const gainRef = useRef<GainNode | null>(null);
  const sourceRef = useRef<MediaElementAudioSourceNode | null>(null);
  // The element the source node is bound to - a MediaElementSourceNode lives
  // and dies with one element, so a media swap (new <video>) must rebuild it.
  const elRef = useRef<HTMLVideoElement | null>(null);
  // Last gain the user asked for. Firefox creates the context "suspended" even
  // mid-gesture and resume() completes (or gets granted) asynchronously; a ramp
  // scheduled before that runs against a frozen clock and never becomes
  // audible - the UI says 200% while the native, unboosted path keeps playing.
  // Whenever the context actually reaches "running", this is what to apply.
  const pendingGainRef = useRef(1);
  // The play/playing listeners that keep the captured element's context awake,
  // held so teardown can detach them from the exact element they were bound to.
  const playResumeRef = useRef<{ video: HTMLVideoElement; fn: () => void } | null>(null);

  const teardown = useCallback(() => {
    if (playResumeRef.current) {
      const { video, fn } = playResumeRef.current;
      video.removeEventListener("play", fn);
      video.removeEventListener("playing", fn);
      playResumeRef.current = null;
    }
    try { sourceRef.current?.disconnect(); } catch { /* already gone */ }
    try { gainRef.current?.disconnect(); } catch { /* already gone */ }
    try { void ctxRef.current?.close(); } catch { /* already closing */ }
    ctxRef.current = null;
    gainRef.current = null;
    sourceRef.current = null;
    elRef.current = null;
  }, []);

  const ensureGraph = useCallback((): GainNode | null => {
    const video = videoRef.current;
    if (!video) return null;
    // Element was swapped under us - drop the stale graph before rebuilding.
    if (sourceRef.current && elRef.current !== video) teardown();
    if (gainRef.current) return gainRef.current;

    // Only build the graph once the page has genuine user activation. The "play"
    // event can fire from muted autoplay (not a gesture), and creating/resuming an
    // AudioContext then just logs "AudioContext couldn't start automatically"
    // without producing sound. Defer until the user actually interacts (dragging
    // the slider, scrolling the wheel, a key press). Undefined API (older Safari)
    // → assume active so behaviour there is unchanged.
    if (navigator.userActivation?.hasBeenActive === false) return null;

    const AC: typeof AudioContext | undefined =
      window.AudioContext ?? (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
    if (!AC) return null;
    try {
      const ctx = new AC();
      const source = ctx.createMediaElementSource(video);
      const gain = ctx.createGain();
      const comp = ctx.createDynamicsCompressor();
      // Soft brickwall that only engages near full scale: content peaking below
      // -3 dBFS (i.e. the untouched 0-100% range) passes through, the boosted
      // overshoot above it gets limited instead of clipping.
      comp.threshold.value = -3;
      comp.knee.value = 6;
      comp.ratio.value = 12;
      comp.attack.value = 0.003;
      comp.release.value = 0.25;
      source.connect(gain);
      gain.connect(comp);
      comp.connect(ctx.destination);
      // However the context reaches "running" (our resume(), a later gesture,
      // the tab regaining audio focus), make the boost real at that moment.
      // And the mirror case: once createMediaElementSource has CAPTURED the
      // element, its audio ONLY exists through this context - so a context that
      // drops to "suspended" while the picture keeps playing (a pause/play
      // cycle, tab backgrounding, an autoplay-policy blip) is silent video, not
      // just a frozen boost. Nothing else resumes it until the next slider drag.
      // Resume it here whenever the video is actually playing - the moment sound
      // is wanted - so it can never strand a boosted stream mute.
      ctx.onstatechange = () => {
        if (ctx.state === "running" && gainRef.current) {
          gainRef.current.gain.setTargetAtTime(pendingGainRef.current, ctx.currentTime, 0.02);
        } else if (ctx.state === "suspended" && videoRef.current && !videoRef.current.paused) {
          ctx.resume().catch(() => { /* autoplay gate: a real gesture retries */ });
        }
      };
      // Resume on play/playing too: onstatechange does not fire when the video
      // resumes while the context is ALREADY suspended, so a pause that suspended
      // the context would otherwise come back picture-only.
      const resumeIfSuspended = () => {
        if (ctxRef.current?.state === "suspended") {
          ctxRef.current.resume().catch(() => { /* gesture-gated; retried next play/drag */ });
        }
      };
      video.addEventListener("play", resumeIfSuspended);
      video.addEventListener("playing", resumeIfSuspended);
      playResumeRef.current = { video, fn: resumeIfSuspended };
      ctxRef.current = ctx;
      gainRef.current = gain;
      sourceRef.current = source;
      elRef.current = video;
      return gain;
    } catch (err) {
      // A tainted (cross-origin, no CORS) element or a hostile browser: keep the
      // native path working, just without boost.
      console.warn("volume boost unavailable, staying at 100% max", err);
      teardown();
      return null;
    }
  }, [videoRef, teardown]);

  // applyPendingGain (re)schedules the ramp for the last requested gain against
  // the context's CURRENT clock - the only schedule that matters once running.
  const applyPendingGain = useCallback(() => {
    const node = gainRef.current;
    const ctx = ctxRef.current;
    if (!node || !ctx) return;
    // Ramp instead of a hard set to avoid a zipper click while dragging.
    node.gain.setTargetAtTime(pendingGainRef.current, ctx.currentTime, 0.02);
  }, []);

  // setBoost applies a 1×-2× amplification. gain <= 1 with no graph yet is a
  // no-op so we never build the graph for users who stay at/under 100%.
  const setBoost = useCallback((gain: number) => {
    if (gain <= 1 && !gainRef.current) return;
    const node = ensureGraph();
    const ctx = ctxRef.current;
    if (!node || !ctx) return;
    pendingGainRef.current = Math.max(0, gain);
    if (ctx.state === "suspended") {
      // Re-apply once the resume actually lands (async in every browser; may
      // even be denied and only granted on a later gesture - each new setBoost
      // retries, and the statechange hook below catches any external resume).
      ctx.resume().then(applyPendingGain).catch(() => {
        // Denied for now (autoplay gate) - the next user gesture retries.
      });
    }
    applyPendingGain();
  }, [ensureGraph, applyPendingGain]);

  useEffect(() => teardown, [teardown]);

  return setBoost;
}
