import { QueryClient } from "@tanstack/react-query";
import { shouldRetryQuery, useServerListStore } from "@evey/store";
import { setupOnlineManager } from "./online-manager";
import { mutationErrorNet } from "./mutationErrorNet";
import { clearProfileCache } from "./profileCache";

// Wired here rather than in a component so it is installed exactly once, before
// the first query is ever created. Without it React Query never learns the
// device went offline (it listens for a browser `online` event React Native
// does not emit), so refetchOnReconnect is dead and a tunnel leaves every
// screen stale.
setupOnlineManager();

export const queryClient = new QueryClient({
  mutationCache: mutationErrorNet(),
  defaultOptions: {
    queries: {
      staleTime: 5 * 60 * 1000,
      // Shared with web and TV: see @evey/store/queryRetry.
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
    // Same reason the query cache goes: the profile list belongs to the server
    // being left, and the picker would otherwise open on the wrong household.
    void clearProfileCache();
  }
});
