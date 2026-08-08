import { useCallback, useEffect, useRef, useState } from "react";
import { noopReporter, type PlayerEventReporter } from "@evey/api";



// Chrome for Android's ONLY web cast path is the Remote Playback API
// (HTMLMediaElement.remote). The Google Cast Sender SDK never initializes
// there - chrome.cast stays undefined and __onGCastApiAvailable is never
// called - so a page that only wires the SDK is blind to Chromecasts on
// mobile. This hook surfaces the native path: watchAvailability() drives a
// device-present signal, prompt()/promptForUrl() open the OS device picker,
// and the connect/disconnect events track the live session so the UI can
// show a real "stop casting" state.
//
// On desktop the Cast SDK stays the preferred path (richer control); WebKit
// exposes this same interface for AirPlay, so Safari gets a working button
// as a side effect.
//
// Requires disableRemotePlayback to NOT be set on the element - the caller
// keeps the attribute on the MAIN player and casts through a dedicated
// hidden <video> instead, because flinging transmits the element's source
// URL and an MSE (hls.js) element has none.

interface RemotePlayback {
  state?: "connecting" | "connected" | "disconnected";
  addEventListener?: (name: string, cb: () => void) => void;
  removeEventListener?: (name: string, cb: () => void) => void;
  watchAvailability(cb: (available: boolean) => void): Promise<number>;
  cancelWatchAvailability(id: number): Promise<void>;
  prompt(): Promise<RemotePromptOutcome>;
}

export type RemotePromptOutcome = "connected" | "dismissed" | "refused" | "error";

// Chromium rejects prompt() with the SAME NotAllowedError for two opposite
// things: the user closing the picker, and the browser refusing to open it at
// all (measured in prod 2026-07-24: rejection 18ms after the tap, activation
// still valid - remote_playback.cc cancels the prompt when the element's
// duration is unknown and its availability-URL list is therefore empty).
// A human cannot see and close a picker in under this many milliseconds, so
// elapsed time is what separates "refused" (a bug to surface) from
// "dismissed" (a choice to respect silently).
export const INSTANT_REJECT_MS = 500;

export function classifyPromptRejection(errName: string | undefined, elapsedMs: number): RemotePromptOutcome {
  if (errName === "NotAllowedError") return elapsedMs < INSTANT_REJECT_MS ? "refused" : "dismissed";
  return "error";
}

function remoteOf(video: HTMLVideoElement | null): RemotePlayback | null {
  const r = (video as unknown as { remote?: RemotePlayback } | null)?.remote;
  return r && typeof r.watchAvailability === "function" ? r : null;
}

