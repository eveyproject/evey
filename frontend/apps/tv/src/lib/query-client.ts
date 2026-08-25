import { QueryClient } from "@tanstack/react-query";
import { shouldRetryQuery, useServerListStore } from "@evey/store";
import { mutationErrorNet } from "./mutationErrorNet";

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
