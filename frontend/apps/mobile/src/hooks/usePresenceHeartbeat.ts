import { useEffect } from "react";
import { AppState } from "react-native";
import { presenceServiceHeartbeat } from "@evey/api";

// Same cadence as the web tab (PresenceHeartbeat.tsx): the server's online
// window is 90s, so 30s tolerates a couple of missed beats before drop-off.
const HEARTBEAT_MS = 30_000;

// usePresenceHeartbeat puts this device on the admin "online" panel while the
// app is in the FOREGROUND. Background/frozen states deliberately stop
// beating: the server window then drops the row, which is the honest state -
// a pocketed phone is not "online". Fire-and-forget: a failed beat is
// recovered by the next one and never surfaces to the user.
//
// enabled gates the whole thing: the auth layout mounts this before its
// early returns (hook-order rule), and beating without a token would 401
// into the unauthorized flow.
export function usePresenceHeartbeat(enabled: boolean) {
  useEffect(() => {
    if (!enabled) return;
    let id: ReturnType<typeof setInterval> | null = null;
    const beat = () => {
      void presenceServiceHeartbeat({}).catch(() => {});
    };
    const start = () => {
      if (id != null) return;
      beat();
      id = setInterval(beat, HEARTBEAT_MS);
    };
    const stop = () => {
      if (id != null) {
        clearInterval(id);
        id = null;
      }
    };
    start();
    const sub = AppState.addEventListener("change", (state) => {
      if (state === "active") start();
      else stop();
    });
    return () => {
      stop();
      sub.remove();
    };
  }, [enabled]);
}
