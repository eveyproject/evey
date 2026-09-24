import { onlineManager, type QueryClient } from "@tanstack/react-query";
import { classifyError } from "./errorClassification";

// React Query learns that the network is gone from a browser `online` /
// `offline` event, which React Native never emits: on the Android television a
// wifi drop left every screen on its failed queries until the viewer moved,
// and the queries never came back on their own. The phone reads the OS network
// state (expo-network); the television has no such module, so it asks the one
// question that matters to it - does the server answer - and only when a
// request has just failed for want of a network, or when the app comes back to
// the foreground (the box waking from standby).

export interface ReachabilityOptions {
  queryClient: QueryClient;
  /** Whether the server answers (its /healthz). */
  probe: () => Promise<boolean>;
  /** Subscribes to the app coming back to the foreground; returns the unsubscribe. */
  onForeground: (listener: () => void) => () => void;
  /** How often the server is asked again while unreachable. */
  pollMs?: number;
}

const POLL_MS = 15_000;

export function setupReachability({ queryClient, probe, onForeground, pollMs = POLL_MS }: ReachabilityOptions): void {
  onlineManager.setEventListener((setOnline) => {
    let poll: ReturnType<typeof setInterval> | null = null;
    let checking = false;
    let disposed = false;

    const check = async (): Promise<boolean> => {
      if (checking) return onlineManager.isOnline();
      checking = true;
      try {
        const ok = await probe().catch(() => false);
        if (disposed) return ok;
        // Back online, React Query resumes the paused queries and refetches
        // the ones that failed; offline, it pauses them instead of failing.
        setOnline(ok);
        if (ok && poll) {
          clearInterval(poll);
          poll = null;
        } else if (!ok && !poll) {
          poll = setInterval(() => void check(), pollMs);
        }
        return ok;
      } finally {
        checking = false;
      }
    };

    // A request that got no answer at all (classifyError "offline"): the
    // network or the server is gone - find out which.
    const unsubscribeCache = queryClient.getQueryCache().subscribe((event) => {
      if (event.type === "updated" && event.action.type === "error" && classifyError(event.action.error) === "offline") {
        void check();
      }
    });

    // Waking up: the network may have come back while the app slept, in which
    // case nothing changes state and the failed screens would stay failed.
    const unsubscribeForeground = onForeground(() => {
      void check().then((ok) => {
        if (ok && !disposed) void queryClient.refetchQueries({ type: "active", predicate: (q) => q.state.status === "error" });
      });
    });

    return () => {
      disposed = true;
      if (poll) clearInterval(poll);
      unsubscribeCache();
      unsubscribeForeground();
    };
  });
}
