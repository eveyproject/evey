import { useCallback, useEffect, useState } from "react";

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
  prompt(): Promise<void>;
}

export type RemotePromptOutcome = "connected" | "dismissed" | "error";

function remoteOf(video: HTMLVideoElement | null): RemotePlayback | null {
  const r = (video as unknown as { remote?: RemotePlayback } | null)?.remote;
  return r && typeof r.watchAvailability === "function" ? r : null;
}

export function useRemotePlayback(video: HTMLVideoElement | null) {
  const [available, setAvailable] = useState(false);
  const [connected, setConnected] = useState(false);
  // Whether the element exposes the Remote Playback API at all. A safety net
  // for gating the button when watchAvailability is slow or never fires.
  const supported = remoteOf(video) != null;

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

  const prompt = useCallback(async () => {
    const remote = remoteOf(video);
    if (!remote) return;
    try {
      await remote.prompt();
    } catch {
      // User dismissed the picker, or no device accepted - nothing to do.
    }
  }, [video]);

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
    video.src = url;
    try {
      await remote.prompt();
      return "connected";
    } catch (err) {
      // NotAllowedError covers both a dismissed picker and an expired user
      // activation - neither deserves an error toast.
      return (err as DOMException | null)?.name === "NotAllowedError" ? "dismissed" : "error";
    }
  }, [video]);

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
