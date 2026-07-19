import { useCallback, useEffect, useState } from "react";

// Chrome for Android's ONLY web cast path is the Remote Playback API
// (HTMLMediaElement.remote). The Google Cast Sender SDK never initializes
// there - chrome.cast stays undefined and __onGCastApiAvailable is never
// called - so a page that only wires the SDK is blind to Chromecasts on
// mobile. This hook surfaces the native path: watchAvailability() drives a
// device-present signal (so a button can appear only when a Chromecast is
// reachable on the user's LAN) and prompt() opens the OS device picker.
//
// It is the mirror of the SDK: on desktop the API is absent or never reports
// availability (desktop uses the Presentation API / the Cast SDK instead), so
// `available` simply stays false and the SDK button keeps driving casting.
//
// Requires disableRemotePlayback to NOT be set on the element - the caller
// gates that attribute on the SDK being active (see HLSPlayer). A limitation
// worth knowing: the native path replays the element's own media, so it casts
// direct-play sources but not an MSE (hls.js) stream, which has no replayable
// URL.

interface RemotePlayback {
  watchAvailability(cb: (available: boolean) => void): Promise<number>;
  cancelWatchAvailability(id: number): Promise<void>;
  prompt(): Promise<void>;
}

function remoteOf(video: HTMLVideoElement | null): RemotePlayback | null {
  const r = (video as unknown as { remote?: RemotePlayback } | null)?.remote;
  return r && typeof r.watchAvailability === "function" ? r : null;
}

export function useRemotePlayback(video: HTMLVideoElement | null) {
  const [available, setAvailable] = useState(false);
  // Whether the element exposes the Remote Playback API at all. True on
  // Chrome Android (the cast surface), false on desktop where the Cast SDK
  // drives casting. A safety net for gating when watchAvailability is slow.
  const supported = remoteOf(video) != null;

  useEffect(() => {
    const remote = remoteOf(video);
    if (!remote) {
      setAvailable(false);
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

    return () => {
      cancelled = true;
      if (watchId != null) remote.cancelWatchAvailability(watchId).catch(() => {});
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
  // This is how HLS reaches a Chromecast on Chrome Android: the receiver flings
  // the element's source URL and decodes the HLS itself (it doesn't need the
  // local element to decode it). The element here is a dedicated, hidden cast
  // <video> - never the MSE-fed local player, which has no flingable URL.
  const promptForUrl = useCallback(async (url: string) => {
    if (!video) return;
    const remote = remoteOf(video);
    if (!remote) return;
    video.src = url;
    try {
      await remote.prompt();
    } catch {
      // User dismissed the picker, or no device accepted - nothing to do.
    }
  }, [video]);

  return { available, supported, prompt, promptForUrl };
}
