import { useEffect } from "react";
import { presenceServiceHeartbeat } from "@evey/api";

// How often an open, visible tab announces itself. The server's online window is
// 90s, so 30s tolerates a couple of missed/throttled beats before drop-off.
const HEARTBEAT_MS = 30_000;

// PresenceHeartbeat pings the server on an interval while the tab is visible, so
// the admin "Online" panel knows this user is connected even when they are only
// browsing (not streaming). Fire-and-forget: a failed beat is ignored (the next
// one recovers), and errors never surface to the user. Rendered once inside the
// authenticated layout, so it runs for every logged-in user.
export function PresenceHeartbeat() {
  useEffect(() => {
    const beat = () => {
      if (typeof document !== "undefined" && document.visibilityState === "hidden") return;
      void presenceServiceHeartbeat({}).catch(() => {});
    };
    beat(); // announce immediately on mount
    const id = setInterval(beat, HEARTBEAT_MS);
    const onVisible = () => {
      if (document.visibilityState === "visible") beat();
    };
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      clearInterval(id);
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, []);
  return null;
}
