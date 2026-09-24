import { QueryClient } from "@tanstack/react-query";
import { AppState } from "react-native";
import { shouldRetryQuery, useServerListStore } from "@evey/store";
import { setupReachability } from "@evey/client";
import { mutationErrorNet } from "./mutationErrorNet";
import { pingServer, resolveServerUrl } from "./server-config";

export const queryClient = new QueryClient({
  mutationCache: mutationErrorNet(),
  defaultOptions: {
    queries: {
      staleTime: 5 * 60 * 1000,
      // Shared with web and mobile: see @evey/store/queryRetry.
      retry: shouldRetryQuery,
    },
  },
});

// React Native emits no browser `online` event: without this the television
// never learnt it had lost the network, and a wifi drop or a wake from standby
// left its screens on failed queries (see setupReachability). The phone reads
// the OS network state instead (expo-network, which this app does not ship).
setupReachability({
  queryClient,
  probe: async () => {
    const url = await resolveServerUrl(useServerListStore.getState().getActiveServer()?.url);
    return url ? pingServer(url) : true;
  },
  onForeground: (listener) => {
    const sub = AppState.addEventListener("change", (next) => {
      if (next === "active") listener();
    });
    return () => sub.remove();
  },
});

// Switching the active server invalidates EVERYTHING the cache holds: query
// keys do not carry the server, so home rows, media details and profile data
// fetched from server A would be served as server B's until each query went
// stale. One subscription here covers every switch path (manage-servers,
// server-connect, the single-server migration) instead of trusting each call
// site to remember. The null guard leaves boot-time rehydration (null -> the
// saved id) alone; going TO null (last server removed) does clear.
useServerListStore.subscribe((state, prev) => {
  if (prev.activeServerId !== null && state.activeServerId !== prev.activeServerId) {
    queryClient.clear();
  }
});