// A cast that fails on the phone is INVISIBLE to the server: the fling is
// phone-to-TV, so when the picker yields no device not one byte is sent and
// the logs stay blank. Reports of "casting is broken" were undiagnosable for
// that reason alone. `report` sends the attempt and its outcome to the
// existing player beacon so the failure exists somewhere an operator can read.
// It carries NO URL - the cast token in it is a 12-hour bearer credential.
export function useRemotePlayback(
  video: HTMLVideoElement | null,
  report: PlayerEventReporter = noopReporter,
) {
  const [available, setAvailable] = useState(false);
  const [connected, setConnected] = useState(false);
  // Whether the element exposes the Remote Playback API at all. A safety net
  // for gating the button when watchAvailability is slow or never fires.
  const supported = remoteOf(video) != null;
  // Read at tap time from the prompt callbacks, which must not re-create
  // themselves on every availability flip.
  const availableRef = useRef(false);
  availableRef.current = available;
  const reportRef = useRef(report);
  reportRef.current = report;

  // The facts no server log could ever answer, captured at the instant of the
  // tap. `activation` is the one that separates the two causes hiding behind a
  // NotAllowedError: false means the tap's transient user activation was spent
  // (an await between the tap and prompt()), true means the browser refused for
  // its own reasons. `src`/`state` say whether the element even had something
  // flingable to offer.
  const reportAttempt = useCallback(() => {
    const remote = remoteOf(video);
    const activation = typeof navigator !== "undefined" && "userActivation" in navigator
      ? (navigator as Navigator & { userActivation?: { isActive: boolean } }).userActivation?.isActive
      : undefined;
    reportRef.current({
      type: "cast_prompt",
      details: `available=${availableRef.current} supported=${remote != null}`
        + ` activation=${activation ?? "n/a"}`
        + ` src=${video?.currentSrc ? "yes" : video?.src ? "set" : "none"}`
        + ` state=${(remote as { state?: string } | null)?.state ?? "n/a"}`
        // duration/readyState are THE facts behind an instant NotAllowedError:
        // Chromium refuses to open the picker while duration is NaN (metadata
        // never loaded), and no server log can see the element's load state.
        + ` duration=${video?.duration ?? "n/a"}`
        + ` readyState=${video?.readyState ?? "n/a"}`,
    });
  }, [video]);

  const reportOutcome = useCallback((outcome: RemotePromptOutcome, errName?: string, elapsedMs?: number) => {
    reportRef.current({
      type: "cast_prompt_result",
      // The DOMException name is the whole diagnosis: NotSupportedError (the
      // source is not flingable) and NotFoundError (no device took it) are
      // different bugs, and an empty catch erased both.
      // ALWAYS carry the exception name, including on a "dismissal":
      // NotAllowedError covers both a user closing the picker and a browser
      // refusing outright, and collapsing them to "dismissed" is exactly what
      // made a real failure look like a user cancelling for weeks.
      details: (errName ? `${outcome}:${errName}` : outcome)
        + (elapsedMs != null ? ` elapsedMs=${Math.round(elapsedMs)}` : ""),
      fatal: outcome === "error" || outcome === "refused",
    });
  }, []);

  useEffect(() => {
    const remote = remoteOf(video);
    if (!remote) {
      setAvailable(false);
      setConnected(false);
      return;
    }
    let cancelled = false;
    let watchId: number | null = null;
    remote
      .watchAvailability((isAvailable) => {
        if (!cancelled) setAvailable(isAvailable);
      })
      .then((id) => {
        if (cancelled) {
          // Unmounted before the watch registered - tear it down immediately.
          remote.cancelWatchAvailability(id).catch(() => {});
        } else {
          watchId = id;
        }
      })
      .catch(() => {
        // Rejected when disableRemotePlayback is set on the element, or the
        // platform can't watch. Treat as simply "no native cast here".
        if (!cancelled) setAvailable(false);
      });

    // Live session tracking: connecting counts as connected for the UI (the
    // handoff is under way, the local player must already stand down).
    const sync = () => {
      if (!cancelled) setConnected(remote.state === "connected" || remote.state === "connecting");
    };
    sync();
    remote.addEventListener?.("connecting", sync);
    remote.addEventListener?.("connect", sync);
    remote.addEventListener?.("disconnect", sync);

    return () => {
      cancelled = true;
      if (watchId != null) remote.cancelWatchAvailability(watchId).catch(() => {});
      remote.removeEventListener?.("connecting", sync);
      remote.removeEventListener?.("connect", sync);
      remote.removeEventListener?.("disconnect", sync);
    };
  }, [video]);

  // Prompts on whatever source the element already carries. The caller MUST
  // reach this without awaiting anything first: prompt() consumes the tap's
  // transient user activation, and a single network round-trip before it makes
  // Chrome reject with NotAllowedError - no picker, instant rejection. Measured
  // in production: 13 taps, 13 rejections in the same second, zero pickers.
  const prompt = useCallback(async (): Promise<RemotePromptOutcome> => {
    const remote = remoteOf(video);
    if (!remote) return "error";
    reportAttempt();
    const startedAt = Date.now();
    try {
      await remote.prompt();
      reportOutcome("connected");
      return "connected";
    } catch (err) {
      const name = (err as DOMException | null)?.name;
      const elapsed = Date.now() - startedAt;
      const outcome = classifyPromptRejection(name, elapsed);
      reportOutcome(outcome, name, elapsed);
      return outcome;
    }
  }, [video, reportAttempt, reportOutcome]);

  // Casts an arbitrary URL by putting it on the element first, then prompting.
  // This is how HLS reaches a Chromecast on Chrome Android: the receiver
  // flings the element's source URL and decodes the HLS itself (it doesn't
  // need the local element to decode it). Outcomes let the caller react
  // honestly: pause the local player only on a real handoff, stay silent on a
  // dismissal, surface an error otherwise (a mute failure is undiagnosable).
  const promptForUrl = useCallback(async (url: string): Promise<RemotePromptOutcome> => {
    if (!video) return "error";
    const remote = remoteOf(video);
    if (!remote) return "error";
    reportAttempt();
    video.src = url;
    const startedAt = Date.now();
    try {
      await remote.prompt();
      reportOutcome("connected");
      return "connected";
    } catch (err) {
      const name = (err as DOMException | null)?.name;
      const elapsed = Date.now() - startedAt;
      const outcome = classifyPromptRejection(name, elapsed);
      reportOutcome(outcome, name, elapsed);
      return outcome;
    }
  }, [video, reportAttempt, reportOutcome]);

  // The spec exposes no disconnect() - flipping disableRemotePlayback runs the
  // "disable remote playback steps", which reject pending promises and tear
  // the connection down. Restore it right after so the element stays castable.
  const stop = useCallback(async () => {
    if (!video) return;
    video.disableRemotePlayback = true;
    video.disableRemotePlayback = false;
  }, [video]);

  return { available, supported, connected, prompt, promptForUrl, stop };
}
