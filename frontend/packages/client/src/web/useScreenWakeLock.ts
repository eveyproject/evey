import { useEffect } from "react";

interface WakeLockSentinelLike {
  release: () => Promise<void>;
  addEventListener?: (type: "release", listener: () => void) => void;
}
interface WakeLockLike {
  request: (type: "screen") => Promise<WakeLockSentinelLike>;
}

/**
 * Holds the screen awake while something is playing.
 *
 * A television puts itself to sleep on an idle remote, and the HDMI output
 * goes with it: the picture AND the sound stop mid-album (reported 2026-09-21
 * on the Android television, which holds a keep-awake for exactly this). The
 * video element covers itself on most browsers; audio alone never does.
 *
 * The lock is dropped by the browser whenever the document is hidden, so it
 * has to be taken again on the way back - without that, one trip to another
 * application silently ends it for the rest of the session.
 */
export function useScreenWakeLock(active: boolean): void {
  useEffect(() => {
    if (!active) return;
    const wakeLock = (navigator as Navigator & { wakeLock?: WakeLockLike }).wakeLock;
    if (!wakeLock) return;

    let live = true;
    let sentinel: WakeLockSentinelLike | null = null;

    const acquire = () => {
      if (!live || document.visibilityState !== "visible" || sentinel) return;
      wakeLock.request("screen").then((lock) => {
        if (!live) { void lock.release().catch(() => { /* already gone */ }); return; }
        sentinel = lock;
        // The browser releases it on its own when the page is hidden; forget
        // it then, so coming back takes a fresh one.
        lock.addEventListener?.("release", () => { sentinel = null; });
      }).catch(() => { /* refused: nothing to undo */ });
    };

    const onVisible = () => {
      if (document.visibilityState === "visible") acquire();
      else sentinel = null;
    };

    acquire();
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      live = false;
      document.removeEventListener("visibilitychange", onVisible);
      const held = sentinel;
      sentinel = null;
      void held?.release().catch(() => { /* already gone */ });
    };
  }, [active]);
}
