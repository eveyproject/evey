import { useEffect } from "react";
import { presenceServiceHeartbeat } from "@evey/api";

// How often an open, visible client announces itself. The server's online
// window is 90s, so 30s tolerates a couple of missed or throttled beats.
const HEARTBEAT_MS = 30_000;

/**
 * Tells the server this viewer is connected while the page is visible, so the
 * admin's "Online" panel knows about a client that is only browsing. Fire and
 * forget: a failed beat is ignored, the next one recovers.
 */
export function usePresenceHeartbeat(enabled = true) {
  useEffect(() => {
    if (!enabled) return;
    const beat = () => {
      if (typeof document !== "undefined" && document.visibilityState === "hidden") return;
      void presenceServiceHeartbeat({}).catch(() => { /* the next beat recovers */ });
    };
    beat();
    const id = setInterval(beat, HEARTBEAT_MS);
    const onVisible = () => { if (document.visibilityState === "visible") beat(); };
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      clearInterval(id);
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, [enabled]);
}
