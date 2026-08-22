import { QueryClient } from "@tanstack/react-query";
import { shouldRetryQuery } from "@evey/store";
import { setupOnlineManager } from "./online-manager";
import { mutationErrorNet } from "./mutationErrorNet";

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
